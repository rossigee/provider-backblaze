package notification

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"

	providerapis "github.com/rossigee/provider-backblaze/apis"
	backblazev1beta1 "github.com/rossigee/provider-backblaze/apis/backblaze/v1beta1"
	apisv1beta1 "github.com/rossigee/provider-backblaze/apis/v1beta1"
	"github.com/rossigee/provider-backblaze/internal/clients"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func strptr(s string) *string { return &s }

func TestNotificationDrift(t *testing.T) {
	base := func() (*clients.B2EventNotification, *backblazev1beta1.BucketNotification) {
		obs := &clients.B2EventNotification{
			Name:        "rule1",
			WebhookURL:  "https://example.com/hook",
			Disabled:    false,
			Description: "desc",
			Events:      []string{"b2:ObjectCreated"},
		}
		n := &backblazev1beta1.BucketNotification{
			Spec: backblazev1beta1.NotificationSpec{
				ForProvider: backblazev1beta1.NotificationParameters{
					Name:        "rule1",
					Events:      []backblazev1beta1.NotificationEvent{backblazev1beta1.NotificationEventBucketCreated},
					WebhookURL:  "https://example.com/hook",
					Description: strptr("desc"),
				},
			},
		}
		return obs, n
	}
	t.Run("no drift", func(t *testing.T) {
		obs, n := base()
		if notificationDrift(obs, n) {
			t.Errorf("expected no drift")
		}
	})
	t.Run("event order ignored", func(t *testing.T) {
		obs, n := base()
		obs.Events = []string{"b2:ObjectDeleted", "b2:ObjectCreated"}
		n.Spec.ForProvider.Events = []backblazev1beta1.NotificationEvent{
			backblazev1beta1.NotificationEventBucketCreated,
			backblazev1beta1.NotificationEventBucketDeleted,
		}
		if notificationDrift(obs, n) {
			t.Errorf("event order should not count as drift")
		}
	})
	cases := []struct {
		name   string
		mutate func(obs *clients.B2EventNotification, n *backblazev1beta1.BucketNotification)
	}{
		{"name", func(obs *clients.B2EventNotification, n *backblazev1beta1.BucketNotification) {
			n.Spec.ForProvider.Name = "other"
		}},
		{"webhook", func(obs *clients.B2EventNotification, n *backblazev1beta1.BucketNotification) {
			n.Spec.ForProvider.WebhookURL = "https://other/hook"
		}},
		{"disabled", func(obs *clients.B2EventNotification, n *backblazev1beta1.BucketNotification) {
			n.Spec.ForProvider.Disabled = true
		}},
		{"description", func(obs *clients.B2EventNotification, n *backblazev1beta1.BucketNotification) {
			n.Spec.ForProvider.Description = strptr("other")
		}},
		{"event count", func(obs *clients.B2EventNotification, n *backblazev1beta1.BucketNotification) {
			n.Spec.ForProvider.Events = append(n.Spec.ForProvider.Events, backblazev1beta1.NotificationEventBucketDeleted)
		}},
		{"event value", func(obs *clients.B2EventNotification, n *backblazev1beta1.BucketNotification) {
			n.Spec.ForProvider.Events = []backblazev1beta1.NotificationEvent{backblazev1beta1.NotificationEventBucketDeleted}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			obs, n := base()
			tc.mutate(obs, n)
			if !notificationDrift(obs, n) {
				t.Errorf("expected drift for %s", tc.name)
			}
		})
	}
}

func TestStringPtrValue(t *testing.T) {
	if got := stringPtrValue(nil); got != "" {
		t.Errorf("nil should give empty string, got %q", got)
	}
	if got := stringPtrValue(strptr("x")); got != "x" {
		t.Errorf("got %q want x", got)
	}
}

func TestCacheRequeue(t *testing.T) {
	if got := cacheRequeue(errors.New("bucket not found")); got != 10*time.Second {
		t.Errorf("not-found should requeue in 10s, got %v", got)
	}
	if got := cacheRequeue(errors.New("boom")); got != time.Minute {
		t.Errorf("other errors should requeue in 1m, got %v", got)
	}
	if got := cacheRequeue(nil); got != time.Minute {
		t.Errorf("nil should requeue in 1m, got %v", got)
	}
}

func TestShouldCreateDelete(t *testing.T) {
	if !shouldCreate(nil) || !shouldDelete(nil) {
		t.Errorf("empty policies should allow all")
	}
	if !shouldCreate(xpv1.ManagementPolicies{xpv1.ManagementActionCreate}) {
		t.Errorf("Create should allow create")
	}
	if shouldCreate(xpv1.ManagementPolicies{xpv1.ManagementActionDelete}) {
		t.Errorf("Delete-only should not allow create")
	}
	if !shouldCreate(xpv1.ManagementPolicies{xpv1.ManagementActionAll}) {
		t.Errorf("* should allow create")
	}
	if !shouldDelete(xpv1.ManagementPolicies{xpv1.ManagementActionDelete}) {
		t.Errorf("Delete should allow delete")
	}
	if shouldDelete(xpv1.ManagementPolicies{xpv1.ManagementActionCreate}) {
		t.Errorf("Create-only should not allow delete")
	}
}

func TestSetReadySyncedEmitNoPanic(t *testing.T) {
	r := &NotificationReconciler{}
	n := &backblazev1beta1.BucketNotification{}
	r.setReady(n, "True", "Available", "ok")
	r.setSynced(n, "True", "ReconcileSuccess", "ok")
	r.emit(n, "Test", errors.New("boom")) // nil recorder, must not panic
}

func b2ListBucketsServer(t *testing.T, buckets []struct {
	BucketID   string
	BucketName string
	AccountID  string
},
) (*httptest.Server, *clients.BackblazeClient) {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/b2api/v3/b2_authorize_account":
			_ = json.NewEncoder(w).Encode(clients.B2AuthorizeAccountResponse{
				AccountID:          "acc-1",
				AuthorizationToken: "tok",
				APIURL:             srv.URL,
				DownloadURL:        srv.URL + "/file",
			})
		case "/b2api/v3/b2_list_buckets":
			type row struct {
				AccountID  string `json:"accountId"`
				BucketID   string `json:"bucketId"`
				BucketName string `json:"bucketName"`
			}
			rows := make([]row, 0, len(buckets))
			for _, b := range buckets {
				rows = append(rows, row{AccountID: b.AccountID, BucketID: b.BucketID, BucketName: b.BucketName})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"buckets": rows})
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srv.Close)
	prev := clients.B2AuthorizeAccountURL
	clients.B2AuthorizeAccountURL = srv.URL + "/b2api/v3/b2_authorize_account"
	t.Cleanup(func() { clients.B2AuthorizeAccountURL = prev })

	c, err := clients.NewBackblazeClient(clients.Config{
		ApplicationKeyID: "id",
		ApplicationKey:   "key",
		Region:           "us-west-001",
	})
	if err != nil {
		t.Fatalf("NewBackblazeClient: %v", err)
	}
	c.HTTPClient = &http.Client{Timeout: 5 * time.Second}
	return srv, c
}

func TestResolveBucket_ByName(t *testing.T) {
	_, svc := b2ListBucketsServer(t, []struct {
		BucketID   string
		BucketName string
		AccountID  string
	}{{BucketID: "b1", BucketName: "my-bucket", AccountID: "acc-1"}})
	n := &backblazev1beta1.BucketNotification{
		Spec: backblazev1beta1.NotificationSpec{
			ForProvider: backblazev1beta1.NotificationParameters{BucketName: strptr("my-bucket")},
		},
	}
	bid, aid, err := resolveBucket(context.Background(), svc, n)
	if err != nil {
		t.Fatalf("resolveBucket: %v", err)
	}
	if bid != "b1" || aid != "acc-1" {
		t.Errorf("got %q/%q want b1/acc-1", bid, aid)
	}
}

func TestResolveBucket_ByID(t *testing.T) {
	_, svc := b2ListBucketsServer(t, []struct {
		BucketID   string
		BucketName string
		AccountID  string
	}{{BucketID: "b9", BucketName: "other", AccountID: "acc-9"}})
	n := &backblazev1beta1.BucketNotification{
		Spec: backblazev1beta1.NotificationSpec{
			ForProvider: backblazev1beta1.NotificationParameters{BucketID: strptr("b9")},
		},
	}
	bid, aid, err := resolveBucket(context.Background(), svc, n)
	if err != nil {
		t.Fatalf("resolveBucket: %v", err)
	}
	if bid != "b9" || aid != "acc-9" {
		t.Errorf("got %q/%q want b9/acc-9", bid, aid)
	}
}

func TestResolveBucket_Errors(t *testing.T) {
	_, svc := b2ListBucketsServer(t, nil)
	n := &backblazev1beta1.BucketNotification{}
	if _, _, err := resolveBucket(context.Background(), svc, n); err == nil {
		t.Errorf("neither bucketId nor bucketName should error")
	}
	n2 := &backblazev1beta1.BucketNotification{
		Spec: backblazev1beta1.NotificationSpec{
			ForProvider: backblazev1beta1.NotificationParameters{BucketName: strptr("missing")},
		},
	}
	if _, _, err := resolveBucket(context.Background(), svc, n2); err == nil {
		t.Errorf("unknown bucket name should error")
	}
}

func notificationTestScheme(t *testing.T) *runtime.Scheme {
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

func TestNotificationReconcile_CreatePath(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/b2api/v3/b2_authorize_account":
			_ = json.NewEncoder(w).Encode(clients.B2AuthorizeAccountResponse{
				AccountID: "acc-1", AuthorizationToken: "tok", APIURL: srv.URL, DownloadURL: srv.URL + "/file",
			})
		case "/b2api/v3/b2_list_buckets":
			_ = json.NewEncoder(w).Encode(map[string]any{"buckets": []any{
				map[string]any{"accountId": "acc-1", "bucketId": "b1", "bucketName": "my-bucket"},
			}})
		case "/b2api/v3/b2_list_event_notifications":
			_ = json.NewEncoder(w).Encode(clients.B2ListEventNotificationsResponse{})
		case "/b2api/v3/b2_create_event_notification":
			var req clients.B2CreateEventNotificationRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("decode create req: %v", err)
			}
			if req.BucketID != "b1" || req.Name != "rule1" {
				t.Errorf("unexpected create req: %+v", req)
			}
			_ = json.NewEncoder(w).Encode(clients.B2EventNotification{
				AccountID: "acc-1", BucketID: "b1", NotificationID: "n-1",
				Name: "rule1", Events: req.Events, WebhookURL: req.WebhookURL,
			})
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	prevAuth := clients.B2AuthorizeAccountURL
	clients.B2AuthorizeAccountURL = srv.URL + "/b2api/v3/b2_authorize_account"
	defer func() { clients.B2AuthorizeAccountURL = prevAuth }()

	s := notificationTestScheme(t)
	pc := &apisv1beta1.ProviderConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "crossplane-system"},
		Spec: apisv1beta1.ProviderConfigSpec{
			BackblazeRegion: "us-west-001",
			Credentials: apisv1beta1.ProviderCredentials{
				Source: "Secret",
				CommonCredentialSelectors: xpv1.CommonCredentialSelectors{
					SecretRef: &xpv1.SecretKeySelector{
						SecretReference: xpv1.SecretReference{Name: "creds", Namespace: "crossplane-system"},
					},
				},
			},
		},
	}
	credSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: "crossplane-system"},
		Data:       map[string][]byte{"applicationKeyId": []byte("id"), "applicationKey": []byte("key")},
	}
	n := &backblazev1beta1.BucketNotification{
		ObjectMeta: metav1.ObjectMeta{Name: "rule1-mr", Namespace: "default"},
		Spec: backblazev1beta1.NotificationSpec{
			ForProvider: backblazev1beta1.NotificationParameters{
				BucketName: strptr("my-bucket"),
				Name:       "rule1",
				Events:     []backblazev1beta1.NotificationEvent{backblazev1beta1.NotificationEventBucketCreated},
				WebhookURL: "https://example.com/hook",
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(pc, credSecret, n).WithStatusSubresource(n).Build()
	r := &NotificationReconciler{Client: cl}
	res, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "rule1-mr", Namespace: "default"}})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if res.RequeueAfter == 0 {
		t.Errorf("expected requeue after success")
	}
	got := &backblazev1beta1.BucketNotification{}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "rule1-mr", Namespace: "default"}, got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status.AtProvider.NotificationID != "n-1" {
		t.Errorf("NotificationID = %q want n-1", got.Status.AtProvider.NotificationID)
	}
	if got.Status.AtProvider.BucketID != "b1" {
		t.Errorf("BucketID = %q want b1", got.Status.AtProvider.BucketID)
	}
	if got.GetCondition(xpv1.TypeReady).Status != corev1.ConditionTrue {
		t.Errorf("Ready should be True, got %v", got.GetCondition(xpv1.TypeReady).Status)
	}
}

func TestNotificationReconcile_ObserveOnlySkipsCreate(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/b2api/v3/b2_authorize_account":
			_ = json.NewEncoder(w).Encode(clients.B2AuthorizeAccountResponse{
				AccountID: "acc-1", AuthorizationToken: "tok", APIURL: srv.URL, DownloadURL: srv.URL + "/file",
			})
		case "/b2api/v3/b2_list_buckets":
			_ = json.NewEncoder(w).Encode(map[string]any{"buckets": []any{
				map[string]any{"accountId": "acc-1", "bucketId": "b1", "bucketName": "my-bucket"},
			}})
		case "/b2api/v3/b2_list_event_notifications":
			_ = json.NewEncoder(w).Encode(clients.B2ListEventNotificationsResponse{})
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	prevAuth := clients.B2AuthorizeAccountURL
	clients.B2AuthorizeAccountURL = srv.URL + "/b2api/v3/b2_authorize_account"
	defer func() { clients.B2AuthorizeAccountURL = prevAuth }()

	s := notificationTestScheme(t)
	pc := &apisv1beta1.ProviderConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "crossplane-system"},
		Spec: apisv1beta1.ProviderConfigSpec{
			Credentials: apisv1beta1.ProviderCredentials{
				Source: "Secret",
				CommonCredentialSelectors: xpv1.CommonCredentialSelectors{
					SecretRef: &xpv1.SecretKeySelector{
						SecretReference: xpv1.SecretReference{Name: "creds", Namespace: "crossplane-system"},
					},
				},
			},
		},
	}
	credSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: "crossplane-system"},
		Data:       map[string][]byte{"applicationKeyId": []byte("id"), "applicationKey": []byte("key")},
	}
	n := &backblazev1beta1.BucketNotification{
		ObjectMeta: metav1.ObjectMeta{Name: "mr", Namespace: "default"},
		Spec: backblazev1beta1.NotificationSpec{
			ManagedResourceSpec: xpv1.ManagedResourceSpec{
				ManagementPolicies: xpv1.ManagementPolicies{xpv1.ManagementActionObserve},
			},
			ForProvider: backblazev1beta1.NotificationParameters{
				BucketName: strptr("my-bucket"),
				Name:       "rule1",
				Events:     []backblazev1beta1.NotificationEvent{backblazev1beta1.NotificationEventBucketCreated},
				WebhookURL: "https://example.com/hook",
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(pc, credSecret, n).WithStatusSubresource(n).Build()
	r := &NotificationReconciler{Client: cl}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "mr", Namespace: "default"}}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	got := &backblazev1beta1.BucketNotification{}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "mr", Namespace: "default"}, got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status.AtProvider.NotificationID != "" {
		t.Errorf("observe-only should not create, got %q", got.Status.AtProvider.NotificationID)
	}
}

func TestNotificationHandleDeletion_ObserveOnly(t *testing.T) {
	s := notificationTestScheme(t)
	now := metav1.Now()
	n := &backblazev1beta1.BucketNotification{
		ObjectMeta: metav1.ObjectMeta{Name: "n", Namespace: "default", DeletionTimestamp: &now, Finalizers: []string{"test"}},
		Spec: backblazev1beta1.NotificationSpec{
			ManagedResourceSpec: xpv1.ManagedResourceSpec{
				ManagementPolicies: xpv1.ManagementPolicies{xpv1.ManagementActionObserve},
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(n).Build()
	r := &NotificationReconciler{Client: cl}
	if _, err := r.handleDeletion(context.Background(), n); err != nil {
		t.Fatalf("observe-only delete should be no-op, got %v", err)
	}
}

func TestNotificationHandleDeletion_NoID(t *testing.T) {
	s := notificationTestScheme(t)
	now := metav1.Now()
	n := &backblazev1beta1.BucketNotification{
		ObjectMeta: metav1.ObjectMeta{Name: "n", Namespace: "default", DeletionTimestamp: &now, Finalizers: []string{"test"}},
	}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(n).Build()
	r := &NotificationReconciler{Client: cl}
	if _, err := r.handleDeletion(context.Background(), n); err != nil {
		t.Fatalf("empty notification ID should be no-op, got %v", err)
	}
}

func TestNotificationReconcile_UpdateDrift(t *testing.T) {
	var updateCalls int32
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/b2api/v3/b2_authorize_account":
			_ = json.NewEncoder(w).Encode(clients.B2AuthorizeAccountResponse{
				AccountID: "acc-1", AuthorizationToken: "tok", APIURL: srv.URL, DownloadURL: srv.URL + "/file",
			})
		case "/b2api/v3/b2_list_buckets":
			_ = json.NewEncoder(w).Encode(map[string]any{"buckets": []any{
				map[string]any{"accountId": "acc-1", "bucketId": "b1", "bucketName": "my-bucket"},
			}})
		case "/b2api/v3/b2_list_event_notifications":
			_ = json.NewEncoder(w).Encode(clients.B2ListEventNotificationsResponse{Notifications: []clients.B2EventNotification{
				{AccountID: "acc-1", BucketID: "b1", NotificationID: "n-1", Name: "rule1",
					Events: []string{"b2:ObjectCreated"}, WebhookURL: "https://old.example.com/hook"},
			}})
		case "/b2api/v3/b2_update_event_notification":
			atomic.AddInt32(&updateCalls, 1)
			var req clients.B2UpdateEventNotificationRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.WebhookURL != "https://new.example.com/hook" {
				t.Errorf("expected updated webhook, got %+v", req)
			}
			_ = json.NewEncoder(w).Encode(clients.B2EventNotification{
				AccountID: "acc-1", BucketID: "b1", NotificationID: "n-1", Name: "rule1",
			})
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	prevAuth := clients.B2AuthorizeAccountURL
	clients.B2AuthorizeAccountURL = srv.URL + "/b2api/v3/b2_authorize_account"
	defer func() { clients.B2AuthorizeAccountURL = prevAuth }()

	s := notificationTestScheme(t)
	pc := &apisv1beta1.ProviderConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "crossplane-system"},
		Spec: apisv1beta1.ProviderConfigSpec{
			Credentials: apisv1beta1.ProviderCredentials{
				Source: "Secret",
				CommonCredentialSelectors: xpv1.CommonCredentialSelectors{
					SecretRef: &xpv1.SecretKeySelector{
						SecretReference: xpv1.SecretReference{Name: "creds", Namespace: "crossplane-system"},
					},
				},
			},
		},
	}
	credSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: "crossplane-system"},
		Data:       map[string][]byte{"applicationKeyId": []byte("id"), "applicationKey": []byte("key")},
	}
	n := &backblazev1beta1.BucketNotification{
		ObjectMeta: metav1.ObjectMeta{Name: "rule1-mr", Namespace: "default"},
		Spec: backblazev1beta1.NotificationSpec{
			ForProvider: backblazev1beta1.NotificationParameters{
				BucketName: strptr("my-bucket"),
				Name:       "rule1",
				Events:     []backblazev1beta1.NotificationEvent{backblazev1beta1.NotificationEventBucketCreated},
				WebhookURL: "https://new.example.com/hook",
			},
		},
		Status: backblazev1beta1.NotificationStatus{
			AtProvider: backblazev1beta1.NotificationObservation{NotificationID: "n-1", BucketID: "b1"},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(pc, credSecret, n).WithStatusSubresource(n).Build()
	r := &NotificationReconciler{Client: cl}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "rule1-mr", Namespace: "default"}}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if atomic.LoadInt32(&updateCalls) != 1 {
		t.Errorf("expected 1 update call, got %d", updateCalls)
	}
}
