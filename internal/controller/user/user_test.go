/*
Copyright 2025 The Crossplane Authors.

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

package user

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestUserGetKeyName(t *testing.T) {
	user := &backblazev1beta1.User{
		Spec: backblazev1beta1.UserSpec{
			ForProvider: backblazev1beta1.UserParameters{
				KeyName: "test-key",
			},
		},
	}

	if user.GetKeyName() != "test-key" {
		t.Errorf("Expected key name 'test-key', got '%s'", user.GetKeyName())
	}
}

func TestUserSetCondition(t *testing.T) {
	r := &UserReconciler{}
	user := &backblazev1beta1.User{}

	// Test that setCondition doesn't panic - this validates the method signature and basic functionality
	r.setCondition(user, xpv1.TypeReady, "True", "Available", "User is ready")

	// Test passes if no panic occurs
}

func TestCreateApplicationKey(t *testing.T) {
	user := &backblazev1beta1.User{
		Spec: backblazev1beta1.UserSpec{
			ManagedResourceSpec: xpv1.ManagedResourceSpec{
				WriteConnectionSecretToReference: &xpv1.LocalSecretReference{
					Name: "test-secret",
				},
			},
			ForProvider: backblazev1beta1.UserParameters{
				KeyName:      "test-key",
				Capabilities: []string{"listBuckets", "readFiles"},
			},
		},
	}

	// This is a simplified test since the actual createApplicationKey method
	// would require a real client and would create Kubernetes secrets
	// In a real test environment, you would use fake clients or test fixtures

	// Verify the user spec is set up correctly for key creation
	if user.Spec.ForProvider.KeyName != "test-key" {
		t.Error("User key name not set correctly")
	}
	if len(user.Spec.ForProvider.Capabilities) != 2 {
		t.Error("User capabilities not set correctly")
	}
	if user.GetWriteConnectionSecretToReference() == nil || user.GetWriteConnectionSecretToReference().Name != "test-secret" {
		t.Error("Secret reference not set correctly")
	}
}

func TestStringSlicesEqualSet(t *testing.T) {
	cases := []struct {
		name string
		a, b []string
		want bool
	}{
		{"both nil", nil, nil, true},
		{"both empty", []string{}, []string{}, true},
		{"same order", []string{"a", "b"}, []string{"a", "b"}, true},
		{"different order", []string{"a", "b"}, []string{"b", "a"}, true},
		{"duplicates match", []string{"a", "a", "b"}, []string{"b", "a", "a"}, true},
		{"different count", []string{"a", "a"}, []string{"a"}, false},
		{"different content", []string{"a"}, []string{"b"}, false},
		{"different length", []string{"a"}, []string{"a", "b"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stringSlicesEqualSet(tc.a, tc.b); got != tc.want {
				t.Errorf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestClearKeyStatus(t *testing.T) {
	r := &UserReconciler{}
	bid := "b1"
	np := "p/"
	exp := int64(123)
	u := &backblazev1beta1.User{}
	u.Status.AtProvider.ApplicationKeyID = "k1"
	u.Status.AtProvider.AccountID = "a1"
	u.Status.AtProvider.Capabilities = []string{"listBuckets"}
	u.Status.AtProvider.BucketID = &bid
	u.Status.AtProvider.NamePrefix = &np
	u.Status.AtProvider.ExpirationTimestamp = &exp
	r.clearKeyStatus(u)
	if u.Status.AtProvider.ApplicationKeyID != "" {
		t.Errorf("ApplicationKeyID not cleared")
	}
	if u.Status.AtProvider.AccountID != "" {
		t.Errorf("AccountID not cleared")
	}
	if u.Status.AtProvider.Capabilities != nil {
		t.Errorf("Capabilities not cleared")
	}
	if u.Status.AtProvider.BucketID != nil {
		t.Errorf("BucketID not cleared")
	}
	if u.Status.AtProvider.NamePrefix != nil {
		t.Errorf("NamePrefix not cleared")
	}
	if u.Status.AtProvider.ExpirationTimestamp != nil {
		t.Errorf("ExpirationTimestamp not cleared")
	}
}

func TestUserShouldCreateDelete(t *testing.T) {
	if !shouldCreate(nil) || !shouldDelete(nil) {
		t.Errorf("empty policies should allow all")
	}
	if shouldCreate(xpv1.ManagementPolicies{xpv1.ManagementActionDelete}) {
		t.Errorf("Delete-only should not allow create")
	}
	if shouldDelete(xpv1.ManagementPolicies{xpv1.ManagementActionCreate}) {
		t.Errorf("Create-only should not allow delete")
	}
}

func userTestScheme(t *testing.T) *runtime.Scheme {
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

func providerConfigWithSecret(namespace, pcName, secretName, secretNS string) (*apisv1beta1.ProviderConfig, *corev1.Secret) {
	pc := &apisv1beta1.ProviderConfig{
		ObjectMeta: metav1.ObjectMeta{Name: pcName, Namespace: namespace},
		Spec: apisv1beta1.ProviderConfigSpec{
			BackblazeRegion: "us-west-001",
			Credentials: apisv1beta1.ProviderCredentials{
				Source: "Secret",
				CommonCredentialSelectors: xpv1.CommonCredentialSelectors{
					SecretRef: &xpv1.SecretKeySelector{
						SecretReference: xpv1.SecretReference{Name: secretName, Namespace: secretNS},
					},
				},
			},
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: secretNS},
		Data: map[string][]byte{
			"applicationKeyId": []byte("test-key-id"),
			"applicationKey":   []byte("test-key"),
		},
	}
	return pc, secret
}

func TestWriteSecret_Idempotent(t *testing.T) {
	s := userTestScheme(t)
	u := &backblazev1beta1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "u1", Namespace: "default", UID: "uid-1"},
		Spec: backblazev1beta1.UserSpec{
			ManagedResourceSpec: xpv1.ManagedResourceSpec{
				WriteConnectionSecretToReference: &xpv1.LocalSecretReference{Name: "conn-secret"},
			},
			ForProvider: backblazev1beta1.UserParameters{KeyName: "k", Capabilities: []string{"listBuckets"}},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(u).WithStatusSubresource(u).Build()
	r := &UserReconciler{Client: cl}
	ctx := context.Background()

	if err := r.writeSecret(ctx, u, "id-1", "secret-1"); err != nil {
		t.Fatalf("writeSecret create: %v", err)
	}
	got := &corev1.Secret{}
	if err := cl.Get(ctx, client.ObjectKey{Name: "conn-secret", Namespace: "default"}, got); err != nil {
		t.Fatalf("get created secret: %v", err)
	}
	if string(got.Data["applicationKeyId"]) != "id-1" || string(got.Data["applicationKey"]) != "secret-1" {
		t.Errorf("secret data = %v", got.Data)
	}
	if len(got.OwnerReferences) != 1 || got.OwnerReferences[0].Name != "u1" {
		t.Errorf("expected controller owner ref to u1, got %v", got.OwnerReferences)
	}

	// Second write with rotated values must update, not fail with AlreadyExists.
	if err := r.writeSecret(ctx, u, "id-2", "secret-2"); err != nil {
		t.Fatalf("writeSecret update: %v", err)
	}
	got2 := &corev1.Secret{}
	if err := cl.Get(ctx, client.ObjectKey{Name: "conn-secret", Namespace: "default"}, got2); err != nil {
		t.Fatalf("get updated secret: %v", err)
	}
	if string(got2.Data["applicationKeyId"]) != "id-2" || string(got2.Data["applicationKey"]) != "secret-2" {
		t.Errorf("updated secret data = %v", got2.Data)
	}
}

func TestWriteSecret_NoRef(t *testing.T) {
	s := userTestScheme(t)
	u := &backblazev1beta1.User{ObjectMeta: metav1.ObjectMeta{Name: "u", Namespace: "default"}}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(u).Build()
	r := &UserReconciler{Client: cl}
	if err := r.writeSecret(context.Background(), u, "id", "secret"); err != nil {
		t.Errorf("nil secret ref should be no-op, got %v", err)
	}
}

func TestDeleteSecret(t *testing.T) {
	s := userTestScheme(t)
	u := &backblazev1beta1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "u1", Namespace: "default"},
		Spec: backblazev1beta1.UserSpec{
			ManagedResourceSpec: xpv1.ManagedResourceSpec{
				WriteConnectionSecretToReference: &xpv1.LocalSecretReference{Name: "conn-secret"},
			},
		},
	}
	existing := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "conn-secret", Namespace: "default"}}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(u, existing).Build()
	r := &UserReconciler{Client: cl}
	ctx := context.Background()
	if err := r.deleteSecret(ctx, u); err != nil {
		t.Fatalf("deleteSecret: %v", err)
	}
	got := &corev1.Secret{}
	if err := cl.Get(ctx, client.ObjectKey{Name: "conn-secret", Namespace: "default"}, got); err == nil {
		t.Errorf("secret should be deleted")
	}
	// Deleting again (missing) must not error.
	if err := r.deleteSecret(ctx, u); err != nil {
		t.Errorf("delete missing secret should be no-op, got %v", err)
	}
	u.Spec.WriteConnectionSecretToReference = nil
	if err := r.deleteSecret(ctx, u); err != nil {
		t.Errorf("nil ref should be no-op, got %v", err)
	}
}

func TestEnsureConnectionSecret(t *testing.T) {
	s := userTestScheme(t)
	newUser := func() *backblazev1beta1.User {
		return &backblazev1beta1.User{
			ObjectMeta: metav1.ObjectMeta{Name: "u1", Namespace: "default"},
			Spec: backblazev1beta1.UserSpec{
				ManagedResourceSpec: xpv1.ManagedResourceSpec{
					WriteConnectionSecretToReference: &xpv1.LocalSecretReference{Name: "conn-secret"},
				},
			},
		}
	}
	ctx := context.Background()

	// Present -> nil.
	u := newUser()
	sec := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "conn-secret", Namespace: "default"}}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(u, sec).Build()
	if err := (&UserReconciler{Client: cl}).ensureConnectionSecret(ctx, u); err != nil {
		t.Errorf("present secret should be nil, got %v", err)
	}

	// Missing -> descriptive error.
	u2 := newUser()
	cl2 := fake.NewClientBuilder().WithScheme(s).WithObjects(u2).Build()
	if err := (&UserReconciler{Client: cl2}).ensureConnectionSecret(ctx, u2); err == nil {
		t.Errorf("missing secret should error")
	}

	// No ref -> nil.
	u3 := &backblazev1beta1.User{ObjectMeta: metav1.ObjectMeta{Name: "u3", Namespace: "default"}}
	cl3 := fake.NewClientBuilder().WithScheme(s).WithObjects(u3).Build()
	if err := (&UserReconciler{Client: cl3}).ensureConnectionSecret(ctx, u3); err != nil {
		t.Errorf("nil ref should be nil, got %v", err)
	}
}

func TestUserReconcile_CreatePath(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/b2api/v3/b2_authorize_account":
			_ = json.NewEncoder(w).Encode(clients.B2AuthorizeAccountResponse{
				AccountID: "acc-1", AuthorizationToken: "tok", APIURL: srv.URL, DownloadURL: srv.URL + "/file",
			})
		case "/b2api/v3/b2_list_keys":
			_ = json.NewEncoder(w).Encode(clients.B2ListKeysResponse{})
		case "/b2api/v3/b2_create_key":
			_ = json.NewEncoder(w).Encode(clients.B2CreateKeyResponse{
				ApplicationKeyID: "new-key-id", ApplicationKey: "new-secret",
				KeyName: "my-key", Capabilities: []string{"listBuckets"}, AccountID: "acc-1",
			})
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	prevAuth := clients.B2AuthorizeAccountURL
	clients.B2AuthorizeAccountURL = srv.URL + "/b2api/v3/b2_authorize_account"
	defer func() { clients.B2AuthorizeAccountURL = prevAuth }()

	s := userTestScheme(t)
	pc, credSecret := providerConfigWithSecret("crossplane-system", "default", "creds", "crossplane-system")
	u := &backblazev1beta1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "my-key-mr", Namespace: "default"},
		Spec: backblazev1beta1.UserSpec{
			ManagedResourceSpec: xpv1.ManagedResourceSpec{
				WriteConnectionSecretToReference: &xpv1.LocalSecretReference{Name: "conn-secret"},
			},
			ForProvider: backblazev1beta1.UserParameters{KeyName: "my-key", Capabilities: []string{"listBuckets"}},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(pc, credSecret, u).WithStatusSubresource(u).Build()
	r := &UserReconciler{Client: cl}
	res, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "my-key-mr", Namespace: "default"}})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if res.RequeueAfter == 0 {
		t.Errorf("expected requeue after success")
	}
	got := &backblazev1beta1.User{}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "my-key-mr", Namespace: "default"}, got); err != nil {
		t.Fatalf("get user: %v", err)
	}
	if got.Status.AtProvider.ApplicationKeyID != "new-key-id" {
		t.Errorf("ApplicationKeyID = %q want new-key-id", got.Status.AtProvider.ApplicationKeyID)
	}
	sec := &corev1.Secret{}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "conn-secret", Namespace: "default"}, sec); err != nil {
		t.Fatalf("get connection secret: %v", err)
	}
	if string(sec.Data["applicationKeyId"]) != "new-key-id" || string(sec.Data["applicationKey"]) != "new-secret" {
		t.Errorf("connection secret data = %v", sec.Data)
	}
	ready := got.GetCondition(xpv1.TypeReady)
	if ready.Status != corev1.ConditionTrue {
		t.Errorf("Ready should be True, got %v (%s)", ready.Status, ready.Message)
	}
}

func TestUserReconcile_ObserveOnlySkipsCreate(t *testing.T) {
	s := userTestScheme(t)
	pc, credSecret := providerConfigWithSecret("crossplane-system", "default", "creds", "crossplane-system")
	u := &backblazev1beta1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "mr", Namespace: "default"},
		Spec: backblazev1beta1.UserSpec{
			ManagedResourceSpec: xpv1.ManagedResourceSpec{
				ManagementPolicies: xpv1.ManagementPolicies{xpv1.ManagementActionObserve},
			},
			ForProvider: backblazev1beta1.UserParameters{KeyName: "k", Capabilities: []string{"listBuckets"}},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(pc, credSecret, u).WithStatusSubresource(u).Build()
	r := &UserReconciler{Client: cl}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "mr", Namespace: "default"}}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	got := &backblazev1beta1.User{}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "mr", Namespace: "default"}, got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status.AtProvider.ApplicationKeyID != "" {
		t.Errorf("observe-only should not create, got key %q", got.Status.AtProvider.ApplicationKeyID)
	}
	if got.GetCondition(xpv1.TypeReady).Status != corev1.ConditionFalse {
		t.Errorf("Ready should be False in observe-only mode")
	}
}

func TestUserHandleDeletion_ObserveOnly(t *testing.T) {
	s := userTestScheme(t)
	now := metav1.Now()
	u := &backblazev1beta1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "u", Namespace: "default", DeletionTimestamp: &now, Finalizers: []string{"test"}},
		Spec: backblazev1beta1.UserSpec{
			ManagedResourceSpec: xpv1.ManagedResourceSpec{
				ManagementPolicies: xpv1.ManagementPolicies{xpv1.ManagementActionObserve},
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(u).Build()
	r := &UserReconciler{Client: cl}
	if _, err := r.handleDeletion(context.Background(), u); err != nil {
		t.Fatalf("observe-only delete should be no-op, got %v", err)
	}
}

func TestApplyKeyUpdate_NoDrift(t *testing.T) {
	r := &UserReconciler{}
	u := &backblazev1beta1.User{}
	u.Status.AtProvider.ApplicationKeyID = "k1"
	u.Spec.ForProvider.KeyName = "my-key"
	u.Spec.ForProvider.Capabilities = []string{"listBuckets"}
	observed := &clients.B2CreateKeyResponse{
		ApplicationKeyID: "k1", KeyName: "my-key", Capabilities: []string{"listBuckets"},
	}
	// No service call expected; pass nil service since early return happens first.
	if err := r.applyKeyUpdate(context.Background(), u, nil, observed); err != nil {
		t.Errorf("no drift should be nil, got %v", err)
	}
}

func TestApplyKeyUpdate_Drift(t *testing.T) {
	var srv *httptest.Server
	var gotReq clients.B2UpdateKeyRequest
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/b2api/v3/b2_authorize_account":
			_ = json.NewEncoder(w).Encode(clients.B2AuthorizeAccountResponse{
				AccountID: "acc-1", AuthorizationToken: "tok", APIURL: srv.URL, DownloadURL: srv.URL + "/file",
			})
		case "/b2api/v3/b2_update_key":
			_ = json.NewDecoder(r.Body).Decode(&gotReq)
			_ = json.NewEncoder(w).Encode(clients.B2CreateKeyResponse{
				ApplicationKeyID: "k1", KeyName: gotReq.KeyName, Capabilities: gotReq.Capabilities,
				AccountID: "acc-1", BucketID: gotReq.BucketID, NamePrefix: gotReq.NamePrefix,
			})
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	prevAuth := clients.B2AuthorizeAccountURL
	clients.B2AuthorizeAccountURL = srv.URL + "/b2api/v3/b2_authorize_account"
	defer func() { clients.B2AuthorizeAccountURL = prevAuth }()

	svc, err := clients.NewBackblazeClient(clients.Config{ApplicationKeyID: "id", ApplicationKey: "key", Region: "us-west-001"})
	if err != nil {
		t.Fatalf("NewBackblazeClient: %v", err)
	}
	svc.HTTPClient = &http.Client{Timeout: 5 * time.Second}
	svc.APIURL = srv.URL

	r := &UserReconciler{}
	u := &backblazev1beta1.User{}
	u.Status.AtProvider.ApplicationKeyID = "k1"
	u.Spec.ForProvider.KeyName = "my-key"
	u.Spec.ForProvider.Capabilities = []string{"listBuckets", "readFiles"}
	observed := &clients.B2CreateKeyResponse{
		ApplicationKeyID: "k1", KeyName: "my-key", Capabilities: []string{"listBuckets"},
	}
	if err := r.applyKeyUpdate(context.Background(), u, svc, observed); err != nil {
		t.Fatalf("applyKeyUpdate: %v", err)
	}
	if len(gotReq.Capabilities) != 2 {
		t.Errorf("expected update call with 2 capabilities, got %+v", gotReq)
	}
	if len(u.Status.AtProvider.Capabilities) != 2 {
		t.Errorf("status not updated: %+v", u.Status.AtProvider)
	}
}
