package cluster

import (
	"context"

	supersetv1alpha1 "github.com/zncdata-labs/superset-operator/api/v1alpha1"
	"github.com/zncdata-labs/superset-operator/internal/controller/node"
	"github.com/zncdata-labs/superset-operator/internal/controller/worker"
	apiv1alpha1 "github.com/zncdata-labs/superset-operator/pkg/apis/v1alpha1"
	"github.com/zncdata-labs/superset-operator/pkg/image"
	"github.com/zncdata-labs/superset-operator/pkg/reconciler"
)

type Reconciler struct {
	reconciler.BaseClusterReconciler[*supersetv1alpha1.SupersetClusterSpec]
	ClusterOperation *apiv1alpha1.ClusterOperationSpec
	ClusterConfig    *supersetv1alpha1.ClusterConfigSpec
	Image            image.Image
}

func (r *Reconciler) RegisterResources(_ context.Context) error {

	node := node.NewReconciler(
		r.Client,
		r.ClusterConfig,
		r.ClusterOperation,
		r.Image,
		"node",
		r.Spec.Node,
	)
	r.AddResource(node)

	worker := worker.NewReconciler(
		r.Client,
		r.ClusterConfig,
		r.ClusterOperation,
		r.Image,
		"worker",
		r.Spec.Worker,
	)
	r.AddResource(worker)

	return nil
}

func NewReconciler(
	client reconciler.ResourceClient,
	cluster *supersetv1alpha1.SupersetCluster,
) *Reconciler {

	image := image.Image{
		Repo:           cluster.Spec.Image.Repo,
		Custom:         cluster.Spec.Image.Custom,
		KDSVersion:     cluster.Spec.Image.KDSVersion,
		ProductVersion: cluster.Spec.Image.ProductVersion,
	}

	return &Reconciler{
		BaseClusterReconciler: *reconciler.NewBaseClusterReconciler(
			client,
			client.GetOwnerName(),
			&cluster.Spec,
		),
		ClusterOperation: cluster.Spec.ClusterOperation,
		ClusterConfig:    cluster.Spec.ClusterConfig,
		Image:            image,
	}
}
