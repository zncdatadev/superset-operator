/*
Copyright 2024 zncdatadev.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"reflect"
	"testing"

	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/common"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
)

var _ common.ClusterInterface = (*SupersetCluster)(nil)

const defaultRoleGroupName = "default"

func TestSupersetClusterSpec_ToGenericSpec(t *testing.T) {
	t.Parallel()

	t.Run("preserves absent optional fields", func(t *testing.T) {
		t.Parallel()

		got := (&SupersetClusterSpec{}).ToGenericSpec()
		if got == nil {
			t.Fatal("ToGenericSpec() returned nil")
		}
		if got.Image != nil {
			t.Errorf("Image = %#v, want nil", got.Image)
		}
		if got.ClusterOperation != nil {
			t.Errorf("ClusterOperation = %#v, want nil", got.ClusterOperation)
		}
		if got.Roles != nil {
			t.Errorf("Roles = %#v, want nil when the typed node role is absent", got.Roles)
		}
	})

	t.Run("projects the typed node role without losing fields", func(t *testing.T) {
		t.Parallel()

		pullPolicy := corev1.PullAlways
		replicas := int32(3)
		clusterOperation := &commonsv1alpha1.ClusterOperationSpec{
			ReconciliationPaused: true,
			Stopped:              true,
		}
		roleConfig := &commonsv1alpha1.RoleConfigSpec{}
		roleGroupConfig := &commonsv1alpha1.RoleGroupConfigSpec{}
		groupConfig := &commonsv1alpha1.RoleGroupConfigSpec{}
		rolePodOverrides := &k8sruntime.RawExtension{Raw: []byte(`{"metadata":{"labels":{"scope":"role"}}}`)}
		groupPodOverrides := &k8sruntime.RawExtension{Raw: []byte(`{"metadata":{"labels":{"scope":"group"}}}`)}
		roleOverrides := &commonsv1alpha1.OverridesSpec{
			CliOverrides:    []string{"role-command", "--role"},
			EnvOverrides:    map[string]string{"ROLE_ENV": "role-value"},
			ConfigOverrides: map[string]map[string]string{"superset_config.py": {"ROLE": "role-value"}},
			PodOverrides:    rolePodOverrides,
		}
		groupOverrides := &commonsv1alpha1.OverridesSpec{
			CliOverrides:    []string{"group-command", "--group"},
			EnvOverrides:    map[string]string{"GROUP_ENV": "group-value"},
			ConfigOverrides: map[string]map[string]string{"superset_config.py": {"GROUP": "group-value"}},
			PodOverrides:    groupPodOverrides,
		}
		spec := &SupersetClusterSpec{
			Image: &ImageSpec{
				Custom:          "registry.example/superset:test",
				Repo:            "registry.example/kubedoop",
				KubedoopVersion: "1.2.3",
				ProductVersion:  "4.1.2",
				PullPolicy:      &pullPolicy,
				PullSecretName:  "registry-credentials",
			},
			ClusterOperation: clusterOperation,
			Node: &NodeSpec{
				RoleGroups: map[string]NodeRoleGroupSpec{
					defaultRoleGroupName: {
						Replicas:      &replicas,
						Config:        &NodeConfigSpec{RoleGroupConfigSpec: groupConfig},
						OverridesSpec: groupOverrides,
					},
					"empty": {},
				},
				Config:        &NodeConfigSpec{RoleGroupConfigSpec: roleGroupConfig},
				RoleConfig:    roleConfig,
				OverridesSpec: roleOverrides,
			},
		}

		got := spec.ToGenericSpec()
		if got.ClusterOperation != clusterOperation {
			t.Error("ClusterOperation does not retain the typed spec pointer")
		}
		if got.Image == nil {
			t.Fatal("Image = nil, want projected image")
		}
		wantImage := commonsv1alpha1.ImageSpec{
			Custom:          spec.Image.Custom,
			Repo:            spec.Image.Repo,
			KubedoopVersion: spec.Image.KubedoopVersion,
			ProductVersion:  spec.Image.ProductVersion,
			PullPolicy:      pullPolicy,
			PullSecretName:  spec.Image.PullSecretName,
		}
		if !reflect.DeepEqual(*got.Image, wantImage) {
			t.Errorf("Image = %#v, want %#v", *got.Image, wantImage)
		}

		if len(got.Roles) != 1 {
			t.Fatalf("len(Roles) = %d, want 1", len(got.Roles))
		}
		role, ok := got.Roles[RoleNameNode]
		if !ok {
			t.Fatalf("Roles does not contain %q: %#v", RoleNameNode, got.Roles)
		}
		if role.RoleConfig != roleConfig {
			t.Error("node RoleConfig does not retain the typed spec pointer")
		}
		if role.Config != roleGroupConfig {
			t.Error("node Config does not retain the typed spec pointer")
		}
		assertOverrides(t, "role", role.GetOverrides(), roleOverrides)

		if len(role.RoleGroups) != 2 {
			t.Fatalf("len(node.RoleGroups) = %d, want 2", len(role.RoleGroups))
		}
		group, ok := role.RoleGroups[defaultRoleGroupName]
		if !ok {
			t.Fatalf("node.RoleGroups does not contain %q: %#v", defaultRoleGroupName, role.RoleGroups)
		}
		if group.Replicas != &replicas {
			t.Error("role-group Replicas does not retain the typed spec pointer")
		}
		if group.Config != groupConfig {
			t.Error("role-group Config does not retain the typed spec pointer")
		}
		assertOverrides(t, "role group", group.GetOverrides(), groupOverrides)

		empty := role.RoleGroups["empty"]
		if empty.Replicas != nil || empty.Config != nil || empty.GetOverrides() != nil {
			t.Errorf("empty role group gained values: %#v", empty)
		}
	})
}

func TestSupersetCluster_GetSpec(t *testing.T) {
	t.Parallel()

	clusterOperation := &commonsv1alpha1.ClusterOperationSpec{Stopped: true}
	replicas := int32(2)
	cluster := &SupersetCluster{
		Spec: SupersetClusterSpec{
			ClusterOperation: clusterOperation,
			Node: &NodeSpec{RoleGroups: map[string]NodeRoleGroupSpec{
				defaultRoleGroupName: {Replicas: &replicas},
			}},
		},
	}

	got := cluster.GetSpec()
	if got == nil {
		t.Fatal("GetSpec() returned nil")
	}
	if got.ClusterOperation != clusterOperation {
		t.Error("GetSpec().ClusterOperation does not point into the typed spec")
	}
	role, ok := got.Roles[RoleNameNode]
	if !ok {
		t.Fatalf("GetSpec().Roles does not contain %q", RoleNameNode)
	}
	group, ok := role.RoleGroups["default"]
	if !ok {
		t.Fatal("GetSpec() did not project the default role group")
	}
	if group.Replicas != &replicas {
		t.Error("GetSpec() did not retain the role-group replica pointer")
	}
}

func TestSupersetCluster_GetStatus(t *testing.T) {
	t.Parallel()

	cluster := &SupersetCluster{}
	got := cluster.GetStatus()
	if got != &cluster.Status.GenericClusterStatus {
		t.Fatal("GetStatus() did not return a pointer into SupersetCluster.Status")
	}

	got.ObservedGeneration = 7
	got.RoleGroups = map[string][]string{RoleNameNode: {"default"}}
	got.Conditions = []metav1.Condition{{Type: "Available", Status: metav1.ConditionTrue}}

	if cluster.Status.ObservedGeneration != 7 {
		t.Errorf("Status.ObservedGeneration = %d, want 7", cluster.Status.ObservedGeneration)
	}
	if !reflect.DeepEqual(cluster.Status.RoleGroups, got.RoleGroups) {
		t.Errorf("Status.RoleGroups = %#v, want %#v", cluster.Status.RoleGroups, got.RoleGroups)
	}
	if !reflect.DeepEqual(cluster.Status.Conditions, got.Conditions) {
		t.Errorf("Status.Conditions = %#v, want %#v", cluster.Status.Conditions, got.Conditions)
	}
}

func TestSupersetCluster_VectorAggregatorConfigMapName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		clusterConfig *ClusterConfigSpec
		want          string
	}{
		{name: "absent cluster config"},
		{
			name:          "configured discovery ConfigMap",
			clusterConfig: &ClusterConfigSpec{VectorAggregatorConfigMapName: "vector-aggregator-discovery"},
			want:          "vector-aggregator-discovery",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cluster := &SupersetCluster{Spec: SupersetClusterSpec{ClusterConfig: tt.clusterConfig}}
			if got := cluster.VectorAggregatorConfigMapName(); got != tt.want {
				t.Errorf("VectorAggregatorConfigMapName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func assertOverrides(
	t *testing.T,
	scope string,
	got *commonsv1alpha1.OverridesSpec,
	want *commonsv1alpha1.OverridesSpec,
) {
	t.Helper()

	if got == nil {
		t.Fatalf("%s overrides = nil", scope)
	}
	if !reflect.DeepEqual(got.ConfigOverrides, want.ConfigOverrides) {
		t.Errorf("%s ConfigOverrides = %#v, want %#v", scope, got.ConfigOverrides, want.ConfigOverrides)
	}
	if !reflect.DeepEqual(got.EnvOverrides, want.EnvOverrides) {
		t.Errorf("%s EnvOverrides = %#v, want %#v", scope, got.EnvOverrides, want.EnvOverrides)
	}
	if !reflect.DeepEqual(got.CliOverrides, want.CliOverrides) {
		t.Errorf("%s CliOverrides = %#v, want %#v", scope, got.CliOverrides, want.CliOverrides)
	}
	if got.PodOverrides != want.PodOverrides {
		t.Errorf("%s PodOverrides does not retain the typed spec pointer", scope)
	}
}
