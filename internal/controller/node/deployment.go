package node

import (
	"github.com/zncdatadev/superset-operator/internal/controller/common"
	"github.com/zncdatadev/superset-operator/pkg/builder"
	"github.com/zncdatadev/superset-operator/pkg/client"
	"github.com/zncdatadev/superset-operator/pkg/reconciler"
)

func NewDeploymentReconciler(
	client *client.Client,
	clusterConfig *common.ClusterConfig,
	options *builder.RoleGroupOptions,
) *reconciler.DeploymentReconciler {
	deploymentBuilder := common.NewDeploymentBuilder(
		client,
		clusterConfig,
		options,
	)

	return reconciler.NewDeploymentReconciler(
		client,
		options,
		deploymentBuilder,
	)
}
