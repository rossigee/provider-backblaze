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

package v1

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPolicyTypeDefinition(t *testing.T) {
	policy := &Policy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "backblaze.crossplane.io/v1",
			Kind:       "Policy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-policy",
		},
		Spec: PolicySpec{
			ForProvider: PolicyParameters{
				PolicyName:  "test-bucket-policy",
				AllowBucket: "test-bucket",
				Description: "Allow all operations for test bucket",
			},
		},
	}

	if policy.Name != "test-policy" {
		t.Errorf("Expected name test-policy, got %v", policy.Name)
	}

	if policy.Spec.ForProvider.PolicyName != "test-bucket-policy" {
		t.Errorf("Expected policy name test-bucket-policy, got %v", policy.Spec.ForProvider.PolicyName)
	}

	if policy.Spec.ForProvider.AllowBucket != "test-bucket" {
		t.Errorf("Expected allow bucket test-bucket, got %v", policy.Spec.ForProvider.AllowBucket)
	}

	if policy.Spec.ForProvider.Description == "" {
		t.Error("Expected description to be non-empty")
	}
}

func TestPolicyAllowBucket(t *testing.T) {
	policy := PolicyParameters{
		PolicyName:  "simple-policy",
		AllowBucket: "test-bucket",
		Description: "Allow all operations for test bucket",
	}

	if policy.AllowBucket != "test-bucket" {
		t.Errorf("Expected AllowBucket test-bucket, got %v", policy.AllowBucket)
	}

	if policy.PolicyName != "simple-policy" {
		t.Errorf("Expected PolicyName simple-policy, got %v", policy.PolicyName)
	}

	if policy.Description == "" {
		t.Error("Expected Description to be non-empty")
	}

	// Test that RawPolicy is not set when using AllowBucket
	if policy.RawPolicy != "" {
		t.Error("Expected RawPolicy to be empty when using AllowBucket")
	}
}

func TestPolicyRawDocument(t *testing.T) {
	policyDoc := `{
		"Version": "2012-10-17",
		"Statement": [
			{
				"Sid": "AllowPublicRead",
				"Effect": "Allow",
				"Principal": "*",
				"Action": "s3:GetObject",
				"Resource": "arn:aws:s3:::my-bucket/*"
			},
			{
				"Sid": "AllowUserWrite",
				"Effect": "Allow",
				"Principal": {
					"AWS": "arn:aws:iam::123456789012:user/myuser"
				},
				"Action": [
					"s3:PutObject",
					"s3:DeleteObject"
				],
				"Resource": "arn:aws:s3:::my-bucket/*"
			}
		]
	}`

	policy := PolicyParameters{
		PolicyName:  "advanced-policy",
		RawPolicy:   policyDoc,
		Description: "Advanced S3-compatible policy",
	}

	if policy.RawPolicy != policyDoc {
		t.Error("RawPolicy was not set correctly")
	}

	// Test that raw policy doesn't have AllowBucket set
	if policy.AllowBucket != "" {
		t.Error("Expected AllowBucket to be empty when using RawPolicy")
	}
}

func TestPolicyList(t *testing.T) {
	policyList := PolicyList{
		Items: []Policy{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "policy-1",
				},
				Spec: PolicySpec{
					ForProvider: PolicyParameters{
						PolicyName:  "bucket-1-policy",
						AllowBucket: "bucket-1",
						Description: "Allow all operations for bucket-1",
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "policy-2",
				},
				Spec: PolicySpec{
					ForProvider: PolicyParameters{
						PolicyName:  "bucket-2-policy",
						RawPolicy:   `{"Version": "2012-10-17", "Statement": []}`,
						Description: "Custom policy for bucket-2",
					},
				},
			},
		},
	}

	if len(policyList.Items) != 2 {
		t.Errorf("Expected 2 policies, got %d", len(policyList.Items))
	}

	// Verify each policy has the expected structure
	for i, policy := range policyList.Items {
		if policy.Spec.ForProvider.PolicyName == "" {
			t.Errorf("Policy %d has empty policy name", i)
		}
		if policy.Spec.ForProvider.Description == "" {
			t.Errorf("Policy %d has empty description", i)
		}
		// Each policy should have either AllowBucket or RawPolicy set
		if policy.Spec.ForProvider.AllowBucket == "" && policy.Spec.ForProvider.RawPolicy == "" {
			t.Errorf("Policy %d has neither AllowBucket nor RawPolicy set", i)
		}
	}
}

func TestPolicyGroupVersionKind(t *testing.T) {
	// Test that the group version kind constants are properly set
	expectedKind := "Policy"
	if PolicyKind != expectedKind {
		t.Errorf("PolicyKind = %v, want %v", PolicyKind, expectedKind)
	}

	if PolicyGroupVersionKind.Kind != expectedKind {
		t.Errorf("PolicyGroupVersionKind.Kind = %v, want %v", PolicyGroupVersionKind.Kind, expectedKind)
	}

	if PolicyGroupVersionKind.Group != Group {
		t.Errorf("PolicyGroupVersionKind.Group = %v, want %v", PolicyGroupVersionKind.Group, Group)
	}
}
