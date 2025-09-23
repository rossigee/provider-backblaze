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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
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
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       UserParameters `json:"forProvider"`
}

// A UserStatus represents the observed state of a User.
type UserStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          UserObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,backblaze}
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="KEY NAME",type="string",JSONPath=".spec.forProvider.keyName"
// +kubebuilder:printcolumn:name="KEY ID",type="string",JSONPath=".status.atProvider.applicationKeyId"

// A User represents a Backblaze B2 application key.
type User struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:",inline"`

	Spec   UserSpec   `json:"spec"`
	Status UserStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// UserList contains a list of User
type UserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:",inline"`
	Items           []User `json:"items"`
}

// User type metadata.
var (
	UserKind             = reflect.TypeOf(User{}).Name()
	UserGroupKind        = schema.GroupKind{Group: Group, Kind: UserKind}
	UserKindAPIVersion   = UserKind + "." + SchemeGroupVersion.String()
	UserGroupVersionKind = SchemeGroupVersion.WithKind(UserKind)
)

func init() {
	SchemeBuilder.Register(&User{}, &UserList{})
}

// GetKeyName returns the key name from the User resource.
func (mg *User) GetKeyName() string {
	return mg.Spec.ForProvider.KeyName
}