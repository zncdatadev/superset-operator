package cluster

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/zncdatadev/superset-operator/internal/controller/common"
	"github.com/zncdatadev/superset-operator/pkg/builder"
	"github.com/zncdatadev/superset-operator/pkg/client"
	"github.com/zncdatadev/superset-operator/pkg/reconciler"
	"github.com/zncdatadev/superset-operator/pkg/util"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ builder.ConfigBuilder = &EnvSecretBuilder{}

type EnvSecretBuilder struct {
	builder.SecretBuilder
	ClusterConfig *common.ClusterConfig
}

func NewEnvSecretBuilder(
	client *client.Client,
	clusterConfig *common.ClusterConfig,
	options builder.Options,
) *EnvSecretBuilder {
	return &EnvSecretBuilder{
		SecretBuilder: *builder.NewSecretBuilder(
			client,
			options,
		),
		ClusterConfig: clusterConfig,
	}
}

func (b *EnvSecretBuilder) getRedisConfig(ctx context.Context) (map[string]string, error) {
	env := make(map[string]string)
	if b.ClusterConfig.Spec == nil && b.ClusterConfig.Spec.Redis == nil {
		return nil, errors.New("redis config in clusterConfig is Required")
	}

	redisSpec := b.ClusterConfig.Spec.Redis

	ns := b.Client.GetOwnerNamespace()

	if redisSpec.ExistSecret != "" {
		secretObj := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: redisSpec.ExistSecret,
			},
		}
		if err := b.Client.Get(ctx, secretObj); err != nil {
			return nil, err
		}
		bytes, ok := secretObj.Data["password"]
		if !ok {
			return nil, fmt.Errorf("password not found in secret: %s, namespace: %s",
				redisSpec.ExistSecret,
				ns,
			)
		}
		env["REDIS_PASSWORD"] = string(bytes)
	}

	if redisSpec.User != "" {
		env["REDIS_USER"] = redisSpec.User
	}

	if redisSpec.Host == "" {
		return nil, fmt.Errorf(
			"redis host is required, cluster: %s, namespace: %s",
			b.Options.GetName(),
			ns,
		)
	}

	env["REDIS_HOST"] = redisSpec.Host
	env["REDIS_PORT"] = fmt.Sprintf("%d", redisSpec.Port)
	env["REDIS_DB"] = fmt.Sprintf("%d", redisSpec.DB)
	env["REDIS_PROTO"] = redisSpec.Proto

	return env, nil
}

func (b *EnvSecretBuilder) getDBConfig(ctx context.Context) (map[string]string, error) {
	env := make(map[string]string)
	if b.ClusterConfig.Spec == nil && b.ClusterConfig.Spec.Database == nil {
		return nil, errors.New("database config in clusterConfig is Required")
	}

	dbSpec := b.ClusterConfig.Spec.Database

	var dbInline *client.DatabaseParams

	if dbSpec.Inline != nil {
		dbInline = client.NewDatabaseParams(
			dbSpec.Inline.Driver,
			dbSpec.Inline.Username,
			dbSpec.Inline.Password,
			dbSpec.Inline.Host,
			dbSpec.Inline.Port,
			dbSpec.Inline.DatabaseName,
		)
	}

	dbConfig := client.DatabaseConfiguration{
		Client:      b.Client,
		Context:     ctx,
		DbReference: dbSpec.Reference,
		DbInline:    dbInline,
	}

	dbParams, err := dbConfig.GetDatabaseParams()
	if err != nil {
		return nil, err
	}

	env["DB_DRIVER"] = dbParams.Driver
	env["DB_HOST"] = dbParams.Host
	env["DB_PORT"] = fmt.Sprintf("%d", dbParams.Port)
	env["DB_NAME"] = dbParams.DbName
	env["DB_USER"] = dbParams.Username
	env["DB_PASS"] = dbParams.Password

	return env, nil
}

// GetAdminInfoFromSecret gets the admin info from the secret.
// If the secret is not set, it will return the admin info from the cluster config.
// If the secret is set, it will check the secret data.
func (b *EnvSecretBuilder) GetAdminInfoFromSecret(ctx context.Context) (map[string]string, error) {
	adminSpec := b.ClusterConfig.Spec.Administrator
	if adminSpec.ExistSecret == "" {
		return map[string]string{
			"ADMIN_USER":      adminSpec.Username,
			"ADMIN_FIRSTNAME": adminSpec.FirstName,
			"ADMIN_LASTNAME":  adminSpec.LastName,
			"ADMIN_EMAIL":     adminSpec.Email,
			"ADMIN_PASSWORD":  adminSpec.Password,
		}, nil
	}

	// exist secret found, use first

	secretObj := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: adminSpec.ExistSecret,
		},
	}

	if err := b.Client.Client.Get(ctx, ctrlclient.ObjectKey{Namespace: b.Client.GetOwnerNamespace(), Name: adminSpec.ExistSecret}, secretObj); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("secret not found, secret: %s, namespace: %s", secretObj.Name, b.Client.GetOwnerNamespace())
		} else {
			return nil, err
		}
	}

	if secretObj.Data == nil {
		return nil, fmt.Errorf("secret data is empty, secret: %s, namespace: %s", secretObj.Name, b.Client.GetOwnerNamespace())
	}

	if _, ok := secretObj.Data["ADMIN_USER"]; !ok {
		return nil, fmt.Errorf("username not found in secret: %s, namespace: %s", secretObj.Name, b.Client.GetOwnerNamespace())
	}

	if _, ok := secretObj.Data["ADMIN_PASSWORD"]; !ok {
		return nil, fmt.Errorf("password not found in secret: %s, namespace: %s", secretObj.Name, b.Client.GetOwnerNamespace())
	}

	logger.V(1).Info("Get admin info from secret, and checkd, it will mount to container deriectly", "namespace", b.Client.GetOwnerNamespace(), "secret", secretObj.Name)
	return nil, nil
}

// getFlaskSecretKey generates a secret key for flask app.
func (b *EnvSecretBuilder) getFlaskSecretKey() (map[string]string, error) {
	var key string
	appSecretKeySpec := b.ClusterConfig.Spec.AppSecretKey
	if appSecretKeySpec == nil {
		key = b.getRandomString(42)
	} else if appSecretKeySpec.ExistSecret != "" {
		secretObj := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: appSecretKeySpec.ExistSecret,
			},
		}
		if err := b.Client.Get(context.Background(), secretObj); err != nil {
			return nil, err
		}
		_, ok := secretObj.Data["SUPERSET_SECRET_KEY"]
		if !ok {
			return nil, fmt.Errorf("secret key not found in secret: %s, namespace: %s",
				appSecretKeySpec.ExistSecret,
				b.Client.GetOwnerNamespace(),
			)
		}
		logger.V(1).Info("Get flask secret key from secret, and checked, it will mount to container deriectly", "namespace", b.Client.GetOwnerNamespace(), "secret", secretObj.Name)
		return nil, nil
	} else if appSecretKeySpec.SecretKey != "" {
		key = appSecretKeySpec.SecretKey
	} else {
		key = b.getRandomString(42)
	}

	return map[string]string{
		"SUPERSET_SECRET_KEY": key,
	}, nil

}

// The secret key is generated by the owner reference UID.
// TODO: maybe we can use a more secure way to generate the secret key.
func (b *EnvSecretBuilder) getRandomString(length int) string {
	uid := b.Client.GetOwnerReference().GetUID()
	data := base64.StdEncoding.EncodeToString([]byte(uid))
	if len(data) > length {
		return data[:length]
	}
	return data
}

func (b *EnvSecretBuilder) Build(ctx context.Context) (ctrlclient.Object, error) {

	adminConfig, err := b.GetAdminInfoFromSecret(ctx)
	if err != nil {
		return nil, err
	}
	b.AddData(adminConfig)

	redisConfig, err := b.getRedisConfig(ctx)
	if err != nil {
		return nil, err
	}
	b.AddData(redisConfig)

	dbConfig, err := b.getDBConfig(ctx)
	if err != nil {
		return nil, err
	}
	b.AddData(dbConfig)

	flaskSecretKeyData, err := b.getFlaskSecretKey()
	if err != nil {
		return nil, err
	}
	b.AddData(flaskSecretKeyData)

	b.SetName(b.ClusterConfig.EnvSecretName)
	return b.GetObject(), nil
}

type SupersetConfigSecretBuilder struct {
	builder.SecretBuilder
	ClusterConfig *common.ClusterConfig
}

func NewSupersetConfigSecretBuilder(
	client *client.Client,
	clusterConfig *common.ClusterConfig,
	options builder.Options,
) *SupersetConfigSecretBuilder {
	return &SupersetConfigSecretBuilder{
		SecretBuilder: *builder.NewSecretBuilder(
			client,
			options,
		),
		ClusterConfig: clusterConfig,
	}
}

func (b *SupersetConfigSecretBuilder) getConfig() string {
	// Attention:
	// Python intends to use 4 spaces per indentation level.
	// We define the config in golang use tab, so we need to convert it to 4 spaces finally.
	const config = `
import os
from flask_caching.backends.rediscache import RedisCache

def env(key, default=None):
	return os.getenv(key, default)
  
# Redis Base URL
REDIS_BASE_URL=f"{env('REDIS_PROTO')}://{env('REDIS_USER', '')}:{env('REDIS_PASSWORD')}@{env('REDIS_HOST')}:{env('REDIS_PORT')}"

# Redis URL Params
REDIS_URL_PARAMS = ""

# Build Redis URLs
CACHE_REDIS_URL = f"{REDIS_BASE_URL}/{env('REDIS_DB', 1)}{REDIS_URL_PARAMS}"
CELERY_REDIS_URL = f"{REDIS_BASE_URL}/{env('REDIS_CELERY_DB', 0)}{REDIS_URL_PARAMS}"

MAPBOX_API_KEY = env('MAPBOX_API_KEY', '')
CACHE_CONFIG = {
	'CACHE_TYPE': 'RedisCache',
	'CACHE_DEFAULT_TIMEOUT': 300,
	'CACHE_KEY_PREFIX': 'superset_',
	'CACHE_REDIS_URL': CACHE_REDIS_URL,
}
DATA_CACHE_CONFIG = CACHE_CONFIG

SQLALCHEMY_DATABASE_URI = f"postgresql+psycopg2://{env('DB_USER')}:{env('DB_PASS')}@{env('DB_HOST')}:{env('DB_PORT')}/{env('DB_NAME')}"
SQLALCHEMY_TRACK_MODIFICATIONS = True
class CeleryConfig:
  imports  = ("superset.sql_lab", )
  broker_url = CELERY_REDIS_URL
  result_backend = CELERY_REDIS_URL

CELERY_CONFIG = CeleryConfig
RESULTS_BACKEND = RedisCache(
      host=env('REDIS_HOST'),
      password=env('REDIS_PASSWORD'),
      port=env('REDIS_PORT'),
      key_prefix='superset_results',
)
`

	return util.IndentTab4Spaces(config)
}

func (b *SupersetConfigSecretBuilder) getSupersetInit() string {
	const init = `
#!/bin/sh
set -eu
echo "Upgrading DB schema..."
superset db upgrade
echo "Initializing roles..."
superset init

echo "Creating admin user..."
superset fab create-admin \
                --username ${ADMIN_USER} \
                --firstname ${ADMIN_FIRSTNAME:-Superset} \
                --lastname ${ADMIN_LASTNAME:-Admin} \
                --email ${ADMIN_EMAIL:-admin@superset.com} \
                --password ${ADMIN_PASSWORD} \
                || true

if [ -f "/app/configs/import_datasources.yaml" ]; then
  echo "Importing database connections.... "
  superset import_datasources -p /app/configs/import_datasources.yaml
fi
`

	return util.IndentTab4Spaces(init)

}

func (b *SupersetConfigSecretBuilder) getSupersetBootstrap() string {
	const bootstrap = `
#!/bin/bash
if [ ! -f ~/bootstrap ]; then echo "Running Superset with uid 0" > ~/bootstrap; fi
`

	return util.IndentTab4Spaces(bootstrap)
}

func (b *SupersetConfigSecretBuilder) Build(_ context.Context) (ctrlclient.Object, error) {
	var config = map[string]string{
		"superset_config.py":    b.getConfig(),
		"superset_init.sh":      b.getSupersetInit(),
		"superset_bootstrap.sh": b.getSupersetBootstrap(),
	}
	b.AddData(config)
	b.SetName(b.ClusterConfig.ConfigSecretName)
	return b.GetObject(), nil
}

func NewEnvSecretReconciler(
	client *client.Client,
	clusterConfig *common.ClusterConfig,
	options builder.Options,
) *reconciler.SimpleResourceReconciler[builder.ConfigBuilder] {

	envSecretBuilder := NewEnvSecretBuilder(
		client,
		clusterConfig,
		options,
	)

	return reconciler.NewSimpleResourceReconciler[builder.ConfigBuilder](
		client,
		options,
		envSecretBuilder,
	)

}

func NewSupersetConfigSecretReconciler(
	client *client.Client,
	clusterConfig *common.ClusterConfig,
	options builder.Options,
) *reconciler.SimpleResourceReconciler[builder.ConfigBuilder] {

	supersetConfigSecretBuilder := NewSupersetConfigSecretBuilder(
		client,
		clusterConfig,
		options,
	)

	return reconciler.NewSimpleResourceReconciler[builder.ConfigBuilder](
		client,
		options,
		supersetConfigSecretBuilder,
	)
}
