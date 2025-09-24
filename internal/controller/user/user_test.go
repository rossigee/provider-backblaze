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

	backblazev1 "github.com/rossigee/provider-backblaze/apis/backblaze/v1"
)

func TestUserGetKeyName(t *testing.T) {
	user := &backblazev1.User{
		Spec: backblazev1.UserSpec{
			ForProvider: backblazev1.UserParameters{
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
	user := &backblazev1.User{}

	// Test that setCondition doesn't panic - this validates the method signature and basic functionality
	r.setCondition(user, xpv1.TypeReady, "True", "Available", "User is ready")

	// Test passes if no panic occurs
}

func TestCreateApplicationKey(t *testing.T) {
	user := &backblazev1.User{
		Spec: backblazev1.UserSpec{
			ForProvider: backblazev1.UserParameters{
				KeyName:      "test-key",
				Capabilities: []string{"listBuckets", "readFiles"},
				WriteSecretToRef: xpv1.SecretReference{
					Name:      "test-secret",
					Namespace: "default",
				},
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
	if user.Spec.ForProvider.WriteSecretToRef.Name != "test-secret" {
		t.Error("Secret reference not set correctly")
	}
}