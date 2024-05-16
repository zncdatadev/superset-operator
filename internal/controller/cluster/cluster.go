package cluster

import (
	supersetv1alpha1 "github.com/zncdata-labs/superset-operator/api/v1alpha1"
	resourceClient "github.com/zncdata-labs/superset-operator/pkg/client"
	"github.com/zncdata-labs/superset-operator/pkg/reconciler"
	"github.com/zncdata-labs/superset-operator/pkg/util"
)

var _ reconciler.Reconciler = &Reconciler{}

type Reconciler struct {
	reconciler.BaseClusterReconciler[*supersetv1alpha1.SupersetClusterSpec]
	ClusterConfig *supersetv1alpha1.ClusterConfigSpec
}

func (r *Reconciler) RegisterResources() error {
	panic("unimplemented")
}

func NewReconciler(
	client resourceClient.ResourceClient,
	instance *supersetv1alpha1.SupersetCluster,
) *Reconciler {

	image := util.Image{
		Custom:         instance.Spec.Image.Custom,
		Repo:           instance.Spec.Image.Repo,
		KDSVersion:     instance.Spec.Image.KDSVersion,
		ProductVersion: instance.Spec.Image.ProductVersion,
	}

	clusterInfo := reconciler.ClusterInfo{
		Name:             instance.Name,
		Namespace:        instance.Namespace,
		ClusterOperation: instance.Spec.ClusterOperation,
		Image:            image,
	}

	return &Reconciler{
		BaseClusterReconciler: *reconciler.NewBaseClusterReconciler(
			client,
			clusterInfo,
			&instance.Spec,
		),
		ClusterConfig: instance.Spec.ClusterConfig,
	}
}
