package worker

import (
	"context"

	"github.com/zncdatadev/superset-operator/internal/controller/common"
	"github.com/zncdatadev/superset-operator/pkg/builder"
	"github.com/zncdatadev/superset-operator/pkg/client"
	"github.com/zncdatadev/superset-operator/pkg/reconciler"
	corev1 "k8s.io/api/core/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type DeploymentBuilder struct {
	common.DeploymentBuilder
}

func NewDeploymentBuilder(
	client *client.Client,
	clusterConfig *common.ClusterConfig,
	options *builder.RoleGroupOptions,
) *DeploymentBuilder {
	deploymentBuilder := common.NewDeploymentBuilder(
		client,
		clusterConfig,
		options,
	)
	return &DeploymentBuilder{
		DeploymentBuilder: *deploymentBuilder,
	}
}

func (b *DeploymentBuilder) Build(ctx context.Context) (ctrlclient.Object, error) {
	_, err := b.DeploymentBuilder.Build(ctx)
	if err != nil {
		return nil, err
	}

	mainContainerBuilder := b.DeploymentBuilder.GetMainContainer()
	mainContainerBuilder.SetCommand([]string{"/bin/sh", "-c", ". /app/pythonpath/superset_bootstrap.sh; celery --app=superset.tasks.celery_app:app worker"})
	mainContainerBuilder.SetLiveProbe(&corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: []string{"sh", "-c", "celery -A superset.tasks.celery_app:app inspect ping -d celery@$HOSTNAME"},
			},
		},
		FailureThreshold:    3,
		InitialDelaySeconds: 120,
		PeriodSeconds:       60,
		SuccessThreshold:    1,
		TimeoutSeconds:      60,
	})
	mainContainer := mainContainerBuilder.Build()

	b.ResetContainers([]corev1.Container{*mainContainer})

	return b.GetObject(), nil
}

func NewDeploymentReconciler(
	client *client.Client,
	clusterConfig *common.ClusterConfig,
	options *builder.RoleGroupOptions,
) *reconciler.DeploymentReconciler {
	deploymentBuilder := NewDeploymentBuilder(
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
