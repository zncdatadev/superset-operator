package v1alpha1

import (
	apiv1alpha1 "github.com/zncdata-labs/superset-operator/pkg/apis/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

type NodeSpec struct {
	RoleGroups          map[string]NodeRoleGroupSpec         `json:"roleGroups,omitempty"`
	Config              *NodeConfigSpec                      `json:"config,omitempty"`
	PodDisruptionBudget *apiv1alpha1.PodDisruptionBudgetSpec `json:"podDisruptionBudget,omitempty"`
	CommandOverrides    []string                             `json:"commandOverrides,omitempty"`
	EnvOverrides        []string                             `json:"envOverrides,omitempty"`
	ConfigOverrides     *NodeConfigOverridesSpec             `json:"configOverrides,omitempty"`
}

type NodeConfigSpec struct {
	Affinity                *corev1.Affinity                     `json:"affinity,omitempty"`
	Tolerations             []corev1.Toleration                  `json:"tolerations,omitempty"`
	PodDisruptionBudget     *apiv1alpha1.PodDisruptionBudgetSpec `json:"podDisruptionBudget,omitempty"`
	GracefulShutdownTimeout *string                              `json:"gracefulShutdownTimeoutSeconds,omitempty"`
	Logging                 *apiv1alpha1.LoggingConfigSpec       `json:"logging,omitempty"`
	Resources               *apiv1alpha1.ResourcesSpec           `json:"resources,omitempty"`
}

type NodeRoleGroupSpec struct {
	Replicas            *int32                               `json:"replicas,omitempty"`
	Config              *NodeConfigSpec                      `json:"config,omitempty"`
	PodDisruptionBudget *apiv1alpha1.PodDisruptionBudgetSpec `json:"podDisruptionBudget,omitempty"`
	CommandOverrides    []string                             `json:"commandOverrides,omitempty"`
	EnvOverrides        map[string]string                    `json:"envOverrides,omitempty"`
	ConfigOverrides     *NodeConfigOverridesSpec             `json:"configOverrides,omitempty"`
}

type NodeConfigOverridesSpec struct {
}
