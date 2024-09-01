package v1alpha1

type ClusterConfigSpec struct {
	// +kubebuilder:validation:Optional
	Authentication *AuthenticationSpec `json:"authentication,omitempty"`

	// Superset administrator user credentials and database connection configurations.
	// It must contains the key:
	//   - `adminUser.username`: The first name of the admin user.
	//   - `adminUser.firstname`: The first name of the admin user.
	//   - `adminUser.lastname`: The last name of the admin user.
	//   - `adminUser.email`: The email of the admin user.
	//   - `adminUser.password`: The password of the admin user.
	//   - `appSecretKey`: It is flask app secret key. You can generate by `openssl rand -hex 32`.
	//   - `connection.sqlalchemyDatabaseUri`: It is the database connection URI. You can use the following format:
	//     - `postgresql://<username>:<password>@<host>:<port>/<database>`
	// +kubebuilder:validation:Required
	CredentialsSecret string `json:"credentialsSecret"`

	// +kubebuilder:validation:Optional
	ListenerClass string `json:"listenerClass,omitempty"`

	// +kubebuilder:validation:Optional
	VectorAggregatorConfigMapName string `json:"vectorAggregatorConfigMapName,omitempty"`
}

// AppSecretKeySpec defines the app secret key spec.
type AppSecretKeySpec struct {
	// +kubebuilder:validation=Optional
	// ExistSecret is the name of the secret that contains the secret key.
	// It must contain the key `SUPERSET_SECRET_KEY`.
	// Note: To avoid the key name confusions, the key name must be started with `SUPERSET_`.
	ExistSecret string `json:"existSecret,omitempty"`
	// +kubebuilder:validation=Optional
	// If value is not set, the secret will be generated.
	// When you migrate the Superset instance, you should keep the same secret key in the new instance.
	SecretKey string `json:"secretKey,omitempty"`
}

// AuthenticationSpec defines the authentication spec.
type AuthenticationSpec struct {
	// +kubebuilder:validation:Required
	AuthenticationClass string `json:"authenticationClass"`

	// +kubebuilder:validation:Optional
	Oidc *OidcSpec `json:"oidc,omitempty"`

	// +kubebuilder:validation:Optional
	SyncRolesAt string `json:"syncRolesAt,omitempty"`

	// +kubebuilder:validation:Optional
	UserRegistration bool `json:"userRegistration,omitempty"`

	// +kubebuilder:validation:Optional
	UserRegistrationRole string `json:"userRegistrationRole,omitempty"`
}

// OidcSpec defines the OIDC spec.
type OidcSpec struct {
	// +kubebuilder:validation:Required
	ClientCredentialsSecret string `json:"clientCredentialsSecret"`

	// +kubebuilder:validation:Optional
	ExtraScopes []string `json:"extraScopes,omitempty"`
}
