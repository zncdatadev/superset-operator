package worker

import (
	"context"

	supersetv1alpha1 "github.com/zncdatadev/superset-operator/api/v1alpha1"
	"github.com/zncdatadev/superset-operator/internal/controller/common"
	resourceClient "github.com/zncdatadev/superset-operator/pkg/client"
	"github.com/zncdatadev/superset-operator/pkg/reconciler"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type DeploymentBuilder struct {
	common.DeploymentBuilder
}

func NewDeploymentBuilder(
	client resourceClient.ResourceClient,
	clusterConfig *common.ClusterConfig,
	roleGroupInfo *reconciler.RoleGroupInfo,
	ports []corev1.ContainerPort,
	envOverrides map[string]string,
	commandOverrides []string,
) *DeploymentBuilder {
	deploymentBuilder := common.NewDeploymentBuilder(
		client,
		clusterConfig,
		roleGroupInfo,
		ports,
		envOverrides,
		commandOverrides,
	)
	return &DeploymentBuilder{
		DeploymentBuilder: *deploymentBuilder,
	}
}

func (b *DeploymentBuilder) Build(ctx context.Context) (client.Object, error) {
	_, err := b.DeploymentBuilder.Build(ctx)
	if err != nil {
		return nil, err
	}

	mainContainer := b.DeploymentBuilder.GetMainContainer().
		SetCommand([]string{"/bin/sh", "-c", ". /app/pythonpath/superset_bootstrap.sh; celery --app=superset.tasks.celery_app:app worker"}).
		SetLiveProbe(&corev1.Probe{
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
		}).
		Build()

	b.ResetContainers([]corev1.Container{*mainContainer})

	return b.DeploymentBuilder.Build(ctx)
}

func NewDeploymentReconciler(
	client resourceClient.ResourceClient,
	clusterConfig *common.ClusterConfig,
	roleGroupInfo *reconciler.RoleGroupInfo,
	ports []corev1.ContainerPort,
	spec *supersetv1alpha1.WorkerRoleGroupSpec,
) *reconciler.DeploymentReconciler {
	deploymentBuilder := NewDeploymentBuilder(
		client,
		clusterConfig,
		roleGroupInfo,
		ports,
		spec.EnvOverrides,
		spec.CommandOverrides,
	)

	return reconciler.NewDeploymentReconciler(
		client,
		roleGroupInfo,
		ports,
		deploymentBuilder,
	)
}
