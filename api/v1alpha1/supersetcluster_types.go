/*
Copyright 2024 zncdata-labs.

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

package v1alpha1

import (
	apiv1alpha1 "github.com/zncdata-labs/superset-operator/pkg/apis/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SupersetClusterSpec defines the desired state of SupersetCluster
type SupersetClusterSpec struct {
	Image            *ImageSpec                        `json:"image,omitempty"`
	ClusterConfig    *ClusterConfigSpec                `json:"clusterConfig"`
	ClusterOperation *apiv1alpha1.ClusterOperationSpec `json:"clusterOperation"`
	Node             *NodeSpec                         `json:"node"`
	Worker           *WorkerSpec                       `json:"worker"`
}

// SupersetClusterStatus defines the observed state of SupersetCluster
type SupersetClusterStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// SupersetCluster is the Schema for the supersetclusters API
type SupersetCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SupersetClusterSpec   `json:"spec,omitempty"`
	Status SupersetClusterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SupersetClusterList contains a list of SupersetCluster
type SupersetClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SupersetCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SupersetCluster{}, &SupersetClusterList{})
}
