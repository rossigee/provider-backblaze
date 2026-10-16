package providerconfig

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/rossigee/provider-backblaze/apis/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	providerapis "github.com/rossigee/provider-backblaze/apis"
)

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatalf("corev1.AddToScheme: %v", err)
	}
	if err := providerapis.AddToScheme(s); err != nil {
		t.Fatalf("apis.AddToScheme: %v", err)
	}
	return s
}

func TestReconcile_MarksAvailable(t *testing.T) {
	s := testScheme(t)
	pc := &v1beta1.ProviderConfig{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "crossplane-system"}}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(pc).WithStatusSubresource(pc).Build()
	r := &reconciler{kube: cl, logger: logr.Discard()}
	res, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "default", Namespace: "crossplane-system"}})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if res.RequeueAfter != 0 {
		t.Errorf("expected no requeue, got %v", res.RequeueAfter)
	}
	got := &v1beta1.ProviderConfig{}
	if err := cl.Get(context.Background(), types.NamespacedName{Name: "default", Namespace: "crossplane-system"}, got); err != nil {
		t.Fatalf("get: %v", err)
	}
	cond := got.GetCondition("Ready")
	if string(cond.Status) != "True" {
		t.Errorf("Ready should be True, got %v (%s)", cond.Status, cond.Message)
	}
}

func TestReconcile_NotFound(t *testing.T) {
	s := testScheme(t)
	cl := fake.NewClientBuilder().WithScheme(s).Build()
	r := &reconciler{kube: cl, logger: logr.Discard()}
	res, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "missing", Namespace: "crossplane-system"}})
	if err != nil {
		t.Fatalf("not-found should be ignored, got %v", err)
	}
	if res.RequeueAfter != 0 {
		t.Errorf("expected no requeue, got %v", res.RequeueAfter)
	}
}
