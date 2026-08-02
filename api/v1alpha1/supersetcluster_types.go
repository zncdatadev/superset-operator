/*
Copyright 2024 zncdatadev.

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
	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SupersetClusterSpec defines the desired state of SupersetCluster
type SupersetClusterSpec struct {
	// +default:value={"repo": "quay.io/zncdatadev", "pullPolicy": "IfNotPresent"}
	Image            *ImageSpec                            `json:"image,omitempty"`
	ClusterConfig    *ClusterConfigSpec                    `json:"clusterConfig"`
	ClusterOperation *commonsv1alpha1.ClusterOperationSpec `json:"clusterOperation,omitempty"`
	Node             *NodeSpec                             `json:"node"`
}

// ToGenericSpec bridges the typed SupersetCluster spec into the framework's generic
// cluster spec the GenericReconciler operates on. Superset has a single role named
// "node"; its role groups, workload config and override layers are mirrored into the
// generic role map so the framework's merge pipeline (ProductConfig < role < roleGroup)
// applies unchanged.
func (s *SupersetClusterSpec) ToGenericSpec() *commonsv1alpha1.GenericClusterSpec {
	result := &commonsv1alpha1.GenericClusterSpec{
		ClusterOperation: s.ClusterOperation,
	}

	if s.Image != nil {
		result.Image = &commonsv1alpha1.ImageSpec{
			Custom:          s.Image.Custom,
			Repo:            s.Image.Repo,
			ProductVersion:  s.Image.ProductVersion,
			KubedoopVersion: s.Image.KubedoopVersion,
		}
		if s.Image.PullPolicy != nil {
			result.Image.PullPolicy = *s.Image.PullPolicy
		}
	}

	if s.Node != nil {
		roleSpec := commonsv1alpha1.RoleSpec{
			RoleConfig: s.Node.RoleConfig,
		}
		if s.Node.Config != nil {
			roleSpec.Config = s.Node.Config.RoleGroupConfigSpec
		}
		if o := s.Node.OverridesSpec; o != nil {
			roleSpec.ConfigOverrides = o.ConfigOverrides
			roleSpec.EnvOverrides = o.EnvOverrides
			roleSpec.CliOverrides = o.CliOverrides
			roleSpec.PodOverrides = o.PodOverrides
		}

		roleGroups := make(map[string]commonsv1alpha1.RoleGroupSpec, len(s.Node.RoleGroups))
		for name, rg := range s.Node.RoleGroups {
			adapted := commonsv1alpha1.RoleGroupSpec{
				Replicas: rg.Replicas,
			}
			if rg.Config != nil {
				adapted.Config = rg.Config.RoleGroupConfigSpec
			}
			if o := rg.OverridesSpec; o != nil {
				adapted.ConfigOverrides = o.ConfigOverrides
				adapted.EnvOverrides = o.EnvOverrides
				adapted.CliOverrides = o.CliOverrides
				adapted.PodOverrides = o.PodOverrides
			}
			roleGroups[name] = adapted
		}
		roleSpec.RoleGroups = roleGroups

		result.Roles = map[string]commonsv1alpha1.RoleSpec{RoleNameNode: roleSpec}
	}

	return result
}

// SupersetClusterStatus defines the observed state of SupersetCluster
type SupersetClusterStatus struct {
	commonsv1alpha1.GenericClusterStatus `json:",inline"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// SupersetCluster is the Schema for the supersetclusters API
type SupersetCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SupersetClusterSpec   `json:"spec,omitempty"`
	Status SupersetClusterStatus `json:"status,omitempty"`
}

// GetSpec implements common.ClusterInterface: it hands the GenericReconciler the
// generic view of the typed spec.
func (s *SupersetCluster) GetSpec() *commonsv1alpha1.GenericClusterSpec {
	return s.Spec.ToGenericSpec()
}

// GetStatus implements common.ClusterInterface: the framework updates conditions,
// observedGeneration and the role-group ledger through the returned pointer.
func (s *SupersetCluster) GetStatus() *commonsv1alpha1.GenericClusterStatus {
	return &s.Status.GenericClusterStatus
}

// VectorAggregatorConfigMapName implements reconciler.VectorAggregatorProvider. When
// non-empty, the GenericReconciler resolves the aggregator address from this ConfigMap
// (key ADDRESS) and exposes it to the handler as buildCtx.VectorAggregatorAddress.
func (s *SupersetCluster) VectorAggregatorConfigMapName() string {
	if s.Spec.ClusterConfig != nil {
		return s.Spec.ClusterConfig.VectorAggregatorConfigMapName
	}
	return ""
}

// +kubebuilder:object:root=true

// SupersetClusterList contains a list of SupersetCluster
type SupersetClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SupersetCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SupersetCluster{}, &SupersetClusterList{})
}
