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
	"encoding/json"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	policyv1beta1 "github.com/rossigee/provider-backblaze/apis/policy/v1beta1"
)

func TestGeneratePolicyDocument(t *testing.T) {
	cases := map[string]struct {
		cr      *policyv1beta1.Policy
		want    string
		wantErr bool
	}{
		"AllowBucket": {
			cr: &policyv1beta1.Policy{
				Spec: policyv1beta1.PolicySpec{
					ForProvider: policyv1beta1.PolicyParameters{
						AllowBucket: "test-bucket",
					},
				},
			},
			want:    `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"*"},"Action":"s3:*","Resource":["arn:aws:s3:::test-bucket","arn:aws:s3:::test-bucket/*"]}]}`,
			wantErr: false,
		},
		"RawPolicy": {
			cr: &policyv1beta1.Policy{
				Spec: policyv1beta1.PolicySpec{
					ForProvider: policyv1beta1.PolicyParameters{
						RawPolicy: `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":"*","Action":"s3:GetObject","Resource":"arn:aws:s3:::test-bucket/*"}]}`,
					},
				},
			},
			want:    `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":"*","Action":"s3:GetObject","Resource":"arn:aws:s3:::test-bucket/*"}]}`,
			wantErr: false,
		},
		"InvalidRawPolicy": {
			cr: &policyv1beta1.Policy{
				Spec: policyv1beta1.PolicySpec{
					ForProvider: policyv1beta1.PolicyParameters{
						RawPolicy: `invalid json`,
					},
				},
			},
			wantErr: true,
		},
		"NeitherSet": {
			cr: &policyv1beta1.Policy{
				Spec: policyv1beta1.PolicySpec{
					ForProvider: policyv1beta1.PolicyParameters{},
				},
			},
			wantErr: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := &external{}

			got, err := e.generatePolicyDocument(tc.cr)

			if tc.wantErr && err == nil {
				t.Error("generatePolicyDocument(...): want error, got nil")
			}

			if !tc.wantErr && err != nil {
				t.Errorf("generatePolicyDocument(...): want nil error, got %v", err)
			}

			if !tc.wantErr {
				// Compare JSON structures instead of strings to handle different ordering
				var gotMap, wantMap map[string]interface{}
				if err := json.Unmarshal([]byte(got), &gotMap); err != nil {
					t.Errorf("Failed to parse generated JSON: %v", err)
					return
				}
				if err := json.Unmarshal([]byte(tc.want), &wantMap); err != nil {
					t.Errorf("Failed to parse expected JSON: %v", err)
					return
				}
				
				// Convert back to normalized JSON for comparison
				gotNormalized, _ := json.Marshal(gotMap)
				wantNormalized, _ := json.Marshal(wantMap)
				
				if string(gotNormalized) != string(wantNormalized) {
					t.Errorf("generatePolicyDocument(...): want %v, got %v", string(wantNormalized), string(gotNormalized))
				}
			}
		})
	}
}

func TestGetBucketNameFromPolicy(t *testing.T) {
	cases := map[string]struct {
		cr      *policyv1beta1.Policy
		want    string
		wantErr bool
	}{
		"AllowBucket": {
			cr: &policyv1beta1.Policy{
				Spec: policyv1beta1.PolicySpec{
					ForProvider: policyv1beta1.PolicyParameters{
						AllowBucket: "test-bucket",
					},
				},
			},
			want:    "test-bucket",
			wantErr: false,
		},
		"RawPolicyWithBucketARN": {
			cr: &policyv1beta1.Policy{
				Spec: policyv1beta1.PolicySpec{
					ForProvider: policyv1beta1.PolicyParameters{
						RawPolicy: `{"Statement":[{"Resource":"arn:aws:s3:::my-bucket/*"}]}`,
					},
				},
			},
			want:    "my-bucket",
			wantErr: false,
		},
		"RawPolicyWithBucketARNArray": {
			cr: &policyv1beta1.Policy{
				Spec: policyv1beta1.PolicySpec{
					ForProvider: policyv1beta1.PolicyParameters{
						RawPolicy: `{"Statement":[{"Resource":["arn:aws:s3:::my-bucket","arn:aws:s3:::my-bucket/*"]}]}`,
					},
				},
			},
			want:    "my-bucket",
			wantErr: false,
		},
		"InvalidRawPolicy": {
			cr: &policyv1beta1.Policy{
				Spec: policyv1beta1.PolicySpec{
					ForProvider: policyv1beta1.PolicyParameters{
						RawPolicy: `invalid json`,
					},
				},
			},
			wantErr: true,
		},
		"NeitherSet": {
			cr: &policyv1beta1.Policy{
				Spec: policyv1beta1.PolicySpec{
					ForProvider: policyv1beta1.PolicyParameters{},
				},
			},
			wantErr: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := &external{}

			got, err := e.getBucketNameFromPolicy(tc.cr)

			if tc.wantErr && err == nil {
				t.Error("getBucketNameFromPolicy(...): want error, got nil")
			}

			if !tc.wantErr && err != nil {
				t.Errorf("getBucketNameFromPolicy(...): want nil error, got %v", err)
			}

			if !tc.wantErr && got != tc.want {
				t.Errorf("getBucketNameFromPolicy(...): want %v, got %v", tc.want, got)
			}
		})
	}
}

func TestExtractBucketFromARN(t *testing.T) {
	cases := map[string]struct {
		arn  string
		want string
	}{
		"BucketOnly": {
			arn:  "arn:aws:s3:::my-bucket",
			want: "my-bucket",
		},
		"BucketWithPath": {
			arn:  "arn:aws:s3:::my-bucket/path/to/object",
			want: "my-bucket",
		},
		"BucketWithWildcard": {
			arn:  "arn:aws:s3:::my-bucket/*",
			want: "my-bucket",
		},
		"NotS3ARN": {
			arn:  "arn:aws:iam::123456789012:user/username",
			want: "",
		},
		"InvalidARN": {
			arn:  "not-an-arn",
			want: "",
		},
		"EmptyARN": {
			arn:  "",
			want: "",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := &external{}
			got := e.extractBucketFromARN(tc.arn)
			if got != tc.want {
				t.Errorf("extractBucketFromARN(%v): want %v, got %v", tc.arn, tc.want, got)
			}
		})
	}
}

func TestIsPolicyUpToDate(t *testing.T) {
	cases := map[string]struct {
		current string
		desired string
		want    bool
	}{
		"Identical": {
			current: `{"Version":"2012-10-17","Statement":[]}`,
			desired: `{"Version":"2012-10-17","Statement":[]}`,
			want:    true,
		},
		"DifferentFormatting": {
			current: `{"Version": "2012-10-17", "Statement": []}`,
			desired: `{"Version":"2012-10-17","Statement":[]}`,
			want:    true,
		},
		"Different": {
			current: `{"Version":"2012-10-17","Statement":[{"Effect":"Allow"}]}`,
			desired: `{"Version":"2012-10-17","Statement":[{"Effect":"Deny"}]}`,
			want:    false,
		},
		"InvalidCurrent": {
			current: `invalid json`,
			desired: `{"Version":"2012-10-17","Statement":[]}`,
			want:    false,
		},
		"InvalidDesired": {
			current: `{"Version":"2012-10-17","Statement":[]}`,
			desired: `invalid json`,
			want:    false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := &external{}
			got := e.isPolicyUpToDate(tc.current, tc.desired)
			if got != tc.want {
				t.Errorf("isPolicyUpToDate(%v, %v): want %v, got %v", tc.current, tc.desired, tc.want, got)
			}
		})
	}
}

func TestPolicyParametersValidation(t *testing.T) {
	cases := map[string]struct {
		policy  *policyv1beta1.Policy
		wantErr bool
	}{
		"ValidAllowBucket": {
			policy: &policyv1beta1.Policy{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-policy",
				},
				Spec: policyv1beta1.PolicySpec{
					ForProvider: policyv1beta1.PolicyParameters{
						AllowBucket: "test-bucket",
						PolicyName:  "TestPolicy",
						Description: "Test policy for bucket access",
					},
				},
			},
			wantErr: false,
		},
		"ValidRawPolicy": {
			policy: &policyv1beta1.Policy{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-policy",
				},
				Spec: policyv1beta1.PolicySpec{
					ForProvider: policyv1beta1.PolicyParameters{
						RawPolicy: `{
							"Version": "2012-10-17",
							"Statement": [
								{
									"Effect": "Allow",
									"Principal": "*",
									"Action": "s3:GetObject",
									"Resource": "arn:aws:s3:::test-bucket/*"
								}
							]
						}`,
						PolicyName:  "CustomPolicy",
						Description: "Custom S3 policy",
					},
				},
			},
			wantErr: false,
		},
		"EmptyPolicy": {
			policy: &policyv1beta1.Policy{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-policy",
				},
				Spec: policyv1beta1.PolicySpec{
					ForProvider: policyv1beta1.PolicyParameters{},
				},
			},
			wantErr: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			// Basic validation - check that at least one policy mode is specified
			hasAllowBucket := tc.policy.Spec.ForProvider.AllowBucket != ""
			hasRawPolicy := tc.policy.Spec.ForProvider.RawPolicy != ""

			if !hasAllowBucket && !hasRawPolicy {
				if !tc.wantErr {
					t.Error("Either AllowBucket or RawPolicy should be specified")
				}
			}

			if hasAllowBucket && hasRawPolicy {
				t.Error("Only one of AllowBucket or RawPolicy should be specified")
			}

			t.Logf("Test case '%s': allowBucket=%t, rawPolicy=%t", 
				name, hasAllowBucket, hasRawPolicy)
		})
	}
}

func TestGetPolicyName(t *testing.T) {
	cases := map[string]struct {
		policy *policyv1beta1.Policy
		want   string
	}{
		"ExplicitPolicyName": {
			policy: &policyv1beta1.Policy{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-policy-resource",
				},
				Spec: policyv1beta1.PolicySpec{
					ForProvider: policyv1beta1.PolicyParameters{
						PolicyName: "MyCustomPolicy",
					},
				},
			},
			want: "MyCustomPolicy",
		},
		"DefaultToResourceName": {
			policy: &policyv1beta1.Policy{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-policy-resource",
				},
				Spec: policyv1beta1.PolicySpec{
					ForProvider: policyv1beta1.PolicyParameters{
						// No explicit PolicyName
					},
				},
			},
			want: "my-policy-resource",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := &external{}
			got := e.getPolicyName(tc.policy)
			if got != tc.want {
				t.Errorf("getPolicyName(): want %v, got %v", tc.want, got)
			}
		})
	}
}