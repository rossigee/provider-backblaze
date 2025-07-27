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
	"reflect"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Synced",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="External Name",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,backblaze}

type Policy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PolicySpec   `json:"spec"`
	Status PolicyStatus `json:"status,omitempty"`
}

type PolicySpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       PolicyParameters `json:"forProvider,omitempty"`
}

type PolicyStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          PolicyProviderStatus `json:"atProvider,omitempty"`
}

type PolicyParameters struct {
	// AllowBucket will create a simple policy that allows all operations for the given bucket.
	// Mutually exclusive to `RawPolicy`.
	AllowBucket string `json:"allowBucket,omitempty"`

	// RawPolicy describes a raw S3-compatible policy document in JSON format.
	// This should follow the AWS S3 IAM policy format, which is compatible with Backblaze B2.
	// Mutually exclusive to `AllowBucket`.
	// For more details, see: https://www.backblaze.com/b2/docs/s3_compatible_api.html
	RawPolicy string `json:"rawPolicy,omitempty"`

	// PolicyName is the name for this policy. If not specified, uses the resource name.
	PolicyName string `json:"policyName,omitempty"`

	// Description provides a human-readable description of what this policy does.
	Description string `json:"description,omitempty"`
}

type PolicyProviderStatus struct {
	// Policy contains the rendered policy in JSON format as it's applied in Backblaze B2.
	Policy string `json:"policy,omitempty"`

	// PolicyName shows the actual name of the policy in Backblaze B2.
	PolicyName string `json:"policyName,omitempty"`
}

// +kubebuilder:object:root=true

type PolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Policy `json:"items"`
}

// Dummy type metadata.
var (
	PolicyKind             = reflect.TypeOf(Policy{}).Name()
	PolicyGroupKind        = schema.GroupKind{Group: Group, Kind: PolicyKind}.String()
	PolicyKindAPIVersion   = PolicyKind + "." + SchemeGroupVersion.String()
	PolicyGroupVersionKind = SchemeGroupVersion.WithKind(PolicyKind)
)

func init() {
	SchemeBuilder.Register(&Policy{}, &PolicyList{})
}
