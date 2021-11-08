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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	BuilderAnnotation = "poco-builder"
)

// BuilderSpec defines the desired state of Builder
type BuilderSpec struct {
	// +optional
	Command []string `json:"command,omitempty"`
	// +optional
	Args []string `json:"args,omitempty"`
	// +optional
	Environment []corev1.EnvVar `json:"environment,omitempty"`
	// +optional
	EnvironmentRefs []corev1.EnvFromSource `json:"environmentRefs,omitempty"`
	// +optional
	Mounts []Mount `json:"mounts,omitempty"`
}

type BuilderState string

// BuilderStatus defines the observed state of Builder
type BuilderStatus struct {
	State BuilderState `json:"state,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Builder is the Schema for the builders API
type Builder struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BuilderSpec   `json:"spec,omitempty"`
	Status BuilderStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// BuilderList contains a list of Builder
type BuilderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Builder `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Builder{}, &BuilderList{})
}
