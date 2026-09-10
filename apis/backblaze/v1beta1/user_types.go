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

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,backblaze}
// +genclient
// +genclient:namespaced
// +groupName=backblaze.m.crossplane.io

// A User represents a Backblaze B2 application key.
type User struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              UserSpec   `json:"spec"`
	Status            UserStatus `json:"status,omitempty"`
}

type UserSpec struct {
	xpv1.ManagedResourceSpec `json:",inline"`
	ForProvider              UserParameters `json:"forProvider"`
}

// UserStatus represents the observed state of a User.
type UserStatus struct {
	xpv1.ConditionedStatus `json:",inline"`
	AtProvider             UserObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// UserList contains a list of User.
type UserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []User `json:"items"`
}
