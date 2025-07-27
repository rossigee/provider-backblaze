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

package features

import (
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/feature"
)

func TestFeatureFlags(t *testing.T) {
	tests := []struct {
		name string
		flag feature.Flag
		want string
	}{
		{
			name: "EnableAlphaExternalSecretStores",
			flag: EnableAlphaExternalSecretStores,
			want: "EnableAlphaExternalSecretStores",
		},
		{
			name: "EnableAlphaManagementPolicies",
			flag: EnableAlphaManagementPolicies,
			want: "EnableAlphaManagementPolicies",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.flag) != tt.want {
				t.Errorf("Feature flag %s = %v, want %v", tt.name, tt.flag, tt.want)
			}
		})
	}
}

func TestFeatureFlagsWithRuntime(t *testing.T) {
	flags := &feature.Flags{}

	// Test enabling features
	flags.Enable(EnableAlphaExternalSecretStores)
	flags.Enable(EnableAlphaManagementPolicies)

	// Test that flags can be enabled without error
	if !flags.Enabled(EnableAlphaExternalSecretStores) {
		t.Error("EnableAlphaExternalSecretStores should be enabled")
	}

	if !flags.Enabled(EnableAlphaManagementPolicies) {
		t.Error("EnableAlphaManagementPolicies should be enabled")
	}
}
