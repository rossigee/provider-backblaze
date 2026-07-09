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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"
)

// UserParameters are the configurable fields of a User (Application Key).
type UserParameters struct {
	// KeyName is the human-readable name for the application key.
	KeyName string `json:"keyName"`
	// Capabilities define what this application key can do.
	// Available capabilities:
	// - listKeys, writeKeys, deleteKeys: manage application keys
	// - listBuckets, writeBuckets: manage buckets
	// - listFiles, readFiles, shareFiles, writeFiles, deleteFile: manage files
	Capabilities []string `json:"capabilities"`
	// BucketID restricts the key to operations on this specific bucket only.
	// +optional
	BucketID *string `json:"bucketId,omitempty"`
	// NamePrefix restricts file operations to files whose names start with this prefix.
	// +optional
	NamePrefix *string `json:"namePrefix,omitempty"`
	// ValidDurationInSeconds sets how long the key will be valid (max 1000 days).
	// +optional
	ValidDurationInSeconds *int64 `json:"validDurationInSeconds,omitempty"`
	// WriteSecretToRef specifies the secret where the application key credentials will be stored.
	WriteSecretToRef xpv1.SecretReference `json:"writeSecretToRef"`
}

// UserObservation are the observable fields of a User.
type UserObservation struct {
	// ApplicationKeyID is the ID of the created application key.
	ApplicationKeyID string `json:"applicationKeyId,omitempty"`
	// AccountID is the account that owns this application key.
	AccountID string `json:"accountId,omitempty"`
	// Capabilities are the capabilities granted to this key.
	Capabilities []string `json:"capabilities,omitempty"`
	// BucketID is the bucket this key is restricted to (if any).
	BucketID *string `json:"bucketId,omitempty"`
	// NamePrefix is the prefix this key is restricted to (if any).
	NamePrefix *string `json:"namePrefix,omitempty"`
	// ExpirationTimestamp is when this key will expire (if set).
	ExpirationTimestamp *int64 `json:"expirationTimestamp,omitempty"`
}

// A UserSpec defines the desired state of a User.
type UserSpec struct {
	DeletionPolicy                   xpv1.DeletionPolicy     `json:"deletionPolicy,omitempty"`
	ManagementPolicies               xpv1.ManagementPolicies `json:"managementPolicies,omitempty"`
	ProviderConfigReference          *xpv1.Reference         `json:"providerConfigReference,omitempty"`
	WriteConnectionSecretToReference *xpv1.SecretReference   `json:"writeConnectionSecretToRef,omitempty"`
	ForProvider                      UserParameters          `json:"forProvider"`
}

// A UserStatus represents the observed state of a User.
type UserStatus struct {
	Conditions []xpv1.Condition `json:"conditions,omitempty"`
	AtProvider UserObservation  `json:"atProvider,omitempty"`
}

// GetCondition returns the status condition by type.
func (s *UserStatus) GetCondition(ct xpv1.ConditionType) xpv1.Condition {
	for _, c := range s.Conditions {
		if c.Type == ct {
			return c
		}
	}
	return xpv1.Condition{Type: ct, Status: corev1.ConditionUnknown}
}

// SetConditions sets the status conditions.
func (s *UserStatus) SetConditions(c ...xpv1.Condition) {
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
// +kubebuilder:printcolumn:name="KEY NAME",type="string",JSONPath=".spec.forProvider.keyName"
// +kubebuilder:printcolumn:name="KEY ID",type="string",JSONPath=".status.atProvider.applicationKeyId"

// A User represents a Backblaze B2 application key.
type User struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:",inline"`
	Spec              UserSpec   `json:"spec"`
	Status            UserStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// UserList contains a list of User
type UserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:",inline"`
	Items           []User `json:"items"`
}

// GetCondition returns the status condition by type.
func (u *User) GetCondition(ct xpv1.ConditionType) xpv1.Condition {
	return u.Status.GetCondition(ct)
}

// SetConditions sets the status conditions.
func (u *User) SetConditions(c ...xpv1.Condition) {
	u.Status.SetConditions(c...)
}

// GetDeletionPolicy returns the deletion policy.
func (u *User) GetDeletionPolicy() xpv1.DeletionPolicy {
	return u.Spec.DeletionPolicy
}

// SetDeletionPolicy sets the deletion policy.
func (u *User) SetDeletionPolicy(dp xpv1.DeletionPolicy) {
	u.Spec.DeletionPolicy = dp
}

// GetManagementPolicies returns the management policies.
func (u *User) GetManagementPolicies() xpv1.ManagementPolicies {
	return u.Spec.ManagementPolicies
}

// SetManagementPolicies sets the management policies.
func (u *User) SetManagementPolicies(mp xpv1.ManagementPolicies) {
	u.Spec.ManagementPolicies = mp
}

// GetProviderConfigReference returns the provider config reference.
func (u *User) GetProviderConfigReference() *xpv1.Reference {
	return u.Spec.ProviderConfigReference
}

// SetProviderConfigReference sets the provider config reference.
func (u *User) SetProviderConfigReference(r *xpv1.Reference) {
	u.Spec.ProviderConfigReference = r
}

// GetWriteConnectionSecretToReference returns the write connection secret to reference.
func (u *User) GetWriteConnectionSecretToReference() *xpv1.SecretReference {
	return u.Spec.WriteConnectionSecretToReference
}

// SetWriteConnectionSecretToReference sets the write connection secret to reference.
func (u *User) SetWriteConnectionSecretToReference(r *xpv1.SecretReference) {
	u.Spec.WriteConnectionSecretToReference = r
}

// User type metadata lives in register.go. Avoid duplicating here.

// GetKeyName returns the key name from the User resource.
func (mg *User) GetKeyName() string {
	return mg.Spec.ForProvider.KeyName
}
