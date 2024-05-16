package worker

import (
	"context"

	supersetv1alpha1 "github.com/zncdata-labs/superset-operator/api/v1alpha1"
	apiv1alpha1 "github.com/zncdata-labs/superset-operator/pkg/apis/v1alpha1"
	"github.com/zncdata-labs/superset-operator/pkg/image"
	"github.com/zncdata-labs/superset-operator/pkg/reconciler"
)

var _ reconciler.RoleReconciler = &Reconciler{}

type Reconciler struct {
	reconciler.BaseRoleReconciler[*supersetv1alpha1.WorkerSpec]
	ClusterConfig *supersetv1alpha1.ClusterConfigSpec
}

func (r *Reconciler) RegisterResources(ctx context.Context) error {
	for name, rg := range r.Spec.RoleGroups {
		mergedRoleGroup := rg.DeepCopy()
		r.MergeRoleGroup(&mergedRoleGroup)

		if err := r.RegisterResourceWithRoleGroup(ctx, name, mergedRoleGroup); err != nil {
			return err
		}

	}

	return nil
}

func (r *Reconciler) RegisterResourceWithRoleGroup(
	_ context.Context,
	name string,
	roleGroup *supersetv1alpha1.WorkerRoleGroupSpec,
) error {
	panic("unimplemented")
}

func NewReconciler(
	client reconciler.ResourceClient,
	clusterConfig *supersetv1alpha1.ClusterConfigSpec,
	clusterOperation *apiv1alpha1.ClusterOperationSpec,
	imageSpec image.Image,
	name string,
	spec *supersetv1alpha1.WorkerSpec,

) *Reconciler {

	return &Reconciler{
		BaseRoleReconciler: *reconciler.NewBaseRoleReconciler(
			client,
			name,
			clusterOperation,
			imageSpec,
			spec,
		),
		ClusterConfig: clusterConfig,
	}
}
