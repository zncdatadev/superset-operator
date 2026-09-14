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
	"fmt"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	opcommon "github.com/zncdatadev/operator-go/pkg/common"
	"github.com/zncdatadev/operator-go/pkg/constant"

	supersetv1alpha1 "github.com/zncdatadev/superset-operator/api/v1alpha1"
)

const gen2ManagedByValue = "superset.kubedoop.dev"

// Gen2MigrationGuard prevents the first Gen 3 reconcile from changing client Service selectors
// while legacy StatefulSets are still present. operator-go preserves a live StatefulSet's
// immutable selector, but the Gen 3 pod template uses a different workload identity; Kubernetes
// would reject that update after the Service had already moved away from the old pods.
type Gen2MigrationGuard struct{}

var _ opcommon.ClusterExtension[*supersetv1alpha1.SupersetCluster] = (*Gen2MigrationGuard)(nil)

// NewGen2MigrationGuard creates the cluster preflight extension.
func NewGen2MigrationGuard() *Gen2MigrationGuard {
	return &Gen2MigrationGuard{}
}

// Name identifies the extension in framework diagnostics.
func (*Gen2MigrationGuard) Name() string {
	return "gen2-statefulset-migration-guard"
}

// PreReconcile blocks before the framework applies ConfigMaps or Services when legacy workloads
// remain. A fresh install and a migration in which the stopped Gen 2 workloads were deliberately
// removed both pass through.
func (*Gen2MigrationGuard) PreReconcile(
	ctx context.Context,
	k8sClient client.Client,
	cr *supersetv1alpha1.SupersetCluster,
) error {
	if cr == nil {
		return nil
	}

	legacy := &appsv1.StatefulSetList{}
	if err := k8sClient.List(
		ctx,
		legacy,
		client.InNamespace(cr.Namespace),
		client.MatchingLabels{
			constant.LabelKubernetesInstance:  cr.Name,
			constant.LabelKubernetesManagedBy: gen2ManagedByValue,
		},
	); err != nil {
		return fmt.Errorf("inspect legacy Superset StatefulSets: %w", err)
	}
	if len(legacy.Items) == 0 {
		return nil
	}

	names := make([]string, 0, len(legacy.Items))
	for i := range legacy.Items {
		names = append(names, legacy.Items[i].Name)
	}
	sort.Strings(names)

	return fmt.Errorf(
		"gen 2 migration required: legacy StatefulSets %s still use managed-by=%s; stop the operator, scale down and remove only these Superset StatefulSets while preserving external database and PVC data, then start the Gen 3 operator",
		strings.Join(names, ", "),
		gen2ManagedByValue,
	)
}

// PostReconcile is a no-op; this extension only guards the mutation boundary.
func (*Gen2MigrationGuard) PostReconcile(
	context.Context,
	client.Client,
	*supersetv1alpha1.SupersetCluster,
) error {
	return nil
}

// OnReconcileError is a no-op; the framework retains and reports the original error.
func (*Gen2MigrationGuard) OnReconcileError(
	context.Context,
	client.Client,
	*supersetv1alpha1.SupersetCluster,
	error,
) error {
	return nil
}
