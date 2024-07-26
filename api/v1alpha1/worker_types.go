package v1alpha1

import (
	apiv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

type WorkerSpec struct {
	RoleGroups          map[string]WorkerRoleGroupSpec       `json:"roleGroups,omitempty"`
	Config              *WorkerConfigSpec                    `json:"config,omitempty"`
	PodOverride         *corev1.PodTemplateSpec              `json:"podOverride,omitempty"`
	PodDisruptionBudget *apiv1alpha1.PodDisruptionBudgetSpec `json:"podDisruptionBudget,omitempty"`
	CommandOverrides    []string                             `json:"commandOverrides,omitempty"`
	EnvOverrides        []string                             `json:"envOverrides,omitempty"`
	ConfigOverrides     *WorkerConfigOverridesSpec           `json:"configOverrides,omitempty"`
}

type WorkerConfigSpec struct {
	Affinity                *corev1.Affinity                     `json:"affinity,omitempty"`
	Tolerations             []corev1.Toleration                  `json:"tolerations,omitempty"`
	PodDisruptionBudget     *apiv1alpha1.PodDisruptionBudgetSpec `json:"podDisruptionBudget,omitempty"`
	GracefulShutdownTimeout string                               `json:"gracefulShutdownTimeoutSeconds,omitempty"`
	Logging                 *apiv1alpha1.LoggingConfigSpec       `json:"logging,omitempty"`
	Resources               *apiv1alpha1.ResourcesSpec           `json:"resources,omitempty"`
}

type WorkerRoleGroupSpec struct {
	Replicas            *int32                               `json:"replicas,omitempty"`
	Config              *WorkerConfigSpec                    `json:"config,omitempty"`
	PodOverride         *corev1.PodTemplateSpec              `json:"podOverride,omitempty"`
	PodDisruptionBudget *apiv1alpha1.PodDisruptionBudgetSpec `json:"podDisruptionBudget,omitempty"`
	CommandOverrides    []string                             `json:"commandOverrides,omitempty"`
	EnvOverrides        map[string]string                    `json:"envOverrides,omitempty"`
	ConfigOverrides     *WorkerConfigOverridesSpec           `json:"configOverrides,omitempty"`
}

type WorkerConfigOverridesSpec struct {
}
