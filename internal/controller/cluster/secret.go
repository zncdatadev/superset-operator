package cluster

import (
	"context"
	"errors"
	"fmt"

	"github.com/zncdata-labs/superset-operator/internal/controller/common"
	"github.com/zncdata-labs/superset-operator/pkg/builder"
	resourceClient "github.com/zncdata-labs/superset-operator/pkg/client"
	"github.com/zncdata-labs/superset-operator/pkg/reconciler"
	"github.com/zncdata-labs/superset-operator/pkg/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ builder.ConfigBuilder = &EnvSecretBuilder{}

type EnvSecretBuilder struct {
	builder.SecretBuilder
	ClusterConfig *common.ClusterConfig
}

func NewEnvSecretBuilder(
	client resourceClient.ResourceClient,
	clusterConfig *common.ClusterConfig,
) *EnvSecretBuilder {
	return &EnvSecretBuilder{
		SecretBuilder: *builder.NewSecretBuilder(
			client,
			clusterConfig.EnvSecretName,
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
			b.Name,
			ns,
		)
	}

	env["REDIS_HOST"] = redisSpec.Host
	env["REDIS_PORT"] = fmt.Sprintf("%d", redisSpec.Port)
	env["REDIS_DB"] = fmt.Sprintf("%d", redisSpec.DB)
	env["REDIS_PROTOCOL"] = redisSpec.Proto

	return env, nil
}

func (b *EnvSecretBuilder) getDBConfig(ctx context.Context) (map[string]string, error) {
	env := make(map[string]string)
	if b.ClusterConfig.Spec == nil && b.ClusterConfig.Spec.Database == nil {
		return nil, errors.New("database config in clusterConfig is Required")
	}

	dbSpec := b.ClusterConfig.Spec.Database

	var dbInline *resourceClient.DatabaseParams

	if dbSpec.Inline != nil {
		dbInline = resourceClient.NewDatabaseParams(
			dbSpec.Inline.Driver,
			dbSpec.Inline.Username,
			dbSpec.Inline.Password,
			dbSpec.Inline.Host,
			dbSpec.Inline.Port,
			dbSpec.Inline.DatabaseName,
		)
	}

	dbConfig := resourceClient.DatabaseConfiguration{
		Client:      b.Client,
		Context:     ctx,
		DbReference: &dbSpec.Reference,
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

func (b *EnvSecretBuilder) Build(ctx context.Context) (ctrlclient.Object, error) {
	redisConfig, err := b.getRedisConfig(ctx)
	if err != nil {
		return nil, err
	}

	dbConfig, err := b.getDBConfig(ctx)
	if err != nil {
		return nil, err
	}
	b.AddData(redisConfig)
	b.AddData(dbConfig)
	return b.GetObject(), nil
}

type SupersetConfigSecretBuilder struct {
	builder.SecretBuilder
	ClusterConfig *common.ClusterConfig
}

func NewSupersetConfigSecretBuilder(
	client resourceClient.ResourceClient,
	clusterConfig *common.ClusterConfig,
) *SupersetConfigSecretBuilder {
	return &SupersetConfigSecretBuilder{
		SecretBuilder: *builder.NewSecretBuilder(
			client,
			clusterConfig.ConfigSecretName,
		),
		ClusterConfig: clusterConfig,
	}
}

func (b *SupersetConfigSecretBuilder) Build(ctx context.Context) (ctrlclient.Object, error) {
	return b.GetObject(), nil
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
                --username admin \
                --firstname Superset \
                --lastname Admin \
                --email admin@superset.com \
                --password admin \
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

func (b *SupersetConfigSecretBuilder) BuildConfig(_ context.Context) (ctrlclient.Object, error) {
	var config = map[string]string{
		"superset_config.py":    b.getConfig(),
		"superset_init.sh":      b.getSupersetInit(),
		"superset_bootstrap.sh": b.getSupersetBootstrap(),
	}
	b.AddData(config)
	return b.GetObject(), nil
}

func NewEnvSecretReconciler(
	client resourceClient.ResourceClient,
	clusterConfig *common.ClusterConfig,
) *reconciler.SimpleResourceReconciler[builder.ConfigBuilder] {

	envSecretBuilder := NewEnvSecretBuilder(
		client,
		clusterConfig,
	)

	return reconciler.NewSimpleResourceReconciler[builder.ConfigBuilder](
		client,
		clusterConfig.ConfigSecretName,
		envSecretBuilder,
	)

}

func NewSupersetConfigSecretReconciler(
	client resourceClient.ResourceClient,
	clusterConfig *common.ClusterConfig,
) *reconciler.SimpleResourceReconciler[builder.ConfigBuilder] {

	supersetConfigSecretBuilder := NewSupersetConfigSecretBuilder(
		client,
		clusterConfig,
	)

	return reconciler.NewSimpleResourceReconciler[builder.ConfigBuilder](
		client,
		clusterConfig.ConfigSecretName,
		supersetConfigSecretBuilder,
	)
}
