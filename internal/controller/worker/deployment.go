package worker

import (
	"context"
	"errors"
	"time"

	supersetv1alpha1 "github.com/zncdatadev/superset-operator/api/v1alpha1"

	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/util"
	"github.com/zncdatadev/superset-operator/internal/controller/common"
	corev1 "k8s.io/api/core/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type DeploymentBuilder struct {
	common.DeploymentBuilder
}

func NewDeploymentBuilder(
	client *client.Client,
	name string,
	clusterConfig *supersetv1alpha1.ClusterConfigSpec,
	envSecretName string,
	configSecretName string,
	replicas *int32,
	ports []corev1.ContainerPort,
	image *util.Image,
	options builder.WorkloadOptions,
) *DeploymentBuilder {
	return &DeploymentBuilder{
		DeploymentBuilder: *common.NewDeploymentBuilder(
			client,
			name,
			clusterConfig,
			envSecretName,
			configSecretName,
			replicas,
			ports,
			image,
			options,
		),
	}
}

func (b *DeploymentBuilder) Build(ctx context.Context) (ctrlclient.Object, error) {
	_, err := b.DeploymentBuilder.Build(ctx)
	if err != nil {
		return nil, err
	}

	mainContainerBuilder := b.DeploymentBuilder.GetMainContainer()
	mainContainerBuilder.SetCommand([]string{"/bin/sh", "-c", ". /app/pythonpath/superset_bootstrap.sh; celery --app=superset.tasks.celery_app:app worker"})
	mainContainerBuilder.SetLivenessProbe(&corev1.Probe{
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

	return b.GetObject()
}

func NewDeploymentReconciler(
	client *client.Client,
	roleGroupInfo reconciler.RoleGroupInfo,
	clusterConfig *supersetv1alpha1.ClusterConfigSpec,
	envSecretName string,
	configSecretName string,
	ports []corev1.ContainerPort,
	image *util.Image,
	stopped bool,
	spec *supersetv1alpha1.WorkerRoleGroupSpec,
) (*reconciler.Deployment, error) {

	options := builder.WorkloadOptions{
		Options: builder.Options{
			ClusterName:   roleGroupInfo.ClusterName,
			RoleName:      roleGroupInfo.RoleName,
			RoleGroupName: roleGroupInfo.RoleGroupName,
			Labels:        roleGroupInfo.GetLabels(),
			Annotations:   roleGroupInfo.GetAnnotations(),
		},
		PodOverrides:     spec.PodOverride,
		EnvOverrides:     spec.EnvOverrides,
		CommandOverrides: spec.CommandOverrides,
		Resource:         spec.Config.Resources,
	}

	if spec.Config != nil {

		var gracefulShutdownTimeout time.Duration
		var err error

		if spec.Config.GracefulShutdownTimeout != "" {
			gracefulShutdownTimeout, err = time.ParseDuration(spec.Config.GracefulShutdownTimeout)

			if err != nil {
				return nil, errors.New("failed to parse graceful shutdown")
			}
		}

		options.TerminationGracePeriod = &gracefulShutdownTimeout

		options.Affinity = spec.Config.Affinity
	}

	deploymentBuilder := common.NewDeploymentBuilder(
		client,
		roleGroupInfo.GetFullName(),
		clusterConfig,
		envSecretName,
		configSecretName,
		spec.Replicas,
		ports,
		image,
		options,
	)

	return reconciler.NewDeployment(
		client,
		roleGroupInfo.GetFullName(),
		deploymentBuilder,
		stopped,
	), nil
}

// func addAffinityToStatefulSetBuilder(objectBuilder *DeploymentBuilder, specAffinity *corev1.Affinity,
// 	instanceName string, roleName string) {
// 	antiAffinityLabels := metav1.LabelSelector{
// 		MatchLabels: map[string]string{
// 			reconciler.LabelInstance:  instanceName,
// 			reconciler.LabelServer:    "superset",
// 			reconciler.LabelComponent: roleName,
// 		},
// 	}
// 	defaultAffinityBuilder := builder.AffinityBuilder{PodAffinity: []*builder.PodAffinity{
// 		builder.NewPodAffinity(builder.StrengthPrefer, true, antiAffinityLabels).Weight(70),
// 	}}

// 	objectBuilder.Affinity(specAffinity, defaultAffinityBuilder.Build())
// }
