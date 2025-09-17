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

package integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"

	// v1 APIs (cluster-scoped)
	bucketv1 "github.com/rossigee/provider-backblaze/apis/bucket/v1"
	userv1 "github.com/rossigee/provider-backblaze/apis/user/v1"
	policyv1 "github.com/rossigee/provider-backblaze/apis/policy/v1"

	// v1beta1 APIs (namespaced)
	bucketv1beta1 "github.com/rossigee/provider-backblaze/apis/bucket/v1beta1"
	userv1beta1 "github.com/rossigee/provider-backblaze/apis/user/v1beta1"
	policyv1beta1 "github.com/rossigee/provider-backblaze/apis/policy/v1beta1"

	apisv1beta1 "github.com/rossigee/provider-backblaze/apis/v1beta1"
)

// TestV2APICompatibility validates that both v1 and v1beta1 APIs are properly registered
func TestV2APICompatibility(t *testing.T) {
	scheme := runtime.NewScheme()

	// Register all API versions
	require.NoError(t, bucketv1.SchemeBuilder.AddToScheme(scheme))
	require.NoError(t, userv1.SchemeBuilder.AddToScheme(scheme))
	require.NoError(t, policyv1.SchemeBuilder.AddToScheme(scheme))
	require.NoError(t, bucketv1beta1.SchemeBuilder.AddToScheme(scheme))
	require.NoError(t, userv1beta1.SchemeBuilder.AddToScheme(scheme))
	require.NoError(t, policyv1beta1.SchemeBuilder.AddToScheme(scheme))
	require.NoError(t, apisv1beta1.SchemeBuilder.AddToScheme(scheme))

	// Test that all types can be resolved
	gvks := scheme.AllKnownTypes()

	// v1 APIs (cluster-scoped)
	assert.Contains(t, gvks, bucketv1.BucketGroupVersionKind, "v1 Bucket GVK should be registered")
	assert.Contains(t, gvks, userv1.UserGroupVersionKind, "v1 User GVK should be registered")
	assert.Contains(t, gvks, policyv1.PolicyGroupVersionKind, "v1 Policy GVK should be registered")

	// v1beta1 APIs (namespaced)
	assert.Contains(t, gvks, bucketv1beta1.BucketGroupVersionKind, "v1beta1 Bucket GVK should be registered")
	assert.Contains(t, gvks, userv1beta1.UserGroupVersionKind, "v1beta1 User GVK should be registered")
	assert.Contains(t, gvks, policyv1beta1.PolicyGroupVersionKind, "v1beta1 Policy GVK should be registered")
}

// TestV1BucketClusterScoped validates v1 cluster-scoped bucket functionality
func TestV1BucketClusterScoped(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, bucketv1.SchemeBuilder.AddToScheme(scheme))
	require.NoError(t, apisv1beta1.SchemeBuilder.AddToScheme(scheme))

	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	bucket := &bucketv1.Bucket{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-cluster-bucket",
		},
		Spec: bucketv1.BucketSpec{
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{Name: "default"},
			},
			ForProvider: bucketv1.BucketParameters{
				BucketName: "test-cluster-bucket-b2",
				Region:     "us-west-001",
				BucketType: "allPrivate",
			},
		},
	}

	ctx := context.Background()

	// Create bucket
	err := client.Create(ctx, bucket)
	require.NoError(t, err, "Should create v1 cluster-scoped bucket")

	// Retrieve bucket
	retrieved := &bucketv1.Bucket{}
	err = client.Get(ctx, types.NamespacedName{Name: "test-cluster-bucket"}, retrieved)
	require.NoError(t, err, "Should retrieve v1 cluster-scoped bucket")

	// Validate properties
	assert.Equal(t, "test-cluster-bucket-b2", retrieved.Spec.ForProvider.BucketName)
	assert.Equal(t, "us-west-001", retrieved.Spec.ForProvider.Region)
	assert.Equal(t, "allPrivate", retrieved.Spec.ForProvider.BucketType)
	assert.Equal(t, "test-cluster-bucket-b2", retrieved.GetBucketName())

	// Validate managed resource interface
	assert.NotNil(t, retrieved.GetProviderConfigReference())
	assert.Equal(t, "default", retrieved.GetProviderConfigReference().Name)
}

// TestV1Beta1BucketNamespaced validates v1beta1 namespaced bucket functionality
func TestV1Beta1BucketNamespaced(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, bucketv1beta1.SchemeBuilder.AddToScheme(scheme))
	require.NoError(t, apisv1beta1.SchemeBuilder.AddToScheme(scheme))

	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	bucket := &bucketv1beta1.Bucket{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-namespaced-bucket",
			Namespace: "test-namespace",
		},
		Spec: bucketv1beta1.BucketSpec{
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{Name: "default"},
			},
			ForProvider: bucketv1beta1.BucketParameters{
				BucketName: "test-namespaced-bucket-b2",
				Region:     "us-west-001",
				BucketType: "allPrivate",
			},
		},
	}

	ctx := context.Background()

	// Create bucket
	err := client.Create(ctx, bucket)
	require.NoError(t, err, "Should create v1beta1 namespaced bucket")

	// Retrieve bucket
	retrieved := &bucketv1beta1.Bucket{}
	err = client.Get(ctx, types.NamespacedName{Name: "test-namespaced-bucket", Namespace: "test-namespace"}, retrieved)
	require.NoError(t, err, "Should retrieve v1beta1 namespaced bucket")

	// Validate properties
	assert.Equal(t, "test-namespaced-bucket-b2", retrieved.Spec.ForProvider.BucketName)
	assert.Equal(t, "us-west-001", retrieved.Spec.ForProvider.Region)
	assert.Equal(t, "allPrivate", retrieved.Spec.ForProvider.BucketType)
	assert.Equal(t, "test-namespace", retrieved.Namespace)
	assert.Equal(t, "test-namespaced-bucket-b2", retrieved.GetBucketName())

	// Validate managed resource interface
	assert.NotNil(t, retrieved.GetProviderConfigReference())
	assert.Equal(t, "default", retrieved.GetProviderConfigReference().Name)
}

// TestV1Beta1UserNamespaced validates v1beta1 namespaced user functionality
func TestV1Beta1UserNamespaced(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, userv1beta1.SchemeBuilder.AddToScheme(scheme))
	require.NoError(t, apisv1beta1.SchemeBuilder.AddToScheme(scheme))

	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	user := &userv1beta1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-namespaced-user",
			Namespace: "test-namespace",
		},
		Spec: userv1beta1.UserSpec{
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{Name: "default"},
			},
			ForProvider: userv1beta1.UserParameters{
				KeyName:      "test-application-key",
				Capabilities: []string{"listFiles", "readFiles"},
				WriteSecretToRef: xpv1.SecretReference{
					Name:      "backblaze-credentials",
					Namespace: "test-namespace",
				},
			},
		},
	}

	ctx := context.Background()

	// Create user
	err := client.Create(ctx, user)
	require.NoError(t, err, "Should create v1beta1 namespaced user")

	// Retrieve user
	retrieved := &userv1beta1.User{}
	err = client.Get(ctx, types.NamespacedName{Name: "test-namespaced-user", Namespace: "test-namespace"}, retrieved)
	require.NoError(t, err, "Should retrieve v1beta1 namespaced user")

	// Validate properties
	assert.Equal(t, "test-application-key", retrieved.Spec.ForProvider.KeyName)
	assert.Equal(t, []string{"listFiles", "readFiles"}, retrieved.Spec.ForProvider.Capabilities)
	assert.Equal(t, "test-namespace", retrieved.Namespace)
	assert.Equal(t, "backblaze-credentials", retrieved.Spec.ForProvider.WriteSecretToRef.Name)
	assert.Equal(t, "test-namespace", retrieved.Spec.ForProvider.WriteSecretToRef.Namespace)
}

// TestV1Beta1PolicyNamespaced validates v1beta1 namespaced policy functionality
func TestV1Beta1PolicyNamespaced(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, policyv1beta1.SchemeBuilder.AddToScheme(scheme))
	require.NoError(t, apisv1beta1.SchemeBuilder.AddToScheme(scheme))

	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	policy := &policyv1beta1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-namespaced-policy",
			Namespace: "test-namespace",
		},
		Spec: policyv1beta1.PolicySpec{
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{Name: "default"},
			},
			ForProvider: policyv1beta1.PolicyParameters{
				AllowBucket: "test-bucket",
				PolicyName:  "test-policy",
				Description: "Test policy for bucket access",
			},
		},
	}

	ctx := context.Background()

	// Create policy
	err := client.Create(ctx, policy)
	require.NoError(t, err, "Should create v1beta1 namespaced policy")

	// Retrieve policy
	retrieved := &policyv1beta1.Policy{}
	err = client.Get(ctx, types.NamespacedName{Name: "test-namespaced-policy", Namespace: "test-namespace"}, retrieved)
	require.NoError(t, err, "Should retrieve v1beta1 namespaced policy")

	// Validate properties
	assert.Equal(t, "test-bucket", retrieved.Spec.ForProvider.AllowBucket)
	assert.Equal(t, "test-policy", retrieved.Spec.ForProvider.PolicyName)
	assert.Equal(t, "Test policy for bucket access", retrieved.Spec.ForProvider.Description)
	assert.Equal(t, "test-namespace", retrieved.Namespace)
}

// TestNamespaceIsolation validates that namespaced resources are properly isolated
func TestNamespaceIsolation(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, bucketv1beta1.SchemeBuilder.AddToScheme(scheme))
	require.NoError(t, apisv1beta1.SchemeBuilder.AddToScheme(scheme))

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	ctx := context.Background()

	// Create buckets in different namespaces
	bucket1 := &bucketv1beta1.Bucket{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "same-name-bucket",
			Namespace: "namespace1",
		},
		Spec: bucketv1beta1.BucketSpec{
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{Name: "default"},
			},
			ForProvider: bucketv1beta1.BucketParameters{
				BucketName: "bucket-in-namespace1",
				Region:     "us-west-001",
			},
		},
	}

	bucket2 := &bucketv1beta1.Bucket{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "same-name-bucket", // Same name, different namespace
			Namespace: "namespace2",
		},
		Spec: bucketv1beta1.BucketSpec{
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{Name: "default"},
			},
			ForProvider: bucketv1beta1.BucketParameters{
				BucketName: "bucket-in-namespace2",
				Region:     "us-west-001",
			},
		},
	}

	// Create both buckets
	require.NoError(t, client.Create(ctx, bucket1))
	require.NoError(t, client.Create(ctx, bucket2))

	// Retrieve from each namespace
	retrieved1 := &bucketv1beta1.Bucket{}
	err := client.Get(ctx, types.NamespacedName{Name: "same-name-bucket", Namespace: "namespace1"}, retrieved1)
	require.NoError(t, err)

	retrieved2 := &bucketv1beta1.Bucket{}
	err = client.Get(ctx, types.NamespacedName{Name: "same-name-bucket", Namespace: "namespace2"}, retrieved2)
	require.NoError(t, err)

	// Validate isolation
	assert.Equal(t, "bucket-in-namespace1", retrieved1.Spec.ForProvider.BucketName)
	assert.Equal(t, "bucket-in-namespace2", retrieved2.Spec.ForProvider.BucketName)
	assert.Equal(t, "namespace1", retrieved1.Namespace)
	assert.Equal(t, "namespace2", retrieved2.Namespace)
}

// TestDualVersionCoexistence validates that v1 and v1beta1 can coexist
func TestDualVersionCoexistence(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, bucketv1.SchemeBuilder.AddToScheme(scheme))
	require.NoError(t, bucketv1beta1.SchemeBuilder.AddToScheme(scheme))
	require.NoError(t, apisv1beta1.SchemeBuilder.AddToScheme(scheme))

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	ctx := context.Background()

	// Create v1 cluster-scoped bucket
	v1Bucket := &bucketv1.Bucket{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster-scoped-bucket",
		},
		Spec: bucketv1.BucketSpec{
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{Name: "default"},
			},
			ForProvider: bucketv1.BucketParameters{
				BucketName: "v1-bucket",
				Region:     "us-west-001",
			},
		},
	}

	// Create v1beta1 namespaced bucket
	v1beta1Bucket := &bucketv1beta1.Bucket{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "namespaced-bucket",
			Namespace: "test-namespace",
		},
		Spec: bucketv1beta1.BucketSpec{
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{Name: "default"},
			},
			ForProvider: bucketv1beta1.BucketParameters{
				BucketName: "v1beta1-bucket",
				Region:     "us-west-001",
			},
		},
	}

	// Create both versions
	require.NoError(t, client.Create(ctx, v1Bucket))
	require.NoError(t, client.Create(ctx, v1beta1Bucket))

	// Retrieve both versions
	retrievedV1 := &bucketv1.Bucket{}
	err := client.Get(ctx, types.NamespacedName{Name: "cluster-scoped-bucket"}, retrievedV1)
	require.NoError(t, err)

	retrievedV1Beta1 := &bucketv1beta1.Bucket{}
	err = client.Get(ctx, types.NamespacedName{Name: "namespaced-bucket", Namespace: "test-namespace"}, retrievedV1Beta1)
	require.NoError(t, err)

	// Validate both work independently
	assert.Equal(t, "v1-bucket", retrievedV1.Spec.ForProvider.BucketName)
	assert.Equal(t, "v1beta1-bucket", retrievedV1Beta1.Spec.ForProvider.BucketName)
	assert.Empty(t, retrievedV1.Namespace) // Cluster-scoped has no namespace
	assert.Equal(t, "test-namespace", retrievedV1Beta1.Namespace)
}

// TestAPIGroupConsistency validates API group naming consistency
func TestAPIGroupConsistency(t *testing.T) {
	// v1 APIs (cluster-scoped)
	assert.Equal(t, "bucket.backblaze.crossplane.io", bucketv1.Group)
	assert.Equal(t, "user.backblaze.crossplane.io", userv1.Group)
	assert.Equal(t, "policy.backblaze.crossplane.io", policyv1.Group)

	// v1beta1 APIs (namespaced with .m. pattern)
	assert.Equal(t, "bucket.backblaze.m.crossplane.io", bucketv1beta1.Group)
	assert.Equal(t, "user.backblaze.m.crossplane.io", userv1beta1.Group)
	assert.Equal(t, "policy.backblaze.m.crossplane.io", policyv1beta1.Group)

	// Validate version consistency
	assert.Equal(t, "v1", bucketv1.Version)
	assert.Equal(t, "v1beta1", bucketv1beta1.Version)
}

// BenchmarkResourceCreation benchmarks resource creation performance
func BenchmarkResourceCreation(b *testing.B) {
	scheme := runtime.NewScheme()
	_ = bucketv1beta1.SchemeBuilder.AddToScheme(scheme)
	_ = apisv1beta1.SchemeBuilder.AddToScheme(scheme)

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bucket := &bucketv1beta1.Bucket{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "benchmark-bucket-" + string(rune(i)),
				Namespace: "test",
			},
			Spec: bucketv1beta1.BucketSpec{
				ResourceSpec: xpv1.ResourceSpec{
					ProviderConfigReference: &xpv1.Reference{Name: "default"},
				},
				ForProvider: bucketv1beta1.BucketParameters{
					BucketName: "benchmark-bucket-" + string(rune(i)),
					Region:     "us-west-001",
				},
			},
		}
		_ = client.Create(ctx, bucket)
	}
}