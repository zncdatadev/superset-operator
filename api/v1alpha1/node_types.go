package v1alpha1

import (
	apiv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

type NodeSpec struct {
	RoleGroups      map[string]NodeRoleGroupSpec    `json:"roleGroups,omitempty"`
	Config          *NodeConfigSpec                 `json:"config,omitempty"`
	RoleConfig      *commonsv1alpha1.RoleConfigSpec `json:"roleConfig,omitempty"`
	PodOverride     *corev1.PodTemplateSpec         `json:"podOverride,omitempty"`
	CliOverrides    []string                        `json:"cliOverrides,omitempty"`
	EnvOverrides    []string                        `json:"envOverrides,omitempty"`
	ConfigOverrides *NodeConfigOverridesSpec        `json:"configOverrides,omitempty"`
}

type NodeConfigSpec struct {
	Affinity                *corev1.Affinity                     `json:"affinity,omitempty"`
	PodDisruptionBudget     *apiv1alpha1.PodDisruptionBudgetSpec `json:"podDisruptionBudget,omitempty"`
	GracefulShutdownTimeout string                               `json:"gracefulShutdownTimeoutSeconds,omitempty"`
	Logging                 *LoggingSpec                         `json:"logging,omitempty"`
	Resources               *apiv1alpha1.ResourcesSpec           `json:"resources,omitempty"`
}

type NodeRoleGroupSpec struct {
	Replicas            *int32                               `json:"replicas,omitempty"`
	Config              *NodeConfigSpec                      `json:"config,omitempty"`
	PodOverride         *corev1.PodTemplateSpec              `json:"podOverride,omitempty"`
	PodDisruptionBudget *apiv1alpha1.PodDisruptionBudgetSpec `json:"podDisruptionBudget,omitempty"`
	CliOverrides        []string                             `json:"cliOverrides,omitempty"`
	EnvOverrides        map[string]string                    `json:"envOverrides,omitempty"`
	ConfigOverrides     *NodeConfigOverridesSpec             `json:"configOverrides,omitempty"`
}

type NodeConfigOverridesSpec struct {
}

type LoggingSpec struct {
	// +kubebuilder:validation:Optional
	EnableVectorAgent bool `json:"enableVectorAgent,omitempty"`
	// +kubebuilder:validation:Optional
	Containers map[string]*apiv1alpha1.LoggingConfigSpec `json:"containers,omitempty"`
}
