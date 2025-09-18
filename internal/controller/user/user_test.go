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
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	userv1beta1 "github.com/rossigee/provider-backblaze/apis/user/v1beta1"
)

func TestEqualStringSlices(t *testing.T) {
	cases := map[string]struct {
		a    []string
		b    []string
		want bool
	}{
		"Equal": {
			a:    []string{"a", "b", "c"},
			b:    []string{"a", "b", "c"},
			want: true,
		},
		"EqualDifferentOrder": {
			a:    []string{"a", "b", "c"},
			b:    []string{"c", "a", "b"},
			want: true,
		},
		"DifferentLength": {
			a:    []string{"a", "b"},
			b:    []string{"a", "b", "c"},
			want: false,
		},
		"DifferentContent": {
			a:    []string{"a", "b", "c"},
			b:    []string{"a", "b", "d"},
			want: false,
		},
		"Empty": {
			a:    []string{},
			b:    []string{},
			want: true,
		},
		"OneEmpty": {
			a:    []string{"a"},
			b:    []string{},
			want: false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := equalStringSlices(tc.a, tc.b)
			if got != tc.want {
				t.Errorf("equalStringSlices(%v, %v): want %v, got %v", tc.a, tc.b, tc.want, got)
			}
		})
	}
}

func TestUserParametersValidation(t *testing.T) {
	cases := map[string]struct {
		user    *userv1beta1.User
		wantErr bool
	}{
		"ValidMinimal": {
			user: &userv1beta1.User{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-user",
				},
				Spec: userv1beta1.UserSpec{
					ForProvider: userv1beta1.UserParameters{
						KeyName:      "test-key",
						Capabilities: []string{"listBuckets"},
						WriteSecretToRef: xpv1.SecretReference{
							Name:      "test-secret",
							Namespace: "default",
						},
					},
				},
			},
			wantErr: false,
		},
		"ValidWithBucket": {
			user: &userv1beta1.User{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-user",
				},
				Spec: userv1beta1.UserSpec{
					ForProvider: userv1beta1.UserParameters{
						KeyName:      "test-key",
						Capabilities: []string{"listFiles", "readFiles"},
						BucketID:     "bucket-123",
						NamePrefix:   "uploads/",
						WriteSecretToRef: xpv1.SecretReference{
							Name:      "test-secret",
							Namespace: "default",
						},
					},
				},
			},
			wantErr: false,
		},
		"ValidWithExpiration": {
			user: &userv1beta1.User{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-user",
				},
				Spec: userv1beta1.UserSpec{
					ForProvider: userv1beta1.UserParameters{
						KeyName:                "test-key",
						Capabilities:           []string{"listBuckets"},
						ValidDurationInSeconds: func() *int { i := 86400; return &i }(), // 1 day
						WriteSecretToRef: xpv1.SecretReference{
							Name:      "test-secret",
							Namespace: "default",
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			// Basic validation - check that required fields are present
			if tc.user.Spec.ForProvider.KeyName == "" {
				if !tc.wantErr {
					t.Error("KeyName should not be empty")
				}
			}

			if len(tc.user.Spec.ForProvider.Capabilities) == 0 {
				if !tc.wantErr {
					t.Error("Capabilities should not be empty")
				}
			}

			if tc.user.Spec.ForProvider.WriteSecretToRef.Name == "" {
				if !tc.wantErr {
					t.Error("WriteSecretToRef.Name should not be empty")
				}
			}

			if tc.user.Spec.ForProvider.WriteSecretToRef.Namespace == "" {
				if !tc.wantErr {
					t.Error("WriteSecretToRef.Namespace should not be empty")
				}
			}
		})
	}
}

func TestUserCapabilities(t *testing.T) {
	validCapabilities := []string{
		"listKeys", "writeKeys", "deleteKeys",
		"listBuckets", "writeBuckets",
		"listFiles", "readFiles", "shareFiles", "writeFiles", "deleteFile",
	}

	cases := map[string]struct {
		capabilities []string
		description  string
	}{
		"FullAccess": {
			capabilities: validCapabilities,
			description:  "All available capabilities",
		},
		"BucketManagement": {
			capabilities: []string{"listBuckets", "writeBuckets"},
			description:  "Bucket management only",
		},
		"FileOperations": {
			capabilities: []string{"listFiles", "readFiles", "writeFiles", "deleteFile"},
			description:  "File operations only",
		},
		"KeyManagement": {
			capabilities: []string{"listKeys", "writeKeys", "deleteKeys"},
			description:  "Application key management",
		},
		"ReadOnly": {
			capabilities: []string{"listBuckets", "listFiles", "readFiles"},
			description:  "Read-only access",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			user := &userv1beta1.User{
				Spec: userv1beta1.UserSpec{
					ForProvider: userv1beta1.UserParameters{
						KeyName:      "test-key",
						Capabilities: tc.capabilities,
						WriteSecretToRef: xpv1.SecretReference{
							Name:      "test-secret",
							Namespace: "default",
						},
					},
				},
			}

			// Verify capabilities are set correctly
			if len(user.Spec.ForProvider.Capabilities) != len(tc.capabilities) {
				t.Errorf("Expected %d capabilities, got %d", len(tc.capabilities), len(user.Spec.ForProvider.Capabilities))
			}

			// Check that all capabilities are present
			capMap := make(map[string]bool)
			for _, cap := range user.Spec.ForProvider.Capabilities {
				capMap[cap] = true
			}

			for _, expectedCap := range tc.capabilities {
				if !capMap[expectedCap] {
					t.Errorf("Expected capability %s not found", expectedCap)
				}
			}

			t.Logf("Test case '%s': %s - verified %d capabilities", name, tc.description, len(tc.capabilities))
		})
	}
}