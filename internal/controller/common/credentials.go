package common

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/zncdatadev/operator-go/pkg/builder"
)

var (
	credentialsKeyMapping = [][]string{
		{"ADMIN_USERNAME", "adminUser.username"},
		{"ADMIN_FIRSTNAME", "adminUser.firstname"},
		{"ADMIN_LASTNAME", "adminUser.lastname"},
		{"ADMIN_EMAIL", "adminUser.email"},
		{"ADMIN_PASSWORD", "adminUser.password"},
		{"SECRET_KEY", "appSecretKey"},
		{"SQLALCHEMY_DATABASE_URI", "connections.sqlalchemyDatabaseUri"},
	}
)

func InjectCredentials(credentialsSecret string, builder builder.ContainerBuilder) {
	envvars := []corev1.EnvVar{}
	for _, pair := range credentialsKeyMapping {
		envvars = append(envvars, corev1.EnvVar{
			Name: pair[0],
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					Key: pair[1],
					LocalObjectReference: corev1.LocalObjectReference{
						Name: credentialsSecret,
					},
				},
			},
		})
	}

	builder.AddEnvVars(envvars)
}
