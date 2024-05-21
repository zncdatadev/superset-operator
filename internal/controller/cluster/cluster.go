package cluster

import (
	"context"

	supersetv1alpha1 "github.com/zncdatadev/superset-operator/api/v1alpha1"
	"github.com/zncdatadev/superset-operator/internal/controller/common"
	resourceClient "github.com/zncdatadev/superset-operator/pkg/client"
	"github.com/zncdatadev/superset-operator/pkg/reconciler"
	"github.com/zncdatadev/superset-operator/pkg/util"
)

var _ reconciler.Reconciler = &Reconciler{}

type Reconciler struct {
	reconciler.BaseClusterReconciler[*supersetv1alpha1.SupersetClusterSpec]
	ClusterConfig *common.ClusterConfig
}

func (r *Reconciler) RegisterResources(_ context.Context) error {
	r.AddResource(NewJobReconciler(r.Client, r.ClusterInfo, r.ClusterConfig))
	r.AddResource(NewEnvSecretReconciler(r.Client, r.ClusterConfig))
	r.AddResource(NewSupersetConfigSecretReconciler(r.Client, r.ClusterConfig))
	return nil
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

	clusterInfo := &reconciler.ClusterInfo{
		Name:             instance.Name,
		Namespace:        instance.Namespace,
		ClusterOperation: instance.Spec.ClusterOperation,
		Image:            image,
	}

	clusterConfig := &common.ClusterConfig{
		EnvSecretName:    instance.Name + "env",
		ConfigSecretName: instance.Name + "config",
		Spec:             instance.Spec.ClusterConfig,
	}

	return &Reconciler{
		BaseClusterReconciler: *reconciler.NewBaseClusterReconciler(
			client,
			clusterInfo,
			&instance.Spec,
		),
		ClusterConfig: clusterConfig,
	}
}
