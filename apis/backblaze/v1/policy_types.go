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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
)

// PolicyParameters are the configurable fields of a Policy.
type PolicyParameters struct {
	// PolicyName is the name for this policy.
	// +optional
	PolicyName *string `json:"policyName,omitempty"`
	// Description provides a human-readable description of the policy.
	// +optional
	Description *string `json:"description,omitempty"`
	// AllowBucket creates a simple policy that allows all operations for the specified bucket.
	// This is mutually exclusive with RawPolicy.
	// +optional
	AllowBucket *string `json:"allowBucket,omitempty"`
	// RawPolicy contains the complete S3-compatible policy document as JSON.
	// This is mutually exclusive with AllowBucket.
	// +optional
	RawPolicy *string `json:"rawPolicy,omitempty"`
}

// PolicyObservation are the observable fields of a Policy.
type PolicyObservation struct {
	// PolicyName is the name of the policy.
	PolicyName string `json:"policyName,omitempty"`
	// PolicyDocument is the actual policy document stored.
	PolicyDocument string `json:"policyDocument,omitempty"`
	// PolicyID is the unique identifier for the policy (if applicable).
	PolicyID string `json:"policyId,omitempty"`
	// CreationTime is when the policy was created.
	CreationTime *metav1.Time `json:"creationTime,omitempty"`
}

// A PolicySpec defines the desired state of a Policy.
type PolicySpec struct {
	DeletionPolicy                   xpv1.DeletionPolicy     `json:"deletionPolicy,omitempty"`
	ManagementPolicies               xpv1.ManagementPolicies `json:"managementPolicies,omitempty"`
	ProviderConfigReference          *xpv1.Reference         `json:"providerConfigReference,omitempty"`
	WriteConnectionSecretToReference *xpv1.SecretReference   `json:"writeConnectionSecretToRef,omitempty"`
	ForProvider                      PolicyParameters        `json:"forProvider"`
}

// A PolicyStatus represents the observed state of a Policy.
type PolicyStatus struct {
	Conditions []xpv1.Condition  `json:"conditions,omitempty"`
	AtProvider PolicyObservation `json:"atProvider,omitempty"`
}

// GetCondition returns the status condition by type.
func (s *PolicyStatus) GetCondition(ct xpv1.ConditionType) xpv1.Condition {
	for _, c := range s.Conditions {
		if c.Type == ct {
			return c
		}
	}
	return xpv1.Condition{Type: ct, Status: corev1.ConditionUnknown}
}

// SetConditions sets the status conditions.
func (s *PolicyStatus) SetConditions(c ...xpv1.Condition) {
	s.Conditions = c
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,backblaze}
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL NAME",type="string",JSONPath=".metadata.annotations.crossplane.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="POLICY NAME",type="string",JSONPath=".status.atProvider.policyName"

// A Policy represents a Backblaze B2 S3-compatible policy.
type Policy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:",inline"`
	Spec              PolicySpec   `json:"spec"`
	Status            PolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// PolicyList contains a list of Policy
type PolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:",inline"`
	Items           []Policy `json:"items"`
}

// GetCondition returns the status condition by type.
func (p *Policy) GetCondition(ct xpv1.ConditionType) xpv1.Condition {
	return p.Status.GetCondition(ct)
}

// SetConditions sets the status conditions.
func (p *Policy) SetConditions(c ...xpv1.Condition) {
	p.Status.SetConditions(c...)
}

// GetDeletionPolicy returns the deletion policy.
func (p *Policy) GetDeletionPolicy() xpv1.DeletionPolicy {
	return p.Spec.DeletionPolicy
}

// SetDeletionPolicy sets the deletion policy.
func (p *Policy) SetDeletionPolicy(dp xpv1.DeletionPolicy) {
	p.Spec.DeletionPolicy = dp
}

// GetManagementPolicies returns the management policies.
func (p *Policy) GetManagementPolicies() xpv1.ManagementPolicies {
	return p.Spec.ManagementPolicies
}

// SetManagementPolicies sets the management policies.
func (p *Policy) SetManagementPolicies(mp xpv1.ManagementPolicies) {
	p.Spec.ManagementPolicies = mp
}

// GetProviderConfigReference returns the provider config reference.
func (p *Policy) GetProviderConfigReference() *xpv1.Reference {
	return p.Spec.ProviderConfigReference
}

// SetProviderConfigReference sets the provider config reference.
func (p *Policy) SetProviderConfigReference(r *xpv1.Reference) {
	p.Spec.ProviderConfigReference = r
}

// GetWriteConnectionSecretToReference returns the write connection secret to reference.
func (p *Policy) GetWriteConnectionSecretToReference() *xpv1.SecretReference {
	return p.Spec.WriteConnectionSecretToReference
}

// SetWriteConnectionSecretToReference sets the write connection secret to reference.
func (p *Policy) SetWriteConnectionSecretToReference(r *xpv1.SecretReference) {
	p.Spec.WriteConnectionSecretToReference = r
}

// Policy type metadata.
var (
	PolicyKind             = reflect.TypeOf(Policy{}).Name()
	PolicyGroupKind        = schema.GroupKind{Group: Group, Kind: PolicyKind}
	PolicyKindAPIVersion   = PolicyKind + "." + SchemeGroupVersion.String()
	PolicyGroupVersionKind = SchemeGroupVersion.WithKind(PolicyKind)
)

// GetPolicyName returns the policy name from the Policy resource.
func (mg *Policy) GetPolicyName() string {
	if mg.Spec.ForProvider.PolicyName != nil {
		return *mg.Spec.ForProvider.PolicyName
	}
	return mg.GetName()
}
