/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package apps

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strconv"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	appsv1 "github.com/lxfontes/poconetes/apis/apps/v1"

	stappsv1 "k8s.io/api/apps/v1"
	autoscaling "k8s.io/api/autoscaling/v2beta2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	certmanager "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1beta1"
	cmmeta "github.com/jetstack/cert-manager/pkg/apis/meta/v1"
)

// FormationReconciler reconciles a Formation object
type FormationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=apps.poconetes.dev,resources=formations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps.poconetes.dev,resources=formations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=apps.poconetes.dev,resources=formations/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Formation object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *FormationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ll := log.FromContext(ctx)

	formation := &appsv1.Formation{}
	if err := r.Get(ctx, req.NamespacedName, formation); err != nil {
		if apierrors.IsNotFound(err) {
			ll.Info("not found, skipping")
			return ctrl.Result{}, nil
		}
	}

	if formation.GetGeneration() != formation.Status.ObservedGeneration {
		if err := r.refreshObjects(ctx, formation); err != nil {
			ll.Error(err, "refreshing objects")
			return ctrl.Result{}, err
		}

		formation.Status.ObservedGeneration = formation.GetGeneration()
		return ctrl.Result{Requeue: true}, r.Status().Update(ctx, formation)
	}

	oldStatus := formation.Status
	if err := r.refreshStatus(ctx, formation); err != nil {
		ll.Error(err, "refreshing status")
		return ctrl.Result{}, err
	}

	if !reflect.DeepEqual(formation.Status, oldStatus) {
		ll.Info("updating status")
		return ctrl.Result{}, r.Status().Update(ctx, formation)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *FormationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1.Formation{}).
		Owns(&stappsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&autoscaling.HorizontalPodAutoscaler{}).
		Complete(r)
}

func (r *FormationReconciler) refreshObjects(ctx context.Context, formation *appsv1.Formation) error {
	plan, err := r.planFor(ctx, formation)
	if err != nil {
		return err
	}
	if err := r.refreshService(ctx, formation); err != nil {
		return err
	}
	if err := r.refreshCertificate(ctx, formation); err != nil {
		return err
	}
	if err := r.refreshDeployment(ctx, plan, formation); err != nil {
		return err
	}
	if err := r.refreshAutoscaler(ctx, plan, formation); err != nil {
		return err
	}

	return nil
}

func (r *FormationReconciler) refreshCertificate(ctx context.Context, formation *appsv1.Formation) error {
	wantLabels := map[string]string{
		appsv1.LabelFormation: formation.GetName(),
	}

	if formation.Spec.TLS == nil {
		return nil
	}

	spec := certmanager.CertificateSpec{
		SecretName: formation.GetName() + "-mtls",
		IssuerRef: cmmeta.ObjectReference{
			Kind: formation.Spec.TLS.Issuer.Kind,
			Name: formation.Spec.TLS.Issuer.Name,
		},
		DNSNames: formation.Spec.TLS.Names,
	}

	certificate := &certmanager.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       formation.GetNamespace(),
			Name:            formation.GetName(),
			Labels:          mergeLabels(formation.GetLabels(), wantLabels),
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(formation, formation.GroupVersionKind())},
		},
		Spec: spec,
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, certificate, func() error {
		certificate.SetLabels(mergeLabels(certificate.GetLabels(), formation.GetLabels(), wantLabels))
		return controllerutil.SetControllerReference(formation, certificate, r.Scheme)
	})
	return err
}

func (r *FormationReconciler) refreshService(ctx context.Context, formation *appsv1.Formation) error {
	wantLabels := map[string]string{
		appsv1.LabelFormation: formation.GetName(),
	}

	ports := []corev1.ServicePort{}
	for _, p := range formation.Spec.Ports {
		port := corev1.ServicePort{
			Name:       p.Name,
			Port:       p.Port,
			TargetPort: intstr.FromString(p.Name),
		}
		ports = append(ports, port)
	}

	sidecarList := &appsv1.SidecarList{}
	if err := r.List(ctx, sidecarList, client.InNamespace(formation.GetNamespace())); err != nil {
		return err
	}

	for _, sidecar := range sidecarList.Items {
		for _, p := range sidecar.Spec.Ports {
			port := corev1.ServicePort{
				Name:       p.Name,
				Port:       p.Port,
				TargetPort: intstr.FromString(p.Name),
			}
			ports = append(ports, port)
		}
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       formation.GetNamespace(),
			Name:            formation.GetName(),
			Labels:          mergeLabels(formation.GetLabels(), wantLabels),
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(formation, formation.GroupVersionKind())},
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: corev1.ClusterIPNone,
			Selector:  wantLabels,
			Ports:     ports,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, service, func() error {
		service.Spec.Selector = wantLabels
		service.Spec.Ports = ports
		service.SetLabels(mergeLabels(service.GetLabels(), formation.GetLabels(), wantLabels))
		return controllerutil.SetControllerReference(formation, service, r.Scheme)
	})
	return err
}

func (r *FormationReconciler) refreshAutoscaler(ctx context.Context, plan *appsv1.Plan, formation *appsv1.Formation) error {
	wantLabels := map[string]string{
		appsv1.LabelFormation: formation.GetName(),
	}

	spec := autoscaling.HorizontalPodAutoscalerSpec{
		ScaleTargetRef: autoscaling.CrossVersionObjectReference{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
			Name:       formation.GetName(),
		},
		MinReplicas: formation.Spec.MinReplicas,
		MaxReplicas: formation.Spec.MaxReplicas,
		Metrics:     formation.Spec.Scaling,
	}

	if len(spec.Metrics) == 0 {
		spec.Metrics = plan.Spec.Scaling
	}

	hpa := &autoscaling.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       formation.GetNamespace(),
			Name:            formation.GetName(),
			Labels:          mergeLabels(formation.GetLabels(), wantLabels),
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(formation, formation.GroupVersionKind())},
		},
		Spec: spec,
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, hpa, func() error {
		hpa.Spec = spec
		hpa.SetLabels(mergeLabels(hpa.GetLabels(), formation.GetLabels(), wantLabels))
		return controllerutil.SetControllerReference(formation, hpa, r.Scheme)
	})
	return err
}

func (r *FormationReconciler) planFor(ctx context.Context, formation *appsv1.Formation) (*appsv1.Plan, error) {
	plan := &appsv1.Plan{
		ObjectMeta: metav1.ObjectMeta{
			Name: appsv1.DefaultPlanName,
		},
	}

	planName := formation.Spec.Plan
	if planName == "" {
		planName = plan.GetName()
	}

	if err := r.Get(ctx, types.NamespacedName{Name: planName}, plan); err != nil {
		return nil, err
	}

	return plan, nil
}

func (r *FormationReconciler) refreshDeployment(ctx context.Context, plan *appsv1.Plan, formation *appsv1.Formation) error {
	wantLabels := map[string]string{
		appsv1.LabelFormation: formation.GetName(),
	}

	defaultLabels := map[string]string{
		appsv1.LabelPlan:      plan.GetName(),
		appsv1.LabelFormation: formation.GetName(),
	}

	defaultMounts := []appsv1.Mount{
		{
			Name: "poconetes-share",
			Path: "/poco/share",
		},
	}

	defaultEnv := []corev1.EnvVar{
		{
			Name:  "POCO_REVISION",
			Value: strconv.FormatInt(formation.GetGeneration(), 10),
		},
	}

	volumes := []corev1.Volume{
		{
			Name: "poconetes-share",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}

	if formation.Spec.TLS != nil {
		defaultMounts = append(defaultMounts, appsv1.Mount{
			Name: "poconetes-mtls",
			Path: "/poco/mtls",
		})

		volumes = append(volumes, corev1.Volume{
			Name: "poconetes-mtls",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: formation.GetName() + "-mtls",
				},
			},
		})
	}

	spec := stappsv1.DeploymentSpec{
		Selector: &metav1.LabelSelector{
			MatchLabels: wantLabels,
		},
		Replicas: formation.Spec.MinReplicas,
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: mergeLabels(formation.GetLabels(), wantLabels),
			},
			Spec: corev1.PodSpec{
				EnableServiceLinks:           boolPtr(false),
				AutomountServiceAccountToken: boolPtr(false),
				Affinity:                     plan.Spec.Affinity,
				DNSConfig:                    plan.Spec.DNS,
				Volumes:                      volumes,
			},
		},
	}

	container := corev1.Container{
		Name:         "pocolet",
		Image:        formation.Spec.Image,
		Command:      formation.Spec.Command,
		Args:         formation.Spec.Args,
		EnvFrom:      mergeEnvFromSource(formation.Spec.EnvironmentRefs),
		Env:          mergeEnvVar(formation.Spec.Environment, defaultEnv),
		VolumeMounts: mergeMounts(defaultMounts, formation.Spec.Mounts),
	}

	if plan.Spec.ContainerResources != nil {
		container.Resources = *plan.Spec.ContainerResources
	}

	for _, p := range formation.Spec.Ports {
		port := corev1.ContainerPort{
			Name:          p.Name,
			ContainerPort: p.Port,
		}
		container.Ports = append(container.Ports, port)
	}

	spec.Template.Spec.Containers = append(spec.Template.Spec.Containers, container)

	allVolumes := append([]appsv1.Mount{}, formation.Spec.Mounts...)

	sidecarList := &appsv1.SidecarList{}
	if err := r.List(ctx, sidecarList, client.InNamespace(formation.GetNamespace())); err != nil {
		return err
	}

	for _, sidecar := range sidecarList.Items {
		sidecarContainer := corev1.Container{
			Name:         "sidecar-" + sidecar.GetName(),
			Image:        sidecar.Spec.Image,
			Command:      sidecar.Spec.Command,
			Args:         sidecar.Spec.Args,
			EnvFrom:      mergeEnvFromSource(formation.Spec.EnvironmentRefs, sidecar.Spec.EnvironmentRefs),
			Env:          mergeEnvVar(formation.Spec.Environment, sidecar.Spec.Environment, defaultEnv),
			VolumeMounts: mergeMounts(defaultMounts, formation.Spec.Mounts, sidecar.Spec.Mounts),
		}

		if plan.Spec.SidecarResources != nil {
			sidecarContainer.Resources = *plan.Spec.SidecarResources
		}

		for _, p := range sidecar.Spec.Ports {
			port := corev1.ContainerPort{
				Name:          p.Name,
				ContainerPort: p.Port,
			}
			sidecarContainer.Ports = append(sidecarContainer.Ports, port)
		}

		spec.Template.Spec.Containers = append(spec.Template.Spec.Containers, sidecarContainer)

		allVolumes = append(allVolumes, sidecar.Spec.Mounts...)
	}

	initializerList := &appsv1.InitializerList{}
	if err := r.List(ctx, initializerList, client.InNamespace(formation.GetNamespace())); err != nil {
		return err
	}

	for _, initializer := range initializerList.Items {
		initializerContainer := corev1.Container{
			Name:         "init-" + initializer.GetName(),
			Image:        initializer.Spec.Image,
			Command:      initializer.Spec.Command,
			Args:         initializer.Spec.Args,
			EnvFrom:      mergeEnvFromSource(formation.Spec.EnvironmentRefs, initializer.Spec.EnvironmentRefs),
			Env:          mergeEnvVar(formation.Spec.Environment, initializer.Spec.Environment, defaultEnv),
			VolumeMounts: mergeMounts(defaultMounts, formation.Spec.Mounts, initializer.Spec.Mounts),
		}

		if plan.Spec.InitializerResources != nil {
			initializerContainer.Resources = *plan.Spec.InitializerResources
		}

		spec.Template.Spec.InitContainers = append(spec.Template.Spec.InitContainers, initializerContainer)

		allVolumes = append(allVolumes, initializer.Spec.Mounts...)
	}

	if err := mergeVolumes(&spec.Template.Spec, allVolumes...); err != nil {
		return err
	}

	deploy := &stappsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            formation.GetName(),
			Namespace:       formation.GetNamespace(),
			Labels:          mergeLabels(formation.GetLabels(), defaultLabels),
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(formation, formation.GroupVersionKind())},
		},
		Spec: spec,
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, deploy, func() error {
		spec.Replicas = deploy.Spec.Replicas // keep this stable, as HPA will be the one controlling it
		deploy.Spec = spec
		deploy.SetLabels(mergeLabels(deploy.GetLabels(), formation.GetLabels(), wantLabels))
		return controllerutil.SetControllerReference(formation, deploy, r.Scheme)
	})
	return err
}

func (r *FormationReconciler) refreshStatus(ctx context.Context, formation *appsv1.Formation) error {
	ll := log.FromContext(ctx)
	deploy := &stappsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: formation.GetNamespace(), Name: formation.GetName()}, deploy); err != nil {
		if apierrors.IsNotFound(err) {
			formation.Status.State = appsv1.FormationStateError
			formation.Status.Message = "deployment not found"
			return nil
		}
	}

	if !metav1.IsControlledBy(deploy, formation) {
		ll.Info("invalid owner for deployment")
		formation.Status.State = appsv1.FormationStateError
		formation.Status.Message = "invalid deployment ownership"
		return nil
	}

	formation.Status.ReplicasDesired = *deploy.Spec.Replicas
	formation.Status.ReplicasAvailable = deploy.Status.AvailableReplicas
	formation.Status.ReplicasUnavailable = deploy.Status.UnavailableReplicas

	if deploy.Status.ObservedGeneration != deploy.GetGeneration() {
		formation.Status.State = appsv1.FormationStateUpdate
		formation.Status.Message = "deployment being created"
		return nil
	}

	if deploy.Status.ReadyReplicas == deploy.Status.UpdatedReplicas &&
		deploy.Status.UnavailableReplicas == 0 {
		formation.Status.State = appsv1.FormationStateOnline
		formation.Status.Message = ""
		return nil
	}

	if deploy.Status.Replicas != deploy.Status.UpdatedReplicas {
		formation.Status.State = appsv1.FormationStateUpdate
		formation.Status.Message = "rolling out"
		return nil
	}

	if deploy.Status.UnavailableReplicas > 0 {
		formation.Status.State = appsv1.FormationStateError
		formation.Status.Message = "replica crashed"
	}

	formation.Status.State = appsv1.FormationStateError
	formation.Status.Message = "could not determine state"

	return nil
}

func mergeLabels(lbls ...map[string]string) map[string]string {
	ret := make(map[string]string)

	for _, lbl := range lbls {
		for k, v := range lbl {
			ret[k] = v
		}
	}

	return ret
}

func mergeMounts(mounts ...[]appsv1.Mount) []corev1.VolumeMount {
	ret := make([]corev1.VolumeMount, 0)

	for _, mnts := range mounts {
		for _, mount := range mnts {
			ret = append(ret, corev1.VolumeMount{
				Name:      mount.Name,
				MountPath: mount.Path,
			})
		}
	}

	return ret
}

func mergeVolumes(spec *corev1.PodSpec, mounts ...appsv1.Mount) error {
	idx := make(map[string]appsv1.Mount)
	// make sure volume is under spec.volumes
	for _, mount := range mounts {
		if dup, ok := idx[mount.Name]; ok {
			if !reflect.DeepEqual(mount, dup) {
				return fmt.Errorf("duplicated mount: %s", mount.Name)
			}
		}

		src := corev1.VolumeSource{}
		if mount.ConfigMap != nil {
			src.ConfigMap = &corev1.ConfigMapVolumeSource{
				Optional:             boolPtr(true),
				LocalObjectReference: *mount.ConfigMap,
			}
		} else if mount.Secret != nil {
			src.Secret = &corev1.SecretVolumeSource{
				Optional:   boolPtr(true),
				SecretName: mount.Secret.Name,
			}
		}

		spec.Volumes = append(spec.Volumes, corev1.Volume{
			Name:         mount.Name,
			VolumeSource: src,
		})

		idx[mount.Name] = mount
	}

	return nil
}

func mergeEnvFromSource(srcs ...[]corev1.EnvFromSource) []corev1.EnvFromSource {
	ret := make([]corev1.EnvFromSource, 0)

	for _, evs := range srcs {
		ret = append(ret, evs...)
	}

	return ret
}

func mergeEnvVar(envs ...[]corev1.EnvVar) []corev1.EnvVar {
	idx := make(map[string]corev1.EnvVar)

	for _, evs := range envs {
		for _, ev := range evs {
			idx[ev.Name] = ev
		}
	}

	keys := make([]string, 0)
	for k := range idx {
		keys = append(keys, k)
	}
	sort.Sort(sort.StringSlice(keys))

	ret := make([]corev1.EnvVar, len(keys))
	for i, k := range keys {
		ret[i] = idx[k]
	}

	return ret
}

func int32Ptr(i int32) *int32 {
	return &i
}

func boolPtr(t bool) *bool {
	return &t
}
