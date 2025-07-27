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

func TestUserTypeDefinition(t *testing.T) {
	user := &User{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "backblaze.crossplane.io/v1",
			Kind:       "User",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-user",
		},
		Spec: UserSpec{
			ForProvider: UserParameters{
				KeyName: "test-api-key",
				Capabilities: []string{
					"listBuckets",
					"listFiles",
					"readFiles",
					"shareFiles",
					"writeFiles",
					"deleteFiles",
				},
			},
		},
	}

	if user.Name != "test-user" {
		t.Errorf("Expected name test-user, got %v", user.Name)
	}

	if user.Spec.ForProvider.KeyName != "test-api-key" {
		t.Errorf("Expected key name test-api-key, got %v", user.Spec.ForProvider.KeyName)
	}

	if len(user.Spec.ForProvider.Capabilities) != 6 {
		t.Errorf("Expected 6 capabilities, got %d", len(user.Spec.ForProvider.Capabilities))
	}
}

func TestUserCapabilities(t *testing.T) {
	validCapabilities := []string{
		"listKeys",
		"writeKeys",
		"deleteKeys",
		"listBuckets",
		"listFiles",
		"readFiles",
		"shareFiles",
		"writeFiles",
		"deleteFiles",
		"readBucketRetentions",
		"writeBucketRetentions",
		"readBucketEncryption",
		"writeBucketEncryption",
		"listAllBucketNames",
	}

	// Test that all standard capabilities can be assigned
	for _, capability := range validCapabilities {
		user := UserParameters{
			KeyName:      "test-key",
			Capabilities: []string{capability},
		}

		if len(user.Capabilities) != 1 {
			t.Errorf("Expected 1 capability, got %d", len(user.Capabilities))
		}

		if user.Capabilities[0] != capability {
			t.Errorf("Expected capability %s, got %s", capability, user.Capabilities[0])
		}
	}
}

func TestUserList(t *testing.T) {
	userList := UserList{
		Items: []User{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "user-1",
				},
				Spec: UserSpec{
					ForProvider: UserParameters{
						KeyName:      "key-1",
						Capabilities: []string{"listBuckets", "readFiles"},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "user-2",
				},
				Spec: UserSpec{
					ForProvider: UserParameters{
						KeyName:      "key-2",
						Capabilities: []string{"writeFiles", "deleteFiles"},
					},
				},
			},
		},
	}

	if len(userList.Items) != 2 {
		t.Errorf("Expected 2 users, got %d", len(userList.Items))
	}

	// Verify each user has the expected structure
	for i, user := range userList.Items {
		if user.Spec.ForProvider.KeyName == "" {
			t.Errorf("User %d has empty key name", i)
		}
		if len(user.Spec.ForProvider.Capabilities) == 0 {
			t.Errorf("User %d has no capabilities", i)
		}
	}
}

func TestUserGroupVersionKind(t *testing.T) {
	// Test that the group version kind constants are properly set
	expectedKind := "User"
	if UserKind != expectedKind {
		t.Errorf("UserKind = %v, want %v", UserKind, expectedKind)
	}

	if UserGroupVersionKind.Kind != expectedKind {
		t.Errorf("UserGroupVersionKind.Kind = %v, want %v", UserGroupVersionKind.Kind, expectedKind)
	}

	if UserGroupVersionKind.Group != Group {
		t.Errorf("UserGroupVersionKind.Group = %v, want %v", UserGroupVersionKind.Group, Group)
	}
}
