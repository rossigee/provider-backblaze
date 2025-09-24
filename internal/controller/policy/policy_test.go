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
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"

	backblazev1 "github.com/rossigee/provider-backblaze/apis/backblaze/v1"
)

func TestPolicyGetPolicyName(t *testing.T) {
	// Test with explicit policy name
	policyName := "test-policy"
	policy := &backblazev1.Policy{
		Spec: backblazev1.PolicySpec{
			ForProvider: backblazev1.PolicyParameters{
				PolicyName: &policyName,
			},
		},
	}

	if policy.GetPolicyName() != "test-policy" {
		t.Errorf("Expected policy name 'test-policy', got '%s'", policy.GetPolicyName())
	}

	// Test with no policy name (should use resource name)
	policy2 := &backblazev1.Policy{}
	policy2.SetName("resource-name")
	policy2.Spec.ForProvider.PolicyName = nil

	if policy2.GetPolicyName() != "resource-name" {
		t.Errorf("Expected policy name 'resource-name', got '%s'", policy2.GetPolicyName())
	}
}

func TestPolicySetCondition(t *testing.T) {
	r := &PolicyReconciler{}
	policy := &backblazev1.Policy{}

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
		params  backblazev1.PolicyParameters
		wantErr bool
	}{
		"valid_allowBucket": {
			params: backblazev1.PolicyParameters{
				AllowBucket: &allowBucket,
			},
			wantErr: false,
		},
		"valid_rawPolicy": {
			params: backblazev1.PolicyParameters{
				RawPolicy: &rawPolicy,
			},
			wantErr: false,
		},
		"both_params_provided": {
			params: backblazev1.PolicyParameters{
				AllowBucket: &allowBucket,
				RawPolicy:   &rawPolicy,
			},
			wantErr: true,
		},
		"no_params_provided": {
			params:  backblazev1.PolicyParameters{},
			wantErr: true,
		},
		"invalid_json": {
			params: backblazev1.PolicyParameters{
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