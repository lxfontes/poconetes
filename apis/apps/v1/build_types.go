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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type BuildState string

const (
	BuildStateUnknown BuildState = ""
	BuildStateSuccess BuildState = "success"
	BuildStateFailure BuildState = "failure"
)

// BuildSpec defines the desired state of Build
type BuildSpec struct {
	// Ref is the git branch
	Ref string `json:"ref"`
	// Head is the git commit ( sha )
	Head string `json:"head"`
	// Image ...
	Image string `json:"image,omitempty"`
	// Tag ...
	Tag string `json:"tag,omitempty"`
	// Skip flags the build as bad, preventing further deployments
	// +optional
	Skip bool `json:"skip,omitempty"`
}

// BuildStatus defines the observed state of Build
type BuildStatus struct {
	State BuildState `json:"state,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:path=builds

// Build is the Schema for the builds API
type Build struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BuildSpec   `json:"spec,omitempty"`
	Status BuildStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// BuildList contains a list of Build
type BuildList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Build `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Build{}, &BuildList{})
}
