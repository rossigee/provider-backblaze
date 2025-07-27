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
// +kubebuilder:printcolumn:name="Key Name",type="string",JSONPath=".spec.forProvider.keyName"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,backblaze}

type User struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   UserSpec   `json:"spec"`
	Status UserStatus `json:"status,omitempty"`
}

type UserSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       UserParameters `json:"forProvider,omitempty"`
}

type UserStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          UserProviderStatus `json:"atProvider,omitempty"`
}

type UserParameters struct {
	// KeyName is the name for the application key.
	// This is a human-readable name to help identify the key.
	// +kubebuilder:validation:Required
	KeyName string `json:"keyName"`

	// Capabilities defines what operations this application key can perform.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	// Valid capabilities include: listKeys, writeKeys, deleteKeys, listBuckets, writeBuckets,
	// deleteFile, listFiles, readFiles, shareFiles, writeFiles, deleteFile
	Capabilities []string `json:"capabilities"`

	// BucketID restricts this key to operations on a specific bucket.
	// If not specified, the key will have access based on capabilities across all accessible buckets.
	BucketID string `json:"bucketID,omitempty"`

	// NamePrefix limits file operations to files whose names start with this prefix.
	// Only applies when BucketID is also specified.
	NamePrefix string `json:"namePrefix,omitempty"`

	// ValidDurationInSeconds specifies how long the key should remain valid.
	// If not specified, the key will remain valid until explicitly deleted.
	// Maximum value is 1000 days (86400000 seconds).
	ValidDurationInSeconds *int `json:"validDurationInSeconds,omitempty"`

	// WriteSecretToRef specifies where to store the generated application key.
	// +kubebuilder:validation:Required
	WriteSecretToRef xpv1.SecretReference `json:"writeSecretToRef"`
}

type UserProviderStatus struct {
	// ApplicationKeyID is the unique identifier for the application key.
	ApplicationKeyID string `json:"applicationKeyID,omitempty"`

	// KeyName is the name of the application key.
	KeyName string `json:"keyName,omitempty"`

	// Capabilities lists the operations this key can perform.
	Capabilities []string `json:"capabilities,omitempty"`

	// BucketID shows which bucket this key is restricted to (if any).
	BucketID string `json:"bucketID,omitempty"`

	// NamePrefix shows the file name prefix restriction (if any).
	NamePrefix string `json:"namePrefix,omitempty"`

	// ExpirationTimestamp shows when the key will expire (if applicable).
	ExpirationTimestamp *metav1.Time `json:"expirationTimestamp,omitempty"`
}

// +kubebuilder:object:root=true

type UserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []User `json:"items"`
}

// Dummy type metadata.
var (
	UserKind             = reflect.TypeOf(User{}).Name()
	UserGroupKind        = schema.GroupKind{Group: Group, Kind: UserKind}.String()
	UserKindAPIVersion   = UserKind + "." + SchemeGroupVersion.String()
	UserGroupVersionKind = SchemeGroupVersion.WithKind(UserKind)
)

func init() {
	SchemeBuilder.Register(&User{}, &UserList{})
}
