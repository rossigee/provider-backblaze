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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
)

// A ProviderConfigSpec defines the desired state of a ProviderConfig.
type ProviderConfigSpec struct {
	// Credentials required to authenticate to this provider.
	Credentials ProviderCredentials `json:"credentials"`
	// +kubebuilder:validation:Required
	// BackblazeRegion is the region where Backblaze B2 resources should be created.
	// Common values: us-west-001, us-west-002, eu-central-003
	BackblazeRegion string `json:"backblazeRegion"`
	// EndpointURL is the custom S3-compatible endpoint URL for Backblaze B2.
	// If not specified, defaults to the region-specific Backblaze B2 endpoint.
	// Format: https://s3.{region}.backblazeb2.com
	EndpointURL string `json:"endpointURL,omitempty"`
}

// ProviderCredentials required to authenticate.
type ProviderCredentials struct {
	//+kubebuilder:validation:Enum=None;Secret;InjectedIdentity;Environment;Filesystem

	// Source represents location of the credentials.
	Source xpv1.CredentialsSource `json:"source,omitempty"`

	// APISecretRef is the reference to the secret with the Backblaze B2 Application Key ID and Application Key.
	// The secret should contain keys:
	// - applicationKeyId: The Backblaze B2 Application Key ID (acts as access key)
	// - applicationKey: The Backblaze B2 Application Key (acts as secret key)
	APISecretRef corev1.SecretReference `json:"apiSecretRef,omitempty"`

	xpv1.CommonCredentialSelectors `json:",inline"`
}

// A ProviderConfigStatus reflects the observed state of a ProviderConfig.
type ProviderConfigStatus struct {
	xpv1.ProviderConfigStatus `json:",inline"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="Secret-Name",type="string",JSONPath=".spec.credentials.secretRef.name",priority=1
// +kubebuilder:printcolumn:name="Region",type="string",JSONPath=".spec.backblazeRegion"
// +kubebuilder:resource:scope=Cluster

// A ProviderConfig configures a Backblaze provider.
type ProviderConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProviderConfigSpec   `json:"spec"`
	Status ProviderConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ProviderConfigList contains a list of ProviderConfig.
type ProviderConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProviderConfig `json:"items"`
}

// +kubebuilder:object:root=true

// A ProviderConfigUsage indicates that a resource is using a ProviderConfig.
type ProviderConfigUsage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	xpv1.ProviderConfigUsage `json:",inline"`
}

// +kubebuilder:object:root=true

// ProviderConfigUsageList contains a list of ProviderConfigUsage
type ProviderConfigUsageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProviderConfigUsage `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ProviderConfig{}, &ProviderConfigList{})
	SchemeBuilder.Register(&ProviderConfigUsage{}, &ProviderConfigUsageList{})
}
