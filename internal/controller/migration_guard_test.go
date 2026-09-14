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

package controller

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/zncdatadev/operator-go/pkg/constant"
)

func TestGen2MigrationGuard(t *testing.T) {
	t.Parallel()

	cr := newControllerTestCluster()
	legacyName := cr.Name + "-node-legacy"
	legacy := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{
		Name:      legacyName,
		Namespace: cr.Namespace,
		Labels: map[string]string{
			constant.LabelKubernetesInstance:  cr.Name,
			constant.LabelKubernetesManagedBy: gen2ManagedByValue,
		},
	}}
	framework := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{
		Name:      cr.Name + "-node-framework",
		Namespace: cr.Namespace,
		Labels: map[string]string{
			constant.LabelKubernetesInstance:  cr.Name,
			constant.LabelKubernetesManagedBy: "operator-go",
		},
	}}

	t.Run("allows a fresh install", func(t *testing.T) {
		t.Parallel()
		k8sClient := fake.NewClientBuilder().WithScheme(controllerTestScheme(t)).Build()
		if err := NewGen2MigrationGuard().PreReconcile(context.Background(), k8sClient, cr.DeepCopy()); err != nil {
			t.Fatalf("PreReconcile() error = %v, want nil", err)
		}
	})

	t.Run("allows an already migrated workload", func(t *testing.T) {
		t.Parallel()
		k8sClient := fake.NewClientBuilder().WithScheme(controllerTestScheme(t)).WithObjects(framework.DeepCopy()).Build()
		if err := NewGen2MigrationGuard().PreReconcile(context.Background(), k8sClient, cr.DeepCopy()); err != nil {
			t.Fatalf("PreReconcile() error = %v, want nil", err)
		}
	})

	t.Run("blocks a legacy workload before resource mutation", func(t *testing.T) {
		t.Parallel()
		k8sClient := fake.NewClientBuilder().WithScheme(controllerTestScheme(t)).WithObjects(legacy.DeepCopy()).Build()
		err := NewGen2MigrationGuard().PreReconcile(context.Background(), k8sClient, cr.DeepCopy())
		if err == nil || !strings.Contains(err.Error(), "gen 2 migration required") ||
			!strings.Contains(err.Error(), legacyName) {
			t.Fatalf("PreReconcile() error = %v, want migration error naming %q", err, legacyName)
		}
	})
}
