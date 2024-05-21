package node

import (
	"context"

	supersetv1alpha1 "github.com/zncdata-labs/superset-operator/api/v1alpha1"
	"github.com/zncdata-labs/superset-operator/internal/controller/common"
	resourceClient "github.com/zncdata-labs/superset-operator/pkg/client"
	"github.com/zncdata-labs/superset-operator/pkg/reconciler"
)

var _ reconciler.RoleReconciler = &Reconciler{}

type Reconciler struct {
	reconciler.BaseRoleReconciler[*supersetv1alpha1.NodeSpec]
	ClusterConfig *common.ClusterConfig
}

func (r *Reconciler) RegisterResources(ctx context.Context) error {
	for name, rg := range r.Spec.RoleGroups {
		mergedRoleGroup := rg.DeepCopy()
		r.MergeRoleGroupSpec(&mergedRoleGroup)

		if err := r.RegisterResourceWithRoleGroup(ctx, name, mergedRoleGroup); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reconciler) RegisterResourceWithRoleGroup(
	_ context.Context,
	name string,
	roleGroup *supersetv1alpha1.NodeRoleGroupSpec,
) error {

	roleGroupInfo := &reconciler.RoleGroupInfo{
		RoleInfo:            *r.RoleInfo,
		Name:                name,
		Replicas:            roleGroup.Replicas,
		PodDisruptionBudget: roleGroup.PodDisruptionBudget,
		CommandOverrides:    roleGroup.CommandOverrides,
		EnvOverrides:        roleGroup.EnvOverrides,
		//PodOverrides:        roleGroup.PodOverrides,	TODO: Uncomment this line
	}

	service := reconciler.NewServiceReconciler(
		r.Client,
		roleGroupInfo.GetFullName(),
		Ports,
	)
	r.AddResource(service)

	deployment := NewDeploymentReconciler(
		r.Client,
		r.ClusterConfig,
		roleGroupInfo,
		Ports,
		roleGroup,
	)
	r.AddResource(deployment)
	return nil
}

func NewReconciler(
	client resourceClient.ResourceClient,
	roleInfo *reconciler.RoleInfo,
	clusterConfig *common.ClusterConfig,
	spec *supersetv1alpha1.NodeSpec,
) *Reconciler {
	return &Reconciler{
		BaseRoleReconciler: *reconciler.NewBaseRoleReconciler(
			client,
			roleInfo,
			spec,
		),
		ClusterConfig: clusterConfig,
	}
}
