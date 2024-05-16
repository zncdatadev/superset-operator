package node

import (
	supersetv1alpha1 "github.com/zncdata-labs/superset-operator/api/v1alpha1"
	"github.com/zncdata-labs/superset-operator/pkg/image"
	"github.com/zncdata-labs/superset-operator/pkg/reconciler"
	corev1 "k8s.io/api/core/v1"
)

type DeploymentReconciler struct {
	reconciler.DeploymentReconciler[*supersetv1alpha1.NodeRoleGroupSpec]
	ClusterConfig *supersetv1alpha1.ClusterConfigSpec
}

func NewDeploymentReconciler(
	client reconciler.ResourceClient,

	name string,

	clusterConfig *supersetv1alpha1.ClusterConfigSpec,
	image image.Image,
	ports []corev1.ContainerPort,

	spec *supersetv1alpha1.NodeRoleGroupSpec,
) *DeploymentReconciler {

	return &DeploymentReconciler{
		DeploymentReconciler: *reconciler.NewDeploymentReconciler(
			client,
			name,
			image,
			ports,
			spec,
		),
		ClusterConfig: clusterConfig,
	}
}
