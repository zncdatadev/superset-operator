package node

import (
	"context"

	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	resourceClient "github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/util"

	supersetv1alpha1 "github.com/zncdatadev/superset-operator/api/v1alpha1"
	"github.com/zncdatadev/superset-operator/internal/controller/common"
)

var _ reconciler.RoleReconciler = &Reconciler{}

type Reconciler struct {
	reconciler.BaseRoleReconciler[*supersetv1alpha1.NodeSpec]
	ClusterConfig *supersetv1alpha1.ClusterConfigSpec
	Image         *util.Image
}

func NewReconciler(
	client *resourceClient.Client,
	clusterStopped bool,
	clusterConfig *supersetv1alpha1.ClusterConfigSpec,
	roleInfo reconciler.RoleInfo,
	image *util.Image,
	spec *supersetv1alpha1.NodeSpec,
) *Reconciler {
	return &Reconciler{
		BaseRoleReconciler: *reconciler.NewBaseRoleReconciler(
			client,
			clusterStopped,
			roleInfo,
			spec,
		),
		ClusterConfig: clusterConfig,
		Image:         image,
	}
}

func (r *Reconciler) RegisterResources(ctx context.Context) error {
	for name, rg := range r.Spec.RoleGroups {

		mergedConfig, err := util.MergeObject(r.Spec.Config, rg.Config)
		if err != nil {
			return err
		}
		overrides, err := util.MergeObject(r.Spec.OverridesSpec, rg.OverridesSpec)
		if err != nil {
			return err
		}

		info := reconciler.RoleGroupInfo{
			RoleInfo:      r.RoleInfo,
			RoleGroupName: name,
		}

		var roleGroupConfig *commonsv1alpha1.RoleGroupConfigSpec
		if mergedConfig != nil {
			roleGroupConfig = mergedConfig.RoleGroupConfigSpec
		}
		reconcilers, err := r.RegisterResourceWithRoleGroup(
			ctx,
			rg.Replicas,
			info,
			overrides,
			roleGroupConfig,
		)

		if err != nil {
			return err
		}

		for _, reconciler := range reconcilers {
			r.AddResource(reconciler)
		}
	}
	return nil
}

func (r *Reconciler) RegisterResourceWithRoleGroup(
	ctx context.Context,
	replicas *int32,
	info reconciler.RoleGroupInfo,
	overrides *commonsv1alpha1.OverridesSpec,
	roleGroupConfig *commonsv1alpha1.RoleGroupConfigSpec,
) ([]reconciler.Reconciler, error) {

	configmapReconciler := common.NewConfigReconciler(
		r.Client,
		r.ClusterConfig,
		info,
	)

	deploymentReconciler, err := NewDeploymentReconciler(
		r.Client,
		info,
		r.ClusterConfig,
		Ports,
		r.Image,
		replicas,
		r.ClusterStopped(),
		overrides,
		roleGroupConfig,
	)
	if err != nil {
		return nil, err
	}

	return []reconciler.Reconciler{configmapReconciler, deploymentReconciler}, nil
}
