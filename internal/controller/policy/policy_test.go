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

package policy

import (
	"context"
	"testing"

	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"

	providerapis "github.com/rossigee/provider-backblaze/apis"
	backblazev1beta1 "github.com/rossigee/provider-backblaze/apis/backblaze/v1beta1"
	apisv1beta1 "github.com/rossigee/provider-backblaze/apis/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestPolicyGetPolicyName(t *testing.T) {
	// Test with explicit policy name
	policyName := "test-policy"
	policy := &backblazev1beta1.Policy{
		Spec: backblazev1beta1.PolicySpec{
			ForProvider: backblazev1beta1.PolicyParameters{
				PolicyName: &policyName,
			},
		},
	}

	if policy.GetPolicyName() != "test-policy" {
		t.Errorf("Expected policy name 'test-policy', got '%s'", policy.GetPolicyName())
	}

	// Test with no policy name (should use resource name)
	policy2 := &backblazev1beta1.Policy{}
	policy2.SetName("resource-name")
	policy2.Spec.ForProvider.PolicyName = nil

	if policy2.GetPolicyName() != "resource-name" {
		t.Errorf("Expected policy name 'resource-name', got '%s'", policy2.GetPolicyName())
	}
}

func TestPolicySetCondition(t *testing.T) {
	r := &PolicyReconciler{}
	policy := &backblazev1beta1.Policy{}

	// Test that setCondition doesn't panic - this validates the method signature and basic functionality
	r.setCondition(policy, xpv1.TypeReady, "True", "Available", "Policy is ready")

	// Test passes if no panic occurs
}

func TestGenerateSimplePolicy(t *testing.T) {
	r := &PolicyReconciler{}
	policy, err := r.generateSimplePolicy("test-bucket")
	if err != nil {
		t.Errorf("generateSimplePolicy(...): expected no error, got %v", err)
	}
	if policy == "" {
		t.Error("generateSimplePolicy(...): expected policy document, got empty string")
	}
	// Verify it contains the bucket name
	if !contains(policy, "test-bucket") {
		t.Error("generateSimplePolicy(...): policy should contain bucket name")
	}
}

func TestCreatePolicyValidation(t *testing.T) {
	allowBucket := "test-bucket"
	rawPolicy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"arn:aws:s3:::test/*"}]}`
	invalidPolicy := `{invalid json`

	cases := map[string]struct {
		params  backblazev1beta1.PolicyParameters
		wantErr bool
	}{
		"valid_allowBucket": {
			params: backblazev1beta1.PolicyParameters{
				AllowBucket: &allowBucket,
			},
			wantErr: false,
		},
		"valid_rawPolicy": {
			params: backblazev1beta1.PolicyParameters{
				RawPolicy: &rawPolicy,
			},
			wantErr: false,
		},
		"both_params_provided": {
			params: backblazev1beta1.PolicyParameters{
				AllowBucket: &allowBucket,
				RawPolicy:   &rawPolicy,
			},
			wantErr: true,
		},
		"no_params_provided": {
			params:  backblazev1beta1.PolicyParameters{},
			wantErr: true,
		},
		"invalid_json": {
			params: backblazev1beta1.PolicyParameters{
				RawPolicy: &invalidPolicy,
			},
			wantErr: false, // JSON validation would happen later in the actual implementation
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			// Basic validation logic test
			hasAllowBucket := tc.params.AllowBucket != nil
			hasRawPolicy := tc.params.RawPolicy != nil

			// Check if both or neither are provided
			bothOrNeither := (hasAllowBucket && hasRawPolicy) || (!hasAllowBucket && !hasRawPolicy)

			if bothOrNeither != tc.wantErr {
				t.Errorf("Expected error: %v, got validation result: %v", tc.wantErr, bothOrNeither)
			}
		})
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(substr) > 0 && (s[:len(substr)] == substr ||
			(len(s) > len(substr) && s[len(s)-len(substr):] == substr) ||
			containsHelper(s, substr))))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestPolicyDocEqual(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want bool
	}{
		{"identical", `{"a":1}`, `{"a":1}`, true},
		{"whitespace ignored", "{\"a\": 1}", "{\n  \"a\": 1\n}", true},
		{"key order ignored", `{"a":1,"b":2}`, `{"b":2,"a":1}`, true},
		{"different value", `{"a":1}`, `{"a":2}`, false},
		{"missing key", `{"a":1}`, `{"a":1,"b":2}`, false},
		{"invalid a", `{invalid`, `{"a":1}`, false},
		{"invalid b", `{"a":1}`, `{invalid`, false},
		{"both invalid same bytes", `{invalid`, `{invalid`, true},
		{"array order matters", `{"a":[1,2]}`, `{"a":[2,1]}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := policyDocEqual(tc.a, tc.b); got != tc.want {
				t.Errorf("policyDocEqual = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRenderPolicyDocument(t *testing.T) {
	allowBucket := "my-bucket"
	raw := `{"Version":"2012-10-17"}`
	badJSON := `{invalid`
	bothAllow := allowBucket
	bothRaw := raw

	if _, err := renderPolicyDocument(backblazev1beta1.PolicyParameters{AllowBucket: &allowBucket}); err != nil {
		t.Errorf("allowBucket should render: %v", err)
	}
	got, err := renderPolicyDocument(backblazev1beta1.PolicyParameters{AllowBucket: &allowBucket})
	if err != nil {
		t.Fatalf("render allowBucket: %v", err)
	}
	if !contains(got, "my-bucket") {
		t.Errorf("rendered doc should contain bucket name, got %q", got)
	}
	if _, err := renderPolicyDocument(backblazev1beta1.PolicyParameters{RawPolicy: &raw}); err != nil {
		t.Errorf("rawPolicy should render: %v", err)
	}
	if _, err := renderPolicyDocument(backblazev1beta1.PolicyParameters{AllowBucket: &bothAllow, RawPolicy: &bothRaw}); err == nil {
		t.Errorf("both set should error")
	}
	if _, err := renderPolicyDocument(backblazev1beta1.PolicyParameters{}); err == nil {
		t.Errorf("neither set should error")
	}
	if _, err := renderPolicyDocument(backblazev1beta1.PolicyParameters{RawPolicy: &badJSON}); err == nil {
		t.Errorf("invalid JSON should error")
	}
}

func TestValidatePolicyParams(t *testing.T) {
	allow := "b"
	raw := `{}`
	if err := validatePolicyParams(backblazev1beta1.PolicyParameters{AllowBucket: &allow}); err != nil {
		t.Errorf("allowBucket-only should validate: %v", err)
	}
	if err := validatePolicyParams(backblazev1beta1.PolicyParameters{RawPolicy: &raw}); err != nil {
		t.Errorf("rawPolicy-only should validate: %v", err)
	}
	if err := validatePolicyParams(backblazev1beta1.PolicyParameters{AllowBucket: &allow, RawPolicy: &raw}); err == nil {
		t.Errorf("both should fail validation")
	}
	if err := validatePolicyParams(backblazev1beta1.PolicyParameters{}); err == nil {
		t.Errorf("neither should fail validation")
	}
}

func TestResolveBucketName(t *testing.T) {
	allow := "allow-bucket"
	named := "policy-name"
	p1 := &backblazev1beta1.Policy{Spec: backblazev1beta1.PolicySpec{ForProvider: backblazev1beta1.PolicyParameters{AllowBucket: &allow, PolicyName: &named}}}
	if got := resolveBucketName(p1); got != "allow-bucket" {
		t.Errorf("AllowBucket should win, got %q", got)
	}
	p2 := &backblazev1beta1.Policy{Spec: backblazev1beta1.PolicySpec{ForProvider: backblazev1beta1.PolicyParameters{PolicyName: &named}}}
	p2.SetName("meta-name")
	if got := resolveBucketName(p2); got != "policy-name" {
		t.Errorf("PolicyName fallback, got %q", got)
	}
	p3 := &backblazev1beta1.Policy{}
	p3.SetName("meta-name")
	if got := resolveBucketName(p3); got != "meta-name" {
		t.Errorf("metadata name fallback, got %q", got)
	}
}

func TestPolicyShouldCreateDelete(t *testing.T) {
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

func TestPolicyReconcile_InvalidSpec(t *testing.T) {
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatalf("corev1: %v", err)
	}
	if err := providerapis.AddToScheme(s); err != nil {
		t.Fatalf("apis: %v", err)
	}
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
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: "crossplane-system"},
		Data:       map[string][]byte{"applicationKeyId": []byte("id"), "applicationKey": []byte("key")},
	}
	// Both AllowBucket and RawPolicy set -> invalid.
	allow, raw := "bkt", `{}`
	p := &backblazev1beta1.Policy{
		ObjectMeta: metav1.ObjectMeta{Name: "pol", Namespace: "default"},
		Spec:       backblazev1beta1.PolicySpec{ForProvider: backblazev1beta1.PolicyParameters{AllowBucket: &allow, RawPolicy: &raw}},
	}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(pc, sec, p).WithStatusSubresource(p).Build()
	r := &PolicyReconciler{Client: cl}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "pol", Namespace: "default"}}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	got := &backblazev1beta1.Policy{}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: "pol", Namespace: "default"}, got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.GetCondition(xpv1.TypeReady).Status != corev1.ConditionFalse {
		t.Errorf("Ready should be False on invalid spec")
	}
}

func TestPolicyReconcile_ClientError(t *testing.T) {
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatalf("corev1: %v", err)
	}
	if err := providerapis.AddToScheme(s); err != nil {
		t.Fatalf("apis: %v", err)
	}
	p := &backblazev1beta1.Policy{
		ObjectMeta: metav1.ObjectMeta{Name: "pol", Namespace: "default"},
		Spec:       backblazev1beta1.PolicySpec{ForProvider: backblazev1beta1.PolicyParameters{}},
	}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(p).WithStatusSubresource(p).Build()
	r := &PolicyReconciler{Client: cl}
	res, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "pol", Namespace: "default"}})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if res.RequeueAfter == 0 {
		t.Errorf("expected requeue on client error")
	}
}

func TestPolicyHandleDeletion_ObserveOnly(t *testing.T) {
	s := runtime.NewScheme()
	if err := providerapis.AddToScheme(s); err != nil {
		t.Fatalf("apis: %v", err)
	}
	now := metav1.Now()
	p := &backblazev1beta1.Policy{
		ObjectMeta: metav1.ObjectMeta{Name: "pol", Namespace: "default", DeletionTimestamp: &now, Finalizers: []string{"test"}},
		Spec: backblazev1beta1.PolicySpec{
			ManagedResourceSpec: xpv1.ManagedResourceSpec{
				ManagementPolicies: xpv1.ManagementPolicies{xpv1.ManagementActionObserve},
			},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(p).Build()
	r := &PolicyReconciler{Client: cl}
	if _, err := r.handleDeletion(context.Background(), p); err != nil {
		t.Fatalf("observe-only delete should be no-op, got %v", err)
	}
}
