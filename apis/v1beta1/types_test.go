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

package v1beta1

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
)

func TestProviderConfigSpec(t *testing.T) {
	tests := []struct {
		name string
		spec ProviderConfigSpec
	}{
		{
			name: "valid config with default endpoint",
			spec: ProviderConfigSpec{
				BackblazeRegion: "us-west-001",
				Credentials: ProviderCredentials{
					Source: xpv1.CredentialsSourceSecret,
					APISecretRef: corev1.SecretReference{
						Name:      "backblaze-secret",
						Namespace: "crossplane-system",
					},
				},
			},
		},
		{
			name: "valid config with custom endpoint",
			spec: ProviderConfigSpec{
				BackblazeRegion: "eu-central-003",
				EndpointURL:     "https://custom.s3.endpoint.com",
				Credentials: ProviderCredentials{
					Source: xpv1.CredentialsSourceSecret,
					APISecretRef: corev1.SecretReference{
						Name:      "backblaze-secret",
						Namespace: "crossplane-system",
					},
				},
			},
		},
		{
			name: "environment credentials",
			spec: ProviderConfigSpec{
				BackblazeRegion: "us-west-002",
				Credentials: ProviderCredentials{
					Source: xpv1.CredentialsSourceEnvironment,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test that the spec can be created without error
			if tt.spec.BackblazeRegion == "" {
				t.Error("BackblazeRegion should not be empty")
			}

			// Test credentials source validation
			validSources := []xpv1.CredentialsSource{
				xpv1.CredentialsSourceNone,
				xpv1.CredentialsSourceSecret,
				xpv1.CredentialsSourceInjectedIdentity,
				xpv1.CredentialsSourceEnvironment,
				xpv1.CredentialsSourceFilesystem,
			}

			found := false
			for _, valid := range validSources {
				if tt.spec.Credentials.Source == valid {
					found = true
					break
				}
			}

			if !found {
				t.Errorf("Invalid credentials source: %v", tt.spec.Credentials.Source)
			}
		})
	}
}

func TestProviderConfig(t *testing.T) {
	pc := &ProviderConfig{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "backblaze.crossplane.io/v1beta1",
			Kind:       "ProviderConfig",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-provider-config",
		},
		Spec: ProviderConfigSpec{
			BackblazeRegion: "us-west-001",
			Credentials: ProviderCredentials{
				Source: xpv1.CredentialsSourceSecret,
				APISecretRef: corev1.SecretReference{
					Name:      "backblaze-secret",
					Namespace: "crossplane-system",
				},
			},
		},
	}

	if pc.Name != "test-provider-config" {
		t.Errorf("Expected name test-provider-config, got %v", pc.Name)
	}

	if pc.Spec.BackblazeRegion != "us-west-001" {
		t.Errorf("Expected region us-west-001, got %v", pc.Spec.BackblazeRegion)
	}

	if pc.Spec.Credentials.Source != xpv1.CredentialsSourceSecret {
		t.Errorf("Expected credentials source Secret, got %v", pc.Spec.Credentials.Source)
	}
}

func TestProviderConfigUsage(t *testing.T) {
	usage := &ProviderConfigUsage{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "backblaze.crossplane.io/v1beta1",
			Kind:       "ProviderConfigUsage",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-usage",
		},
		ProviderConfigUsage: xpv1.ProviderConfigUsage{
			ProviderConfigReference: xpv1.Reference{
				Name: "test-provider-config",
			},
			ResourceReference: xpv1.TypedReference{
				APIVersion: "backblaze.crossplane.io/v1",
				Kind:       "Bucket",
				Name:       "test-bucket",
			},
		},
	}

	if usage.Name != "test-usage" {
		t.Errorf("Expected name test-usage, got %v", usage.Name)
	}

	if usage.ProviderConfigReference.Name != "test-provider-config" {
		t.Errorf("Expected provider config ref test-provider-config, got %v",
			usage.ProviderConfigReference.Name)
	}

	if usage.ResourceReference.Kind != "Bucket" {
		t.Errorf("Expected resource kind Bucket, got %v", usage.ResourceReference.Kind)
	}
}

func TestProviderCredentialsValidation(t *testing.T) {
	tests := []struct {
		name        string
		credentials ProviderCredentials
		expectValid bool
	}{
		{
			name: "secret credentials with reference",
			credentials: ProviderCredentials{
				Source: xpv1.CredentialsSourceSecret,
				APISecretRef: corev1.SecretReference{
					Name:      "backblaze-secret",
					Namespace: "crossplane-system",
				},
			},
			expectValid: true,
		},
		{
			name: "environment credentials",
			credentials: ProviderCredentials{
				Source: xpv1.CredentialsSourceEnvironment,
			},
			expectValid: true,
		},
		{
			name: "filesystem credentials",
			credentials: ProviderCredentials{
				Source: xpv1.CredentialsSourceFilesystem,
				CommonCredentialSelectors: xpv1.CommonCredentialSelectors{
					Fs: &xpv1.FsSelector{
						Path: "/etc/backblaze/credentials",
					},
				},
			},
			expectValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Basic validation that credentials can be created
			if tt.credentials.Source == "" && tt.expectValid {
				t.Error("Credentials source should not be empty for valid credentials")
			}

			// Test secret reference validation
			if tt.credentials.Source == xpv1.CredentialsSourceSecret {
				if tt.credentials.APISecretRef.Name == "" && tt.expectValid {
					t.Error("Secret name should not be empty when using secret source")
				}
			}
		})
	}
}
