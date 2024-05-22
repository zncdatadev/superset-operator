package cluster

import (
	"context"

	supersetv1alpha1 "github.com/zncdatadev/superset-operator/api/v1alpha1"
	"github.com/zncdatadev/superset-operator/internal/controller/common"
	"github.com/zncdatadev/superset-operator/internal/controller/node"
	"github.com/zncdatadev/superset-operator/internal/controller/worker"
	"github.com/zncdatadev/superset-operator/pkg/builder"
	resourceClient "github.com/zncdatadev/superset-operator/pkg/client"
	"github.com/zncdatadev/superset-operator/pkg/reconciler"
	"github.com/zncdatadev/superset-operator/pkg/util"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	logger = ctrl.Log.WithName("controller").WithName("Cluster")
)

var _ reconciler.Reconciler = &Reconciler{}

type Reconciler struct {
	reconciler.BaseClusterReconciler[*supersetv1alpha1.SupersetClusterSpec]
	ClusterConfig *common.ClusterConfig
	Options       *builder.ClusterOptions
}

func (r *Reconciler) RegisterResources(ctx context.Context) error {
	r.AddResource(NewEnvSecretReconciler(r.Client, r.ClusterConfig, r.Options))
	r.AddResource(NewSupersetConfigSecretReconciler(r.Client, r.ClusterConfig, r.Options))
	r.AddResource(NewJobReconciler(r.Client, r.ClusterConfig, r.Options))

	nodeReconciler := node.NewReconciler(
		r.Client,
		r.ClusterConfig,
		&builder.RoleOptions{ClusterOptions: *r.Options, Name: "node"},
		r.Spec.Node,
	)
	if err := nodeReconciler.RegisterResources(ctx); err != nil {
		return err
	}

	r.AddResource(nodeReconciler)

	workerReconciler := worker.NewReconciler(
		r.Client,
		r.ClusterConfig,
		&builder.RoleOptions{ClusterOptions: *r.Options, Name: "worker"},
		r.Spec.Worker,
	)

	if err := workerReconciler.RegisterResources(ctx); err != nil {
		return err
	}
	r.AddResource(workerReconciler)

	return nil
}

func NewReconciler(
	client *resourceClient.Client,
	instance *supersetv1alpha1.SupersetCluster,
) *Reconciler {
	image := &util.Image{
		Custom:         instance.Spec.Image.Custom,
		Repo:           instance.Spec.Image.Repo,
		KDSVersion:     instance.Spec.Image.KDSVersion,
		ProductVersion: instance.Spec.Image.ProductVersion,
	}

	clusterOptions := &builder.ClusterOptions{
		Name:      instance.Name,
		Namespace: instance.Namespace,
		Labels: map[string]string{
			"app.kubernetes.io/name":       "hbase",
			"app.kubernetes.io/managed-by": "hbase.zncdata.dev",
			"app.kubernetes.io/instance":   instance.Name,
		},
		Annotations: instance.GetAnnotations(),

		ClusterOperation: instance.Spec.ClusterOperation,
		Image:            image,
	}

	clusterConfig := &common.ClusterConfig{
		EnvSecretName:    instance.Name + "-env",
		ConfigSecretName: instance.Name + "-config",
		Spec:             instance.Spec.ClusterConfig,
	}

	return &Reconciler{
		BaseClusterReconciler: *reconciler.NewBaseClusterReconciler(
			client,
			clusterOptions,
			&instance.Spec,
		),
		Options:       clusterOptions,
		ClusterConfig: clusterConfig,
	}
}
