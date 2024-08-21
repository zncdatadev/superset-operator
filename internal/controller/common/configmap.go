package common

import (
	"context"

	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	SupersetConfigFilename = "superset_config.py"
	SupersetLogFilename    = "log_config.py"
)

type SupersetConfigMapBuilder struct {
	builder.ConfigMapBuilder
}

func NewSupersetConfigBuilder(
	client *client.Client,
	name string,
	options builder.WorkloadOptions,
) *SupersetConfigMapBuilder {
	return &SupersetConfigMapBuilder{
		ConfigMapBuilder: *builder.NewConfigMapBuilder(
			client,
			name,
			options.Labels,
			options.Annotations,
		),
	}
}

func (b *SupersetConfigMapBuilder) getLogConfig() string {
	config := `
import logging
import os
from pathlib import Path

import flask.config
from pythonjsonlogger import jsonlogger

from superset.utils.logging_configurator import LoggingConfigurator

LOGDIR = Path('/kubedoop/log/superset')

os.makedirs(LOGDIR, exist_ok=True)

LOGLEVEL = logging.INFO


class JsonLoggingConfigurator(LoggingConfigurator):
	def configure_logging(self, app_config: flask.config.Config, debug_mode: bool):
		logFormat = '%(asctime)s:%(levelname)s:%(name)s:%(message)s'

		plainTextFormatter = logging.Formatter(logFormat)
		jsonFormatter = jsonlogger.JsonFormatter(logFormat)

		consoleHandler = logging.StreamHandler()
		consoleHandler.setLevel(LOGLEVEL)
		consoleHandler.setFormatter(plainTextFormatter)

		fileHandler = logging.handlers.RotatingFileHandler(
			LOGDIR.joinpath('superset.py.json'),
			maxBytes=1048576,
			backupCount=1,
		)
		fileHandler.setLevel(LOGLEVEL)
		fileHandler.setFormatter(jsonFormatter)

		rootLogger = logging.getLogger()
		rootLogger.setLevel(LOGLEVEL)
		rootLogger.addHandler(consoleHandler)
		rootLogger.addHandler(fileHandler)
	`

	return util.IndentTab4Spaces(config)
}

func (b *SupersetConfigMapBuilder) getAPPConfig() string {
	config := `
import os

from flask_appbuilder.security.manager import (
    AUTH_DB, 
    AUTH_LDAP,
    AUTH_OAUTH, 
    AUTH_OID, 
    AUTH_REMOTE_USER,
    )
from superset.stats_logger import StatsdStatsLogger

from log_config import JsonLoggingConfigurator


LOGGING_CONFIGURATOR = JsonLoggingConfigurator()

MAPBOX_API_KEY = os.environ.get('MAPBOX_API_KEY', '')

ROW_LIMIT = 10000

SECRET_KEY = os.environ.get('SECRET_KEY')

SQLALCHEMY_DATABASE_URI = os.environ.get('SQLALCHEMY_DATABASE_URI')

STATS_LOGGER = StatsdStatsLogger(host='0.0.0.0', port=9125)

SUPERSET_WEBSERVER_TIMEOUT = 300

TALISMAN_ENABLED = False
	`

	return util.IndentTab4Spaces(config)
}

func (b *SupersetConfigMapBuilder) Build(_ context.Context) (ctrlclient.Object, error) {
	var data = map[string]string{
		SupersetLogFilename:    b.getLogConfig(),
		SupersetConfigFilename: b.getAPPConfig(),
	}

	b.AddData(data)

	return b.GetObject(), nil
}

func NewConfigReconciler(
	client *client.Client,
	roleGroupInfo reconciler.RoleGroupInfo,
) *reconciler.SimpleResourceReconciler[builder.ConfigBuilder] {

	options := builder.WorkloadOptions{
		Options: builder.Options{
			ClusterName: roleGroupInfo.GetFullName(),
			Labels:      roleGroupInfo.GetLabels(),
			Annotations: roleGroupInfo.GetAnnotations(),
		},
	}

	supersetConfigSecretBuilder := NewSupersetConfigBuilder(
		client,
		roleGroupInfo.GetFullName(),
		options,
	)

	return reconciler.NewSimpleResourceReconciler[builder.ConfigBuilder](
		client,
		roleGroupInfo.GetFullName(),
		supersetConfigSecretBuilder,
	)
}
