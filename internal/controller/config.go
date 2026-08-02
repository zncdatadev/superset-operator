package controller

import (
	"net/url"
	"path"
	"strconv"
	"strings"

	authv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/authentication/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/constant"

	supersetv1alpha1 "github.com/zncdatadev/superset-operator/api/v1alpha1"
)

// indent4 expands tabs to four spaces — exactly the Gen 2 util.IndentTab4Spaces
// semantics (despite its name, it never indented lines). Line indentation would break
// the rendered files: they are Python modules, where a leading indent on any top-level
// statement is an IndentationError. Tabs only appear inside block literals (e.g. the
// AUTH_ROLES_MAPPING dict), where expansion keeps the source clean.
func indent4(s string) string {
	return strings.ReplaceAll(s, "\t", "    ")
}

const (
	supersetConfigFilename = "superset_config.py"
	logConfigFilename      = "log_config.py"

	// supersetLogContainer names the log producer the framework logging engine renders for.
	// It deliberately differs from the pod container name ("node"): the Vector pipeline tags
	// each event with the per-container log directory it was read from, and Superset's log
	// contract (asserted by downstream consumers) pins that tag to the PRODUCT name.
	supersetLogContainer = "superset"
	supersetLogFileName  = "superset.py.json"
	supersetLogDir       = constant.KubedoopLogDir + supersetLogContainer
)

const (
	defaultLDAPFieldEmail     = "email"
	defaultLDAPFieldGivenName = "givenName"
	defaultLDAPFieldGroup     = "memberOf"
	defaultLDAPFieldSurname   = "sn"
	defaultLDAPFieldUid       = "uid"

	ldapBindCredentialsUserFilename     = "user"
	ldapBindCredentialsPasswordFilename = "password"
)

// renderSupersetConfig renders superset_config.py. Logging is delegated to the
// framework-rendered log_config.py (a logging.config dictConfig module); the no-op
// LoggingConfigurator stops Superset's app factory from reconfiguring root logging
// afterwards. Authentication sections are appended per the resolved AuthenticationClass.
func renderSupersetConfig(
	authProvider *authv1alpha1.AuthenticationProvider,
	auth *supersetv1alpha1.AuthenticationSpec,
	vectorActive bool,
) string {
	config := `import logging.config
import os

from flask_appbuilder.security.manager import ( AUTH_DB, AUTH_LDAP, AUTH_OAUTH, AUTH_OID, AUTH_REMOTE_USER )
from superset.stats_logger import StatsdStatsLogger
from superset.utils.logging_configurator import LoggingConfigurator

import log_config


class KubedoopLoggingConfigurator(LoggingConfigurator):
    def configure_logging(self, app_config, debug_mode):
        # Logging is fully owned by log_config.LOGGING (rendered by the operator from
        # the CRD logging spec); keep Superset's app factory from reconfiguring it.
        pass


LOGGING_CONFIGURATOR = KubedoopLoggingConfigurator()

`
	if vectorActive {
		// The shared log volume is only mounted when the Vector pipeline is active; the
		// dictConfig file handler below opens its file at import time.
		config += `os.makedirs('` + supersetLogDir + `', exist_ok=True)
`
	}
	config += `logging.config.dictConfig(log_config.LOGGING)

ROW_LIMIT = 10000

SECRET_KEY = os.environ.get('SECRET_KEY')

SQLALCHEMY_DATABASE_URI = os.environ.get('SQLALCHEMY_DATABASE_URI')

STATS_LOGGER = StatsdStatsLogger(host='0.0.0.0', port=9125)

SUPERSET_WEBSERVER_TIMEOUT = 300

TALISMAN_ENABLED = False
`
	if authProvider == nil {
		return indent4(config)
	}

	if authProvider.OIDC != nil {
		config += renderOIDCConfig(*authProvider.OIDC, auth)
	}

	if authProvider.LDAP != nil {
		config += renderLDAPConfig(*authProvider.LDAP)
	}

	return indent4(config)
}

// renderOIDCConfig renders the Flask-AppBuilder OAuth provider block. CLIENT_ID and
// CLIENT_SECRET are injected into the container from the CR's clientCredentialsSecret.
func renderOIDCConfig(oidcProvider authv1alpha1.OIDCProvider, auth *supersetv1alpha1.AuthenticationSpec) string {
	scopes := []string{"openid", "email", "profile"}
	issuer := url.URL{
		Scheme: "http",
		Host:   oidcProvider.Hostname,
		Path:   oidcProvider.RootPath,
	}

	if oidcProvider.Port != 0 {
		issuer.Host += ":" + strconv.Itoa(oidcProvider.Port)
	}

	if auth != nil && auth.Oidc != nil {
		scopes = append(scopes, auth.Oidc.ExtraScopes...)
	}

	providerHint := oidcProvider.ProviderHint

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
            'client_secret': os.environ.get('CLIENT_SECRET'),    # Client secret for this Client Id (Identify Superset application)
            'client_kwargs': {
                'scope': '` + strings.Join(scopes, " ") + `'               # Scope for the Authorization
            },
			'api_base_url': '` + issuer.String() + `/protocol/',    # Base URL for the API
            'server_metadata_url': '` + issuer.String() + `/.well-known/openid-configuration',
        }
    }
]
`
	return indent4(config)
}

// renderLDAPConfig renders the Flask-AppBuilder LDAP block. Bind credentials are read
// from the secret-operator CSI volume mounted at /kubedoop/secret/<secretClass>.
func renderLDAPConfig(ldapProvider authv1alpha1.LDAPProvider) string {
	server := url.URL{Scheme: "ldap", Host: ldapProvider.Hostname}
	if ldapProvider.Port != 0 {
		server.Host += ":" + strconv.Itoa(ldapProvider.Port)
	}

	ldapFieldUid := defaultLDAPFieldUid
	ldapFieldSurname := defaultLDAPFieldSurname
	ldapFieldGivenName := defaultLDAPFieldGivenName
	ldapFieldEmail := defaultLDAPFieldEmail
	ldapFieldGroup := defaultLDAPFieldGroup

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
		mountPath := path.Join(constant.KubedoopSecretDir, ldapProvider.BindCredentials.SecretClass)
		config += `
with open('` + path.Join(mountPath, ldapBindCredentialsUserFilename) + `', 'r') as f:
    AUTH_LDAP_BIND_USER = f.readline().strip()

with open('` + path.Join(mountPath, ldapBindCredentialsPasswordFilename) + `', 'r') as f:
    AUTH_LDAP_BIND_PASSWORD = f.readline().strip()
`
	}

	// TODO: Add TLS configuration
	return indent4(config)
}

// mainContainerCommands returns the main container entrypoint script: it stages the
// operator-rendered config onto the Python path, initializes and migrates the metadata
// database, creates the admin user, then runs gunicorn with graceful TERM forwarding.
func mainContainerCommands() string {
	cmds := `
mkdir --parents /kubedoop/app/pythonpath

cp /kubedoop/mount/config/* /kubedoop/app/pythonpath


prepare_signal_handlers()
{
	unset term_child_pid
	unset term_kill_needed
	trap 'handle_term_signal' TERM
}


handle_term_signal()
{
	if [ "${term_child_pid}" ]; then
		kill -TERM "${term_child_pid}" 2>/dev/null
	else
		term_kill_needed="yes"
	fi
}


wait_for_termination()
{
	set +e
	term_child_pid=$1
	if [[ -v term_kill_needed ]]; then
		kill -TERM "${term_child_pid}" 2>/dev/null
	fi
	wait ${term_child_pid} 2>/dev/null
	trap - TERM
	wait ${term_child_pid} 2>/dev/null
	set -e
}


superset db upgrade

set +x # Disable debug mode
superset fab create-admin \
	--username "${ADMIN_USERNAME}" \
	--firstname "${ADMIN_FIRSTNAME}" \
	--lastname "${ADMIN_LASTNAME}" \
	--email "${ADMIN_EMAIL}" \
	--password "${ADMIN_PASSWORD}"

set -x # Enable debug mode

superset init

prepare_signal_handlers


gunicorn \
	--bind 0.0.0.0:${SUPERSET_PORT} \
	--threads 20 \
	--timeout 300 \
	--limit-request-line 0 \
	--limit-request-field_size 0 \
	'superset.app:create_app()' &


wait_for_termination $!
`
	return indent4(cmds)
}

// metricsContainerCommands returns the statsd-exporter sidecar entrypoint: Superset
// emits StatsD counters over UDP and the exporter serves them as Prometheus metrics.
func metricsContainerCommands() string {
	cmds := `
prepare_signal_handlers()
{
	unset term_child_pid
	unset term_kill_needed
	trap 'handle_term_signal' TERM
}


handle_term_signal()
{
	if [ "${term_child_pid}" ]; then
		kill -TERM "${term_child_pid}" 2>/dev/null
	else
		term_kill_needed="yes"
	fi
}


wait_for_termination()
{
	set +e
	term_child_pid=$1
	if [[ -v term_kill_needed ]]; then
		kill -TERM "${term_child_pid}" 2>/dev/null
	fi
	wait ${term_child_pid} 2>/dev/null
	trap - TERM
	wait ${term_child_pid} 2>/dev/null
	set -e
}


prepare_signal_handlers
/kubedoop/bin/statsd-exporter &
wait_for_termination $!
`
	return indent4(cmds)
}
