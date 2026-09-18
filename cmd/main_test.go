package main

import (
	"testing"

	"github.com/zncdatadev/operator-go/pkg/reconciler"

	supersetv1alpha1 "github.com/zncdatadev/superset-operator/api/v1alpha1"
)

const (
	testCredentialsSecret = "superset-credentials"
	testSharedSecret      = "shared"
)

func TestExternalDependencies(t *testing.T) {
	tests := []struct {
		name string
		cr   *supersetv1alpha1.SupersetCluster
		want []reconciler.Dependency
	}{
		{
			name: "nil cluster config",
			cr:   &supersetv1alpha1.SupersetCluster{},
		},
		{
			name: "credentials secret",
			cr: &supersetv1alpha1.SupersetCluster{Spec: supersetv1alpha1.SupersetClusterSpec{
				ClusterConfig: &supersetv1alpha1.ClusterConfigSpec{CredentialsSecret: testCredentialsSecret},
			}},
			want: []reconciler.Dependency{{
				Kind: reconciler.DependencySecret,
				Name: testCredentialsSecret,
			}},
		},
		{
			name: "credentials and oidc secrets",
			cr: &supersetv1alpha1.SupersetCluster{Spec: supersetv1alpha1.SupersetClusterSpec{
				ClusterConfig: &supersetv1alpha1.ClusterConfigSpec{
					CredentialsSecret: testCredentialsSecret,
					Authentication: &supersetv1alpha1.AuthenticationSpec{Oidc: &supersetv1alpha1.OidcSpec{
						ClientCredentialsSecret: "oidc-client",
					}},
				},
			}},
			want: []reconciler.Dependency{
				{Kind: reconciler.DependencySecret, Name: testCredentialsSecret},
				{Kind: reconciler.DependencySecret, Name: "oidc-client"},
			},
		},
		{
			name: "duplicate secret is declared once",
			cr: &supersetv1alpha1.SupersetCluster{Spec: supersetv1alpha1.SupersetClusterSpec{
				ClusterConfig: &supersetv1alpha1.ClusterConfigSpec{
					CredentialsSecret: testSharedSecret,
					Authentication: &supersetv1alpha1.AuthenticationSpec{Oidc: &supersetv1alpha1.OidcSpec{
						ClientCredentialsSecret: testSharedSecret,
					}},
				},
			}},
			want: []reconciler.Dependency{{Kind: reconciler.DependencySecret, Name: testSharedSecret}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := externalDependencies(tt.cr)
			if len(got) != len(tt.want) {
				t.Fatalf("externalDependencies() length = %d, want %d: %#v", len(got), len(tt.want), got)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("externalDependencies()[%d] = %#v, want %#v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
