package common

import (
	"context"
	"net/url"
	"path"
	"strconv"
	"strings"

	authv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/authentication/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/builder"
	"github.com/zncdatadev/operator-go/pkg/client"
	"github.com/zncdatadev/operator-go/pkg/constants"
	"github.com/zncdatadev/operator-go/pkg/productlogging"
	"github.com/zncdatadev/operator-go/pkg/reconciler"
	"github.com/zncdatadev/operator-go/pkg/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	supersetv1alpha1 "github.com/zncdatadev/superset-operator/api/v1alpha1"
)

const (
	SupersetConfigFilename = "superset_config.py"
	SupersetLogFilename    = "log_config.py"
)

var (
	SupersetLogPath = path.Join(constants.KubedoopLogDir, "superset")
)

const (
	DefaultLDAPFieldEmail     = "email"
	DefaultLDAPFieldGivenName = "givenName"
	DefaultLDAPFieldGroup     = "memberOf"
	DefaultLDAPFieldSurname   = "sn"
	DefaultLDAPFieldUid       = "uid"

	LDAPBindCredentialsUserFilename     = "user"
	LDAPBindCredentialsPasswordFilename = "password"
)

type SupersetConfigMapBuilder struct {
	builder.ConfigMapBuilder

	ClusterConfig *supersetv1alpha1.ClusterConfigSpec

	ClusterName   string
	RoleName      string
	RoleGroupName string
}

func NewSupersetConfigBuilder(
	client *client.Client,
	roleGroupInfo reconciler.RoleGroupInfo,
	clusterConfig *supersetv1alpha1.ClusterConfigSpec,
) *SupersetConfigMapBuilder {
	return &SupersetConfigMapBuilder{
		ConfigMapBuilder: *builder.NewConfigMapBuilder(
			client,
			roleGroupInfo.GetFullName(),
			func(o *builder.Options) {
				o.Labels = roleGroupInfo.GetLabels()
				o.Annotations = roleGroupInfo.GetAnnotations()
			},
		),
		ClusterConfig: clusterConfig,
		ClusterName:   roleGroupInfo.ClusterName,
		RoleName:      roleGroupInfo.RoleName,
		RoleGroupName: roleGroupInfo.RoleGroupName,
	}
}

func (b *SupersetConfigMapBuilder) getVectorConfig(ctx context.Context) (string, error) {
	if b.ClusterConfig != nil && b.ClusterConfig.VectorAggregatorConfigMapName != "" {
		s, err := productlogging.MakeVectorYaml(
			ctx,
			b.Client.Client,
			b.Client.GetOwnerNamespace(),
			b.ClusterName,
			b.RoleName,
			b.RoleGroupName,
			b.ClusterConfig.VectorAggregatorConfigMapName,
		)
		if err != nil {
			return "", err
		}
		return s, nil
	}

	return "", nil
}

func (b *SupersetConfigMapBuilder) getLogConfig() string {
	config := `
import logging
import os
from pathlib import Path

import flask.config
from pythonjsonlogger import jsonlogger

from superset.utils.logging_configurator import LoggingConfigurator

LOGDIR = Path('` + SupersetLogPath + `')

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

func (b *SupersetConfigMapBuilder) getAuthProvider(ctx context.Context) (*authv1alpha1.AuthenticationProvider, error) {
	if b.ClusterConfig.Authentication == nil {
		return nil, nil
	}

	authClass := &authv1alpha1.AuthenticationClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.ClusterConfig.Authentication.AuthenticationClass,
			Namespace: b.Client.GetOwnerNamespace(),
		},
	}
	if err := b.Client.GetWithObject(ctx, authClass); err != nil {
		return nil, err
	}

	return authClass.Spec.AuthenticationProvider, nil

}

func (b *SupersetConfigMapBuilder) getLDAPConfig(ldapProvider authv1alpha1.LDAPProvider) string {

	server := url.URL{Scheme: "ldap", Host: ldapProvider.Hostname}
	if ldapProvider.Port != 0 {
		server.Host += ":" + strconv.Itoa(ldapProvider.Port)
	}

	ldapFieldUid := DefaultLDAPFieldUid
	ldapFieldSurname := DefaultLDAPFieldSurname
	ldapFieldGivenName := DefaultLDAPFieldGivenName
	ldapFieldEmail := DefaultLDAPFieldEmail
	ldapFieldGroup := DefaultLDAPFieldGroup

	if ldapProvider.LDAPFieldNames != nil {
		ldapFieldUid = ldapProvider.LDAPFieldNames.Uid
		ldapFieldSurname = ldapProvider.LDAPFieldNames.Surname
		ldapFieldGivenName = ldapProvider.LDAPFieldNames.GivenName
		ldapFieldEmail = ldapProvider.LDAPFieldNames.Email
		ldapFieldGroup = ldapProvider.LDAPFieldNames.Group
	}

	// AUTH_ROLES_MAPPING is a dictionary that maps LDAP groups to Superset roles for ldap permissions.
	// The key is the LDAP group and the value is the Superset role.
	// the LDAP group should be created in the LDAP server first, and add the user to the group.
	config := `
# Set the authentication type to OAuth
AUTH_TYPE = AUTH_LDAP
AUTH_USER_REGISTRATION=True
AUTH_LDAP_SERVER = '` + server.String() + `'
AUTH_LDAP_SEARCH = '` + ldapProvider.SearchBase + `'
AUTH_LDAP_SEARCH_FILTER = '` + ldapProvider.SearchFilter + `'
AUTH_LDAP_UID_FIELD = '` + ldapFieldUid + `'
AUTH_LDAP_GROUP_FIELD = '` + ldapFieldGroup + `'
AUTH_LDAP_FIRSTNAME_FIELD = '` + ldapFieldGivenName + `'
AUTH_LDAP_LASTNAME_FIELD = '` + ldapFieldSurname + `'
AUTH_LDAP_EMAIL_FIELD = '` + ldapFieldEmail + `'
AUTH_ROLES_MAPPING = {
	"cn=superset_users,ou=groups,dc=example,dc=com": ["Admin"],
	"cn=superset_admins,ou=groups,dc=example,dc=com": ["Admin"],
}
`

	if ldapProvider.BindCredentials != nil {
		mouhtPath := path.Join(constants.KubedoopSecretDir, ldapProvider.BindCredentials.SecretClass)
		config += `
with open('` + path.Join(mouhtPath, LDAPBindCredentialsUserFilename) + `', 'r') as f:
    AUTH_LDAP_BIND_USER = f.readline().strip()

with open('` + path.Join(mouhtPath, LDAPBindCredentialsPasswordFilename) + `', 'r') as f:
    AUTH_LDAP_BIND_PASSWORD = f.readline().strip()
`
	}

	// TODO: Add TLS configuration
	return util.IndentTab4Spaces(config)
}

func (b *SupersetConfigMapBuilder) getOIDCConfig(oidcPrivider authv1alpha1.OIDCProvider) string {
	scopes := []string{"openid", "email", "profile"}
	issuer := url.URL{
		Scheme: "http",
		Host:   oidcPrivider.Hostname,
		Path:   oidcPrivider.RootPath,
	}

	if oidcPrivider.Port != 0 {
		issuer.Host += ":" + strconv.Itoa(oidcPrivider.Port)
	}

	if b.ClusterConfig.Authentication.Oidc != nil {
		scopes = append(scopes, b.ClusterConfig.Authentication.Oidc.ExtraScopes...)
	}

	providerHint := oidcPrivider.ProviderHint

	config := `
# Set the authentication type to OAuth
AUTH_TYPE = AUTH_OAUTH

AUTH_ROLES_SYNC_AT_LOGIN = False
AUTH_TYPE = AUTH_OAUTH
AUTH_USER_REGISTRATION = True
AUTH_USER_REGISTRATION_ROLE = "Public"
OAUTH_PROVIDERS = [
    {   'name': '` + providerHint + `',    # Name of the provider
        'token_key': 'access_token',    # Name of the token in the response of access_token_url
        'icon': 'fa-address-card',    # Icon for the provider
        'remote_app': {
            'client_id': os.environ.get('CLIENT_ID'),    # Client Id (Identify Superset application)
            'client_secret': os.environ.get('CLIENT_SECRET'),    # Secret for this Client Id (Identify Superset application)
            'client_kwargs': {
                'scope': '` + strings.Join(scopes, " ") + `'               # Scope for the Authorization
            },
			'api_base_url': '` + issuer.String() + `/protocol/',    # Base URL for the API
            'server_metadata_url': '` + issuer.String() + `/.well-known/openid-configuration',
        }
    }
]
`
	return util.IndentTab4Spaces(config)
}

func (b *SupersetConfigMapBuilder) getAPPConfig(authProvider *authv1alpha1.AuthenticationProvider) string {
	config := `import os

from flask_appbuilder.security.manager import ( AUTH_DB, AUTH_LDAP, AUTH_OAUTH, AUTH_OID, AUTH_REMOTE_USER )
from superset.stats_logger import StatsdStatsLogger

from log_config import JsonLoggingConfigurator


LOGGING_CONFIGURATOR = JsonLoggingConfigurator()

ROW_LIMIT = 10000

SECRET_KEY = os.environ.get('SECRET_KEY')

SQLALCHEMY_DATABASE_URI = os.environ.get('SQLALCHEMY_DATABASE_URI')

STATS_LOGGER = StatsdStatsLogger(host='0.0.0.0', port=9125)

SUPERSET_WEBSERVER_TIMEOUT = 300

TALISMAN_ENABLED = False
`
	if authProvider == nil {
		return util.IndentTab4Spaces(config)
	}

	if authProvider.OIDC != nil {
		config += b.getOIDCConfig(*authProvider.OIDC)
	}

	if authProvider.LDAP != nil {
		config += b.getLDAPConfig(*authProvider.LDAP)
	}

	return util.IndentTab4Spaces(config)
}

func (b *SupersetConfigMapBuilder) Build(ctx context.Context) (ctrlclient.Object, error) {

	authProvider, err := b.getAuthProvider(ctx)
	if err != nil {
		return nil, err
	}

	b.AddItem(SupersetLogFilename, b.getLogConfig())
	b.AddItem(SupersetConfigFilename, b.getAPPConfig(authProvider))

	vectorConfig, err := b.getVectorConfig(ctx)
	if err != nil {
		return nil, err
	}
	b.AddItem(builder.VectorConfigFileName, vectorConfig)
	return b.GetObject(), nil
}

func NewConfigReconciler(
	client *client.Client,
	clusterConfig *supersetv1alpha1.ClusterConfigSpec,
	roleGroupInfo reconciler.RoleGroupInfo,
) *reconciler.SimpleResourceReconciler[builder.ConfigBuilder] {

	supersetConfigSecretBuilder := NewSupersetConfigBuilder(
		client,
		roleGroupInfo,
		clusterConfig,
	)

	return reconciler.NewSimpleResourceReconciler[builder.ConfigBuilder](
		client,
		supersetConfigSecretBuilder,
	)
}
