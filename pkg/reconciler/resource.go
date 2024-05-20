package reconciler

import (
	"context"
	"time"

	"github.com/zncdata-labs/superset-operator/pkg/builder"
	resourceClient "github.com/zncdata-labs/superset-operator/pkg/client"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ResourceReconciler[B builder.Builder] interface {
	Reconciler
	GetBuilder() B
	ResourceReconcile(ctx context.Context, resource client.Object) Result
}

var _ ResourceReconciler[builder.Builder] = &GenericResourceReconciler[AnySpec, builder.Builder]{}

type GenericResourceReconciler[T AnySpec, B builder.Builder] struct {
	BaseReconciler[T]
	Builder B
}

func (r *GenericResourceReconciler[T, B]) GetBuilder() B {
	return r.Builder
}

func (r *GenericResourceReconciler[T, B]) ResourceReconcile(ctx context.Context, resource client.Object) Result {

	if err := ctrl.SetControllerReference(r.Client.OwnerReference, resource, r.GetCtrlScheme()); err != nil {
		return NewResult(true, 0, err)
	}

	if mutation, err := r.Client.CreateOrUpdate(ctx, resource); err != nil {
		return NewResult(true, 0, err)
	} else if mutation {
		return NewResult(true, time.Second, nil)
	}
	return NewResult(false, 0, nil)
}

func (r *GenericResourceReconciler[T, B]) Reconcile(ctx context.Context) Result {
	resource, err := r.GetBuilder().Build(ctx)

	if err != nil {
		return NewResult(true, 0, err)
	}
	return r.ResourceReconcile(ctx, resource)
}

func (r *GenericResourceReconciler[T, B]) Ready(ctx context.Context) Result {
	return NewResult(false, 0, nil)
}

func NewGenericResourceReconciler[T AnySpec, B builder.Builder](
	client resourceClient.ResourceClient,
	name string,
	spec T,
	builder B,
) *GenericResourceReconciler[T, B] {
	return &GenericResourceReconciler[T, B]{
		BaseReconciler: BaseReconciler[T]{
			Client: client,
			Name:   name,
			Spec:   spec,
		},
		Builder: builder,
	}
}

type SimpleResourceReconciler[B builder.Builder] struct {
	GenericResourceReconciler[AnySpec, B]
}

// NewSimpleResourceReconciler creates a new resource reconciler with a simple builder
// that does not require a spec, and can not use the spec.
func NewSimpleResourceReconciler[B builder.Builder](
	client resourceClient.ResourceClient,
	name string,
	builder B,
) *SimpleResourceReconciler[B] {
	return &SimpleResourceReconciler[B]{
		GenericResourceReconciler: *NewGenericResourceReconciler[AnySpec, B](
			client,
			name,
			nil,
			builder,
		),
	}
}
