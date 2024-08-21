package common

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/zncdatadev/operator-go/pkg/builder"
)

var (
	configMapKey = map[string]string{
		"ADMIN_USERNAME":  "adminUser.username",
		"ADMIN_FIRSTNAME": "adminUser.firstname",
		"ADMIN_LASTNAME":  "adminUser.lastname",
		"ADMIN_EMAIL":     "adminUser.email",
		"ADMIN_PASSWORD":  "adminUser.password",
		"SECRET_KEY":      "appSecretKey",
		// "SQLALCHEMY_DATABASE_URI": "connections.sqlalchemyDatabaseUri",
	}
)

func InjectCredentials(credentialsSecret string, builder builder.ContainerBuilder) {
	envvars := []corev1.EnvVar{}
	for key, value := range configMapKey {
		envvars = append(envvars, corev1.EnvVar{
			Name: key,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					Key: value,
					LocalObjectReference: corev1.LocalObjectReference{
						Name: credentialsSecret,
					},
				},
			},
		})
	}

	builder.AddEnvVars(envvars)
	builder.AddEnvVar(&corev1.EnvVar{
		Name:  "SQLALCHEMY_DATABASE_URI",
		Value: "postgresql+psycopg2://superset:superset@192.168.205.1:5432/superset",
	})
}
