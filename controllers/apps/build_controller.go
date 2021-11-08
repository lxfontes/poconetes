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
	"sort"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	appsv1 "github.com/lxfontes/poconetes/apis/apps/v1"
	v1 "github.com/lxfontes/poconetes/apis/apps/v1"
)

const builderIndexName = "idx." + v1.BuilderAnnotation

// BuildReconciler reconciles a Build object
type BuildReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	indexer client.FieldIndexer
}

//+kubebuilder:rbac:groups=apps.poconetes.dev,resources=builds,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps.poconetes.dev,resources=builds/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=apps.poconetes.dev,resources=builds/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps.poconetes.dev,resources=formations,verbs=get;list;watch;update;patch

// Reconcile ...
func (r *BuildReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ll := log.FromContext(ctx)
	build := &v1.Build{}
	if err := r.Get(ctx, req.NamespacedName, build); err != nil {
		return ctrl.Result{}, err
	}

	builderName, ok := build.GetAnnotations()[v1.BuilderAnnotation]
	if !ok {
		ll.Info("no build ownership defined")
		return ctrl.Result{}, nil
	}

	builds := &v1.BuildList{}
	if err := r.List(ctx,
		builds,
		client.InNamespace(req.Namespace),
		client.MatchingFields{builderIndexName: builderName},
	); err != nil {
		return ctrl.Result{}, err
	}

	// builds should be ordered by creation timestamp ( newer first )
	buildSorter(builds.Items)

	// if this reconciliation is not for the last build with state = success from this builder, then bail
	var lastOkBuild *v1.Build
	for _, b := range builds.Items {
		if b.Status.State == v1.BuildStateSuccess {
			lastOkBuild = &b
			break
		}
	}

	if lastOkBuild == nil {
		ll.Info("no successful builds")
		return ctrl.Result{}, nil
	}

	if lastOkBuild.UID != build.UID {
		ll.Info("not the latest build, skipping")
		return ctrl.Result{}, nil
	}

	// get all formations in the namespace that have the build annotation
	formations := &v1.FormationList{}
	if err := r.List(
		ctx,
		formations,
		client.InNamespace(build.Namespace),
		client.MatchingFields{builderIndexName: builderName},
	); err != nil {
		return ctrl.Result{}, err
	}
	if len(formations.Items) == 0 {
		ll.Info("no formations found")
	}

	// replace the image and save the formation
	var hasErrors bool
	for _, formRaw := range formations.Items {
		ll.Info("Updating Formation", "name", formRaw.GetName())
		f := formRaw.DeepCopy()
		f.Spec.Image = build.Spec.Image
		if err := r.Update(ctx, f); err != nil {
			ll.Error(err, "failed to promote build")
			hasErrors = true
		}
	}

	return ctrl.Result{Requeue: hasErrors}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BuildReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &v1.Formation{}, builderIndexName, func(rawObj client.Object) []string {
		f := rawObj.(*v1.Formation)
		builderTag, ok := f.GetAnnotations()[v1.BuilderAnnotation]
		if !ok {
			return nil
		}

		return []string{builderTag}
	}); err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &v1.Build{}, builderIndexName, func(rawObj client.Object) []string {
		f := rawObj.(*v1.Build)
		builderTag, ok := f.GetAnnotations()[v1.BuilderAnnotation]
		if !ok {
			return nil
		}

		return []string{builderTag}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1.Build{}).
		Complete(r)
}

func buildSorter(builds []v1.Build) {
	sort.Slice(builds, func(i, j int) bool {
		return builds[j].CreationTimestamp.Before(&builds[i].CreationTimestamp)
	})
}
