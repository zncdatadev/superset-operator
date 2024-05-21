package node

import (
	supersetv1alpha1 "github.com/zncdata-labs/superset-operator/api/v1alpha1"
	"github.com/zncdata-labs/superset-operator/internal/controller/common"
	resourceClient "github.com/zncdata-labs/superset-operator/pkg/client"
	"github.com/zncdata-labs/superset-operator/pkg/reconciler"
	corev1 "k8s.io/api/core/v1"
)

func NewDeploymentReconciler(
	client resourceClient.ResourceClient,
	clusterConfig *common.ClusterConfig,
	roleGroupInfo *reconciler.RoleGroupInfo,
	ports []corev1.ContainerPort,
	spec *supersetv1alpha1.NodeRoleGroupSpec,
) *reconciler.DeploymentReconciler {
	deploymentBuilder := common.NewDeploymentBuilder(
		client,
		clusterConfig,
		roleGroupInfo,
		ports,
		spec.EnvOverrides,
		spec.CommandOverrides,
	)

	return reconciler.NewDeploymentReconciler(
		client,
		roleGroupInfo,
		ports,
		deploymentBuilder,
	)
}
