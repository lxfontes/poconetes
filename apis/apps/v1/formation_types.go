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

package v1

import (
	autoscaling "k8s.io/api/autoscaling/v2beta2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	LabelFormation = "poco-formation"
)

// FormationTLS ...
type FormationTLS struct {
	// +required
	Issuer corev1.ObjectReference `json:"issuer"`
	// +required
	Names []string `json:"names"`
}

// FormationSpec defines the desired state of Formation
type FormationSpec struct {
	// +Optional
	Plan string `json:"plan,omitempty"`
	// +required
	Image string `json:"image"`
	// +required
	MaxReplicas int32 `json:"maxReplicas"`
	// +optional
	MinReplicas *int32 `json:"minReplicas,omitempty"`
	// +optional
	Scaling []autoscaling.MetricSpec `json:"scaling,omitempty"`
	// +Optional
	TLS *FormationTLS `json:"tls,omitempty"`

	RunSpec `json:",inline"`
}

// Mount ...
type Mount struct {
	// +required
	Name string `json:"name"`
	// +required
	Path string `json:"path"`
	// +optional
	ConfigMap *corev1.LocalObjectReference `json:"configMapRef,omitempty"`
	// +optional
	Secret *corev1.LocalObjectReference `json:"secretRef,omitempty"`
}

type RunSpec struct {
	// +optional
	Command []string `json:"command,omitempty"`
	// +optional
	Args []string `json:"args,omitempty"`
	// +optional
	Ports []FormationPort `json:"ports,omitempty"`
	// +optional
	Environment []corev1.EnvVar `json:"environment,omitempty"`
	// +optional
	EnvironmentRefs []corev1.EnvFromSource `json:"environmentRefs,omitempty"`
	// +optional
	Mounts []Mount `json:"mounts,omitempty"`
}

// FormationPort ...
type FormationPort struct {
	// +required
	Name string `json:"name"`
	// +required
	Port int32 `json:"port,omitempty"`
	// +optional
	Protocol corev1.Protocol `json:"protocol,omitempty"`
}

// FormationStatus defines the observed state of Formation
type FormationStatus struct {
	ObservedGeneration  int64          `json:"observedGeneration,omitempty"`
	State               FormationState `json:"state"`
	Message             string         `json:"message,omitempty"`
	ReplicasDesired     int32          `json:"replicasDesired,omitempty"`
	ReplicasAvailable   int32          `json:"replicasAvailable"`
	ReplicasUnavailable int32          `json:"replicasUnavailable"`
}

type FormationState string

const (
	FormationStateUnknown FormationState = ""
	FormationStateOnline  FormationState = "online"
	FormationStateIdle    FormationState = "idle"
	FormationStateUpdate  FormationState = "update"
	FormationStateError   FormationState = "error"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Formation is the Schema for the formations API
type Formation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FormationSpec   `json:"spec,omitempty"`
	Status FormationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// FormationList contains a list of Formation
type FormationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Formation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Formation{}, &FormationList{})
}
