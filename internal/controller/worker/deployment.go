package worker

import (
	"context"
	supersetv1alpha1 "github.com/zncdatadev/superset-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
	spec *supersetv1alpha1.WorkerConfigSpec,
) *reconciler.DeploymentReconciler {
	deploymentBuilder := NewDeploymentBuilder(
		client,
		clusterConfig,
		options,
	)

	var specAffinity *corev1.Affinity
	if spec != nil {
		specAffinity = spec.Affinity
	}
	addAffinityToStatefulSetBuilder(deploymentBuilder, specAffinity, options.RoleOptions.ClusterOptions.Name,
		options.RoleOptions.Name)

	return reconciler.NewDeploymentReconciler(
		client,
		options,
		deploymentBuilder,
	)
}

func addAffinityToStatefulSetBuilder(objectBuilder *DeploymentBuilder, specAffinity *corev1.Affinity,
	instanceName string, roleName string) {
	antiAffinityLabels := metav1.LabelSelector{
		MatchLabels: map[string]string{
			reconciler.LabelInstance:  instanceName,
			reconciler.LabelServer:    "superset",
			reconciler.LabelComponent: roleName,
		},
	}
	defaultAffinityBuilder := builder.AffinityBuilder{PodAffinity: []*builder.PodAffinity{
		builder.NewPodAffinity(builder.StrengthPrefer, true, antiAffinityLabels).Weight(70),
	}}

	objectBuilder.Affinity(specAffinity, defaultAffinityBuilder.Build())
}
