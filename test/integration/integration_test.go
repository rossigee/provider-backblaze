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
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/rossigee/provider-backblaze/internal/clients"
)

// Integration test configuration
const (
	testTimeout     = 5 * time.Minute
	cleanupTimeout  = 30 * time.Second
	testBucketPrefix = "provider-backblaze-test"
)

// TestConfig holds configuration for integration tests
type TestConfig struct {
	ApplicationKeyID string
	ApplicationKey   string
	Region           string
	BucketName       string
	SkipCleanup      bool
}

// setupTestConfig loads configuration from environment variables
func setupTestConfig(t *testing.T) *TestConfig {
	config := &TestConfig{
		ApplicationKeyID: os.Getenv("B2_APPLICATION_KEY_ID"),
		ApplicationKey:   os.Getenv("B2_APPLICATION_KEY"),
		Region:          os.Getenv("B2_REGION"),
		SkipCleanup:     os.Getenv("SKIP_CLEANUP") == "true",
	}

	// Set defaults
	if config.Region == "" {
		config.Region = "us-west-001"
	}

	// Generate unique bucket name for this test run
	config.BucketName = fmt.Sprintf("%s-%d", testBucketPrefix, time.Now().Unix())

	// Skip tests if credentials are not provided
	if config.ApplicationKeyID == "" || config.ApplicationKey == "" {
		t.Skip("Skipping integration tests - B2_APPLICATION_KEY_ID and B2_APPLICATION_KEY environment variables must be set")
	}

	return config
}

// setupBackblazeClient creates a real Backblaze client for testing
func setupBackblazeClient(t *testing.T, config *TestConfig) *clients.BackblazeClient {
	clientConfig := clients.Config{
		ApplicationKeyID: config.ApplicationKeyID,
		ApplicationKey:   config.ApplicationKey,
		Region:          config.Region,
	}

	client, err := clients.NewBackblazeClient(clientConfig)
	if err != nil {
		t.Fatalf("Failed to create Backblaze client: %v", err)
	}

	return client
}

func TestBackblazeClientIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	config := setupTestConfig(t)
	client := setupBackblazeClient(t, config)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	t.Run("TestClientConnection", func(t *testing.T) {
		// Test that we can connect and list buckets
		buckets, err := client.ListBuckets(ctx)
		if err != nil {
			t.Fatalf("Failed to list buckets: %v", err)
		}
		t.Logf("Successfully connected to Backblaze B2. Found %d buckets", len(buckets))
	})
}

func TestBucketLifecycleIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	config := setupTestConfig(t)
	client := setupBackblazeClient(t, config)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	bucketName := config.BucketName

	// Cleanup function
	cleanup := func() {
		if !config.SkipCleanup {
			t.Logf("Cleaning up test bucket: %s", bucketName)
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), cleanupTimeout)
			defer cleanupCancel()
			
			// Try to delete all objects first
			_ = client.DeleteAllObjectsInBucket(cleanupCtx, bucketName)
			// Then delete the bucket
			_ = client.DeleteBucket(cleanupCtx, bucketName)
		}
	}
	defer cleanup()

	t.Run("CreateBucket", func(t *testing.T) {
		err := client.CreateBucket(ctx, bucketName, "allPrivate", config.Region)
		if err != nil {
			t.Fatalf("Failed to create bucket %s: %v", bucketName, err)
		}
		t.Logf("Successfully created bucket: %s", bucketName)
	})

	t.Run("BucketExists", func(t *testing.T) {
		exists, err := client.BucketExists(ctx, bucketName)
		if err != nil {
			t.Fatalf("Failed to check bucket existence: %v", err)
		}
		if !exists {
			t.Fatalf("Bucket %s should exist but doesn't", bucketName)
		}
		t.Logf("Confirmed bucket exists: %s", bucketName)
	})

	t.Run("GetBucketLocation", func(t *testing.T) {
		location, err := client.GetBucketLocation(ctx, bucketName)
		if err != nil {
			t.Fatalf("Failed to get bucket location: %v", err)
		}
		t.Logf("Bucket location: %s (expected: %s)", location, config.Region)
		// Note: Backblaze B2 may return empty location for default region
		if location != "" && location != config.Region {
			t.Errorf("Expected bucket location %s, got %s", config.Region, location)
		}
	})

	t.Run("ListBucketsContainsOurs", func(t *testing.T) {
		buckets, err := client.ListBuckets(ctx)
		if err != nil {
			t.Fatalf("Failed to list buckets: %v", err)
		}

		found := false
		for _, bucket := range buckets {
			if bucket.Name != nil && *bucket.Name == bucketName {
				found = true
				t.Logf("Found our bucket in list: %s (created: %v)", *bucket.Name, bucket.CreationDate)
				break
			}
		}

		if !found {
			t.Errorf("Our bucket %s was not found in the bucket list", bucketName)
		}
	})

	t.Run("DeleteBucket", func(t *testing.T) {
		err := client.DeleteBucket(ctx, bucketName)
		if err != nil {
			t.Fatalf("Failed to delete bucket %s: %v", bucketName, err)
		}
		t.Logf("Successfully deleted bucket: %s", bucketName)

		// Verify it's gone
		exists, err := client.BucketExists(ctx, bucketName)
		if err != nil {
			t.Fatalf("Failed to check bucket existence after deletion: %v", err)
		}
		if exists {
			t.Errorf("Bucket %s should not exist after deletion", bucketName)
		}
	})
}

func TestApplicationKeyLifecycleIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	config := setupTestConfig(t)
	client := setupBackblazeClient(t, config)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	keyName := fmt.Sprintf("test-key-%d", time.Now().Unix())
	var keyID string

	// Cleanup function
	cleanup := func() {
		if !config.SkipCleanup && keyID != "" {
			t.Logf("Cleaning up test application key: %s", keyID)
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), cleanupTimeout)
			defer cleanupCancel()
			_ = client.DeleteApplicationKey(cleanupCtx, keyID)
		}
	}
	defer cleanup()

	t.Run("CreateApplicationKey", func(t *testing.T) {
		capabilities := []string{"listBuckets", "listFiles", "readFiles"}
		
		key, err := client.CreateApplicationKey(ctx, keyName, capabilities, "", "", nil)
		if err != nil {
			t.Fatalf("Failed to create application key: %v", err)
		}

		if key.ApplicationKeyID == "" {
			t.Fatal("Created key should have an ID")
		}
		if key.ApplicationKey == "" {
			t.Fatal("Created key should have a secret")
		}
		if key.KeyName != keyName {
			t.Errorf("Expected key name %s, got %s", keyName, key.KeyName)
		}

		keyID = key.ApplicationKeyID
		t.Logf("Successfully created application key: %s (ID: %s)", keyName, keyID)
	})

	t.Run("GetApplicationKey", func(t *testing.T) {
		if keyID == "" {
			t.Skip("No key ID from previous test")
		}

		key, err := client.GetApplicationKey(ctx, keyID)
		if err != nil {
			t.Fatalf("Failed to get application key: %v", err)
		}

		if key.ApplicationKeyID != keyID {
			t.Errorf("Expected key ID %s, got %s", keyID, key.ApplicationKeyID)
		}
		if key.KeyName != keyName {
			t.Errorf("Expected key name %s, got %s", keyName, key.KeyName)
		}

		t.Logf("Successfully retrieved application key: %s", key.KeyName)
	})

	t.Run("DeleteApplicationKey", func(t *testing.T) {
		if keyID == "" {
			t.Skip("No key ID from previous test")
		}

		err := client.DeleteApplicationKey(ctx, keyID)
		if err != nil {
			t.Fatalf("Failed to delete application key: %v", err)
		}

		t.Logf("Successfully deleted application key: %s", keyID)

		// Verify it's gone
		_, err = client.GetApplicationKey(ctx, keyID)
		if err == nil {
			t.Error("Application key should not exist after deletion")
		}
		if err.Error() != "application key not found" {
			t.Errorf("Expected 'application key not found' error, got: %v", err)
		}
	})
}

func TestBucketPolicyIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	config := setupTestConfig(t)
	client := setupBackblazeClient(t, config)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	bucketName := config.BucketName

	// Cleanup function
	cleanup := func() {
		if !config.SkipCleanup {
			t.Logf("Cleaning up test bucket and policy: %s", bucketName)
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), cleanupTimeout)
			defer cleanupCancel()
			
			// Delete policy first, then bucket
			_ = client.DeleteBucketPolicy(cleanupCtx, bucketName)
			_ = client.DeleteAllObjectsInBucket(cleanupCtx, bucketName)
			_ = client.DeleteBucket(cleanupCtx, bucketName)
		}
	}
	defer cleanup()

	// Create bucket for policy testing
	t.Run("SetupBucketForPolicy", func(t *testing.T) {
		err := client.CreateBucket(ctx, bucketName, "allPrivate", config.Region)
		if err != nil {
			t.Fatalf("Failed to create bucket %s: %v", bucketName, err)
		}
		t.Logf("Created test bucket for policy testing: %s", bucketName)
	})

	policyDocument := fmt.Sprintf(`{
		"Version": "2012-10-17",
		"Statement": [
			{
				"Effect": "Allow",
				"Principal": "*",
				"Action": "s3:GetObject",
				"Resource": "arn:aws:s3:::%s/*"
			}
		]
	}`, bucketName)

	t.Run("PutBucketPolicy", func(t *testing.T) {
		err := client.PutBucketPolicy(ctx, bucketName, policyDocument)
		if err != nil {
			t.Fatalf("Failed to put bucket policy: %v", err)
		}
		t.Logf("Successfully applied policy to bucket: %s", bucketName)
	})

	t.Run("GetBucketPolicy", func(t *testing.T) {
		policy, err := client.GetBucketPolicy(ctx, bucketName)
		if err != nil {
			t.Fatalf("Failed to get bucket policy: %v", err)
		}

		if policy == "" {
			t.Fatal("Retrieved policy should not be empty")
		}

		t.Logf("Successfully retrieved bucket policy (length: %d bytes)", len(policy))
	})

	t.Run("DeleteBucketPolicy", func(t *testing.T) {
		err := client.DeleteBucketPolicy(ctx, bucketName)
		if err != nil {
			t.Fatalf("Failed to delete bucket policy: %v", err)
		}
		t.Logf("Successfully deleted bucket policy")

		// Verify it's gone
		_, err = client.GetBucketPolicy(ctx, bucketName)
		if err == nil {
			t.Error("Bucket policy should not exist after deletion")
		}
		if err.Error() != "bucket policy not found" {
			t.Logf("Expected 'bucket policy not found' error, got: %v (this may be acceptable)", err)
		}
	})
}

func TestB2AuthenticationIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	config := setupTestConfig(t)
	client := setupBackblazeClient(t, config)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	t.Run("B2NativeAPIAuthentication", func(t *testing.T) {
		// Test B2 native API authentication by creating a key
		keyName := fmt.Sprintf("auth-test-key-%d", time.Now().Unix())
		capabilities := []string{"listBuckets"}
		
		key, err := client.CreateApplicationKey(ctx, keyName, capabilities, "", "", nil)
		if err != nil {
			t.Fatalf("Failed to authenticate with B2 API: %v", err)
		}

		// Cleanup
		defer func() {
			if !config.SkipCleanup {
				_ = client.DeleteApplicationKey(ctx, key.ApplicationKeyID)
			}
		}()

		if key.ApplicationKeyID == "" {
			t.Fatal("Authentication succeeded but no key ID returned")
		}

		t.Logf("Successfully authenticated with B2 native API and created key: %s", key.ApplicationKeyID)
	})

	t.Run("S3CompatibleAPIAuthentication", func(t *testing.T) {
		// Test S3-compatible API authentication by listing buckets
		buckets, err := client.ListBuckets(ctx)
		if err != nil {
			t.Fatalf("Failed to authenticate with S3-compatible API: %v", err)
		}

		t.Logf("Successfully authenticated with S3-compatible API. Found %d buckets", len(buckets))
	})
}

func TestErrorHandlingIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	config := setupTestConfig(t)
	client := setupBackblazeClient(t, config)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	t.Run("NonExistentBucket", func(t *testing.T) {
		nonExistentBucket := "this-bucket-should-not-exist-12345"
		
		exists, err := client.BucketExists(ctx, nonExistentBucket)
		if err != nil {
			t.Fatalf("BucketExists should handle non-existent buckets gracefully: %v", err)
		}
		if exists {
			t.Errorf("Bucket %s should not exist", nonExistentBucket)
		}
	})

	t.Run("NonExistentApplicationKey", func(t *testing.T) {
		nonExistentKeyID := "this-key-should-not-exist-12345"
		
		_, err := client.GetApplicationKey(ctx, nonExistentKeyID)
		if err == nil {
			t.Error("GetApplicationKey should return error for non-existent key")
		}
		if err.Error() != "application key not found" {
			t.Logf("Expected 'application key not found', got: %v (this may be acceptable)", err)
		}
	})

	t.Run("NonExistentBucketPolicy", func(t *testing.T) {
		nonExistentBucket := "this-bucket-should-not-exist-12345"
		
		_, err := client.GetBucketPolicy(ctx, nonExistentBucket)
		if err == nil {
			t.Error("GetBucketPolicy should return error for non-existent bucket")
		}
	})
}

// BenchmarkBackblazeOperations provides performance benchmarks
func BenchmarkBackblazeOperations(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping benchmarks in short mode")
	}

	config := &TestConfig{
		ApplicationKeyID: os.Getenv("B2_APPLICATION_KEY_ID"),
		ApplicationKey:   os.Getenv("B2_APPLICATION_KEY"),
		Region:          os.Getenv("B2_REGION"),
	}

	if config.Region == "" {
		config.Region = "us-west-001"
	}

	if config.ApplicationKeyID == "" || config.ApplicationKey == "" {
		b.Skip("Skipping benchmarks - B2_APPLICATION_KEY_ID and B2_APPLICATION_KEY environment variables must be set")
	}

	clientConfig := clients.Config{
		ApplicationKeyID: config.ApplicationKeyID,
		ApplicationKey:   config.ApplicationKey,
		Region:          config.Region,
	}

	client, err := clients.NewBackblazeClient(clientConfig)
	if err != nil {
		b.Fatalf("Failed to create Backblaze client: %v", err)
	}

	ctx := context.Background()

	b.Run("ListBuckets", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := client.ListBuckets(ctx)
			if err != nil {
				b.Fatalf("ListBuckets failed: %v", err)
			}
		}
	})

	b.Run("BucketExists", func(b *testing.B) {
		testBucket := "nonexistent-bucket-for-benchmark"
		for i := 0; i < b.N; i++ {
			_, err := client.BucketExists(ctx, testBucket)
			if err != nil {
				b.Fatalf("BucketExists failed: %v", err)
			}
		}
	})
}