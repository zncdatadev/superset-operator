package node

import (
	"github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/util"
	supersetv1alpha1 "github.com/zncdatadev/superset-operator/api/v1alpha1"
	"github.com/zncdatadev/superset-operator/internal/controller/common"
	corev1 "k8s.io/api/core/v1"

	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
)

func NewDeploymentReconciler(
	client *client.Client,
	roleGroupInfo reconciler.RoleGroupInfo,
	clusterConfig *supersetv1alpha1.ClusterConfigSpec,
	ports []corev1.ContainerPort,
	image *util.Image,
	replicas *int32,
	stopped bool,
	overrides *commonsv1alpha1.OverridesSpec,
	roleGroupConfig *commonsv1alpha1.RoleGroupConfigSpec,
) (*reconciler.Deployment, error) {

	deploymentBuilder := common.NewDeploymentBuilder(
		client,
		roleGroupInfo,
		clusterConfig,
		replicas,
		ports,
		image,
		overrides,
		roleGroupConfig,
	)

	return reconciler.NewDeployment(
		client,
		deploymentBuilder,
		stopped,
	), nil
}
