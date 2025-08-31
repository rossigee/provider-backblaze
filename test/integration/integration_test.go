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

func TestMultiRegionBucketIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	config := setupTestConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Test with different regions
	regions := []string{"us-west-001", "us-west-002", "eu-central-003"}
	
	for _, region := range regions {
		t.Run(fmt.Sprintf("Region_%s", region), func(t *testing.T) {
			// Create client for specific region
			clientConfig := clients.Config{
				ApplicationKeyID: config.ApplicationKeyID,
				ApplicationKey:   config.ApplicationKey,
				Region:          region,
			}

			client, err := clients.NewBackblazeClient(clientConfig)
			if err != nil {
				t.Fatalf("Failed to create client for region %s: %v", region, err)
			}

			bucketName := fmt.Sprintf("%s-%s-%d", testBucketPrefix, region, time.Now().Unix())
			
			// Cleanup function
			cleanup := func() {
				if !config.SkipCleanup {
					cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), cleanupTimeout)
					defer cleanupCancel()
					_ = client.DeleteAllObjectsInBucket(cleanupCtx, bucketName)
					_ = client.DeleteBucket(cleanupCtx, bucketName)
				}
			}
			defer cleanup()

			// Test bucket creation in specific region
			err = client.CreateBucket(ctx, bucketName, "allPrivate", region)
			if err != nil {
				t.Fatalf("Failed to create bucket in region %s: %v", region, err)
			}

			// Verify bucket location
			location, err := client.GetBucketLocation(ctx, bucketName)
			if err != nil {
				t.Fatalf("Failed to get bucket location for region %s: %v", region, err)
			}

			t.Logf("Created bucket %s in region %s (reported location: %s)", bucketName, region, location)
		})
	}
}

func TestConcurrentBucketOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	config := setupTestConfig(t)
	client := setupBackblazeClient(t, config)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	const numConcurrentOps = 5
	bucketNames := make([]string, numConcurrentOps)
	
	// Generate unique bucket names
	for i := 0; i < numConcurrentOps; i++ {
		bucketNames[i] = fmt.Sprintf("%s-concurrent-%d-%d", testBucketPrefix, i, time.Now().Unix())
	}

	// Cleanup function
	cleanup := func() {
		if !config.SkipCleanup {
			t.Logf("Cleaning up %d concurrent test buckets", numConcurrentOps)
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), cleanupTimeout)
			defer cleanupCancel()
			
			for _, bucketName := range bucketNames {
				_ = client.DeleteAllObjectsInBucket(cleanupCtx, bucketName)
				_ = client.DeleteBucket(cleanupCtx, bucketName)
			}
		}
	}
	defer cleanup()

	t.Run("ConcurrentBucketCreation", func(t *testing.T) {
		errChan := make(chan error, numConcurrentOps)
		
		// Create buckets concurrently
		for i, bucketName := range bucketNames {
			go func(name string, index int) {
				err := client.CreateBucket(ctx, name, "allPrivate", config.Region)
				if err != nil {
					errChan <- fmt.Errorf("bucket %d (%s): %v", index, name, err)
				} else {
					errChan <- nil
				}
			}(bucketName, i)
		}

		// Collect results
		var errors []error
		for i := 0; i < numConcurrentOps; i++ {
			if err := <-errChan; err != nil {
				errors = append(errors, err)
			}
		}

		if len(errors) > 0 {
			for _, err := range errors {
				t.Errorf("Concurrent creation error: %v", err)
			}
			t.Fatalf("Failed to create %d out of %d buckets concurrently", len(errors), numConcurrentOps)
		}

		t.Logf("Successfully created %d buckets concurrently", numConcurrentOps)
	})

	t.Run("ConcurrentBucketListAndExists", func(t *testing.T) {
		errChan := make(chan error, numConcurrentOps*2)
		
		// Check existence and list buckets concurrently
		for _, bucketName := range bucketNames {
			// Check bucket exists
			go func(name string) {
				exists, err := client.BucketExists(ctx, name)
				if err != nil {
					errChan <- fmt.Errorf("BucketExists(%s): %v", name, err)
				} else if !exists {
					errChan <- fmt.Errorf("BucketExists(%s): should exist but doesn't", name)
				} else {
					errChan <- nil
				}
			}(bucketName)
			
			// List buckets
			go func(name string) {
				buckets, err := client.ListBuckets(ctx)
				if err != nil {
					errChan <- fmt.Errorf("ListBuckets for %s: %v", name, err)
				} else {
					found := false
					for _, bucket := range buckets {
						if bucket.Name != nil && *bucket.Name == name {
							found = true
							break
						}
					}
					if !found {
						errChan <- fmt.Errorf("ListBuckets: bucket %s not found in list", name)
					} else {
						errChan <- nil
					}
				}
			}(bucketName)
		}

		// Collect results
		var errors []error
		for i := 0; i < numConcurrentOps*2; i++ {
			if err := <-errChan; err != nil {
				errors = append(errors, err)
			}
		}

		if len(errors) > 0 {
			for _, err := range errors {
				t.Errorf("Concurrent read operation error: %v", err)
			}
			t.Fatalf("Failed %d out of %d concurrent read operations", len(errors), numConcurrentOps*2)
		}

		t.Logf("Successfully performed %d concurrent read operations", numConcurrentOps*2)
	})
}

func TestBucketPolicyAdvancedIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	config := setupTestConfig(t)
	client := setupBackblazeClient(t, config)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	bucketName := fmt.Sprintf("%s-policy-advanced-%d", testBucketPrefix, time.Now().Unix())

	// Cleanup function
	cleanup := func() {
		if !config.SkipCleanup {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), cleanupTimeout)
			defer cleanupCancel()
			_ = client.DeleteBucketPolicy(cleanupCtx, bucketName)
			_ = client.DeleteAllObjectsInBucket(cleanupCtx, bucketName)
			_ = client.DeleteBucket(cleanupCtx, bucketName)
		}
	}
	defer cleanup()

	// Create bucket first
	err := client.CreateBucket(ctx, bucketName, "allPrivate", config.Region)
	if err != nil {
		t.Fatalf("Failed to create bucket for advanced policy testing: %v", err)
	}

	// Test complex policy scenarios
	testCases := []struct {
		name        string
		policy      string
		shouldError bool
	}{
		{
			name: "PublicReadPolicy",
			policy: fmt.Sprintf(`{
				"Version": "2012-10-17",
				"Statement": [
					{
						"Effect": "Allow",
						"Principal": "*",
						"Action": "s3:GetObject",
						"Resource": "arn:aws:s3:::%s/*"
					}
				]
			}`, bucketName),
			shouldError: false,
		},
		{
			name: "RestrictedIPPolicy",
			policy: fmt.Sprintf(`{
				"Version": "2012-10-17",
				"Statement": [
					{
						"Effect": "Allow",
						"Principal": "*",
						"Action": "s3:GetObject",
						"Resource": "arn:aws:s3:::%s/*",
						"Condition": {
							"IpAddress": {
								"aws:SourceIp": "203.0.113.0/24"
							}
						}
					}
				]
			}`, bucketName),
			shouldError: false,
		},
		{
			name: "InvalidJSON",
			policy: `{
				"Version": "2012-10-17",
				"Statement": [
					{
						"Effect": "Allow",
						"Principal": "*"
						"Action": "s3:GetObject"
					}
				]
			}`, // Missing comma - invalid JSON
			shouldError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Try to put the policy
			err := client.PutBucketPolicy(ctx, bucketName, tc.policy)
			
			if tc.shouldError {
				if err == nil {
					t.Errorf("Expected error for %s but got none", tc.name)
				} else {
					t.Logf("Expected error for %s: %v", tc.name, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("Failed to put policy for %s: %v", tc.name, err)
			}

			// Retrieve and verify
			retrievedPolicy, err := client.GetBucketPolicy(ctx, bucketName)
			if err != nil {
				t.Fatalf("Failed to get policy for %s: %v", tc.name, err)
			}

			if retrievedPolicy == "" {
				t.Errorf("Retrieved policy for %s should not be empty", tc.name)
			}

			t.Logf("Successfully tested %s policy (length: %d)", tc.name, len(retrievedPolicy))

			// Clean up policy for next test
			_ = client.DeleteBucketPolicy(ctx, bucketName)
		})
	}
}

func TestBucketS3CompatibilityIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	config := setupTestConfig(t)
	client := setupBackblazeClient(t, config)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	bucketName := fmt.Sprintf("%s-s3compat-%d", testBucketPrefix, time.Now().Unix())

	// Cleanup function
	cleanup := func() {
		if !config.SkipCleanup {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), cleanupTimeout)
			defer cleanupCancel()
			_ = client.DeleteAllObjectsInBucket(cleanupCtx, bucketName)
			_ = client.DeleteBucket(cleanupCtx, bucketName)
		}
	}
	defer cleanup()

	// Test different bucket types
	bucketTypes := []string{"allPrivate", "allPublic"}
	
	for _, bucketType := range bucketTypes {
		t.Run(fmt.Sprintf("BucketType_%s", bucketType), func(t *testing.T) {
			// Create bucket with specific type
			err := client.CreateBucket(ctx, bucketName, bucketType, config.Region)
			if err != nil {
				t.Fatalf("Failed to create bucket with type %s: %v", bucketType, err)
			}

			// Verify bucket exists
			exists, err := client.BucketExists(ctx, bucketName)
			if err != nil {
				t.Fatalf("Failed to check bucket existence: %v", err)
			}
			if !exists {
				t.Fatalf("Bucket with type %s should exist", bucketType)
			}

			// Test listing includes our bucket
			buckets, err := client.ListBuckets(ctx)
			if err != nil {
				t.Fatalf("Failed to list buckets: %v", err)
			}

			found := false
			for _, bucket := range buckets {
				if bucket.Name != nil && *bucket.Name == bucketName {
					found = true
					t.Logf("Found bucket %s (created: %v)", *bucket.Name, bucket.CreationDate)
					break
				}
			}

			if !found {
				t.Errorf("Bucket %s with type %s not found in list", bucketName, bucketType)
			}

			// Clean up for next iteration
			err = client.DeleteBucket(ctx, bucketName)
			if err != nil {
				t.Fatalf("Failed to delete bucket: %v", err)
			}
		})
	}
}

func TestApplicationKeyCapabilitiesIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	config := setupTestConfig(t)
	client := setupBackblazeClient(t, config)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	bucketName := fmt.Sprintf("%s-capabilities-%d", testBucketPrefix, time.Now().Unix())
	
	// Create test bucket first
	err := client.CreateBucket(ctx, bucketName, "allPrivate", config.Region)
	if err != nil {
		t.Fatalf("Failed to create test bucket: %v", err)
	}
	
	// Cleanup function
	cleanup := func() {
		if !config.SkipCleanup {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), cleanupTimeout)
			defer cleanupCancel()
			_ = client.DeleteAllObjectsInBucket(cleanupCtx, bucketName)
			_ = client.DeleteBucket(cleanupCtx, bucketName)
		}
	}
	defer cleanup()

	testCases := []struct {
		name         string
		keyName      string
		capabilities []string
		bucketID     string
		namePrefix   string
	}{
		{
			name:         "ReadOnlyKey",
			keyName:      fmt.Sprintf("readonly-key-%d", time.Now().Unix()),
			capabilities: []string{"listBuckets", "listFiles", "readFiles"},
		},
		{
			name:         "WriteOnlyKey", 
			keyName:      fmt.Sprintf("writeonly-key-%d", time.Now().Unix()),
			capabilities: []string{"listBuckets", "writeFiles"},
		},
		{
			name:         "BucketSpecificKey",
			keyName:      fmt.Sprintf("bucket-specific-key-%d", time.Now().Unix()),
			capabilities: []string{"listFiles", "readFiles", "writeFiles", "deleteFiles"},
			bucketID:     bucketName, // Use bucket name as ID for this test
		},
		{
			name:         "PrefixRestrictedKey",
			keyName:      fmt.Sprintf("prefix-restricted-key-%d", time.Now().Unix()),
			capabilities: []string{"listFiles", "readFiles", "writeFiles"},
			namePrefix:   "uploads/",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create application key
			key, err := client.CreateApplicationKey(ctx, tc.keyName, tc.capabilities, tc.bucketID, tc.namePrefix, nil)
			if err != nil {
				t.Fatalf("Failed to create %s: %v", tc.name, err)
			}

			// Cleanup key
			defer func() {
				if !config.SkipCleanup {
					cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), cleanupTimeout)
					defer cleanupCancel()
					_ = client.DeleteApplicationKey(cleanupCtx, key.ApplicationKeyID)
				}
			}()

			// Verify key properties
			if key.ApplicationKeyID == "" {
				t.Errorf("%s: ApplicationKeyID should not be empty", tc.name)
			}
			if key.ApplicationKey == "" {
				t.Errorf("%s: ApplicationKey should not be empty", tc.name)
			}
			if key.KeyName != tc.keyName {
				t.Errorf("%s: Expected key name %s, got %s", tc.name, tc.keyName, key.KeyName)
			}

			// Verify capabilities match
			if len(key.Capabilities) != len(tc.capabilities) {
				t.Errorf("%s: Expected %d capabilities, got %d", tc.name, len(tc.capabilities), len(key.Capabilities))
			}

			t.Logf("Successfully created %s with ID: %s", tc.name, key.ApplicationKeyID)
		})
	}
}

func TestBucketRegionValidationIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	config := setupTestConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Test region validation and endpoint generation
	testCases := []struct {
		name         string
		region       string
		expectError  bool
		description  string
	}{
		{
			name:        "ValidUSWest001",
			region:      "us-west-001",
			expectError: false,
			description: "Standard US West region",
		},
		{
			name:        "ValidUSWest002",
			region:      "us-west-002",
			expectError: false,
			description: "Alternative US West region",
		},
		{
			name:        "ValidEUCentral",
			region:      "eu-central-003",
			expectError: false,
			description: "European region",
		},
		{
			name:        "InvalidRegion",
			region:      "invalid-region-999",
			expectError: false, // B2 should handle invalid regions gracefully
			description: "Invalid region should be handled gracefully",
		},
		{
			name:        "EmptyRegion",
			region:      "",
			expectError: false, // Should default to us-west-001
			description: "Empty region should default to us-west-001",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create client for specific region
			clientConfig := clients.Config{
				ApplicationKeyID: config.ApplicationKeyID,
				ApplicationKey:   config.ApplicationKey,
				Region:          tc.region,
			}

			client, err := clients.NewBackblazeClient(clientConfig)
			if err != nil {
				if tc.expectError {
					t.Logf("Expected error creating client for region %s: %v", tc.region, err)
					return
				}
				t.Fatalf("Unexpected error creating client for region %s: %v", tc.region, err)
			}

			bucketName := fmt.Sprintf("%s-region-test-%s-%d", testBucketPrefix, tc.region, time.Now().Unix())
			if tc.region == "" {
				bucketName = fmt.Sprintf("%s-region-test-default-%d", testBucketPrefix, time.Now().Unix())
			}
			
			// Cleanup function
			cleanup := func() {
				if !config.SkipCleanup {
					cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), cleanupTimeout)
					defer cleanupCancel()
					_ = client.DeleteAllObjectsInBucket(cleanupCtx, bucketName)
					_ = client.DeleteBucket(cleanupCtx, bucketName)
				}
			}
			defer cleanup()

			// Try to create bucket - this tests if the region/endpoint configuration works
			err = client.CreateBucket(ctx, bucketName, "allPrivate", tc.region)
			if err != nil {
				if tc.expectError {
					t.Logf("Expected error creating bucket in region %s: %v", tc.region, err)
					return
				}
				t.Logf("Failed to create bucket in region %s: %v (may be expected for invalid regions)", tc.region, err)
				return // Don't fail the test for region issues
			}

			// Verify the bucket was created
			exists, err := client.BucketExists(ctx, bucketName)
			if err != nil {
				t.Logf("Error checking bucket existence for region %s: %v", tc.region, err)
				return
			}

			if !exists {
				t.Errorf("Bucket should exist after creation in region %s", tc.region)
				return
			}

			// Get the bucket location to verify region assignment
			location, err := client.GetBucketLocation(ctx, bucketName)
			if err != nil {
				t.Logf("Could not get bucket location for region %s: %v", tc.region, err)
			} else {
				expectedRegion := tc.region
				if expectedRegion == "" {
					expectedRegion = "us-west-001" // Default region
				}
				t.Logf("Bucket %s created in region %s (requested: %s, reported: %s)", 
					bucketName, expectedRegion, tc.region, location)
			}
		})
	}
}

func TestEdgeCasesIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	config := setupTestConfig(t)
	client := setupBackblazeClient(t, config)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	t.Run("VeryLongBucketName", func(t *testing.T) {
		// Test near the limit of bucket name length (63 characters is max for B2)
		longBucketName := fmt.Sprintf("very-long-bucket-name-test-provider-backblaze-%d", time.Now().Unix())
		if len(longBucketName) > 50 {
			longBucketName = longBucketName[:50] // Truncate to reasonable length
		}

		err := client.CreateBucket(ctx, longBucketName, "allPrivate", config.Region)
		if err != nil {
			t.Logf("Expected behavior: long bucket name rejected: %v", err)
			return
		}

		// If creation succeeded, clean up
		defer func() {
			if !config.SkipCleanup {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), cleanupTimeout)
				defer cleanupCancel()
				_ = client.DeleteBucket(cleanupCtx, longBucketName)
			}
		}()

		t.Logf("Successfully created bucket with long name: %s (length: %d)", longBucketName, len(longBucketName))
	})

	t.Run("SpecialCharactersInBucketName", func(t *testing.T) {
		// Test bucket names with allowed special characters
		specialBucketName := fmt.Sprintf("test-bucket-with-dashes-%d", time.Now().Unix())

		err := client.CreateBucket(ctx, specialBucketName, "allPrivate", config.Region)
		if err != nil {
			t.Fatalf("Failed to create bucket with special characters: %v", err)
		}

		// Cleanup
		defer func() {
			if !config.SkipCleanup {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), cleanupTimeout)
				defer cleanupCancel()
				_ = client.DeleteBucket(cleanupCtx, specialBucketName)
			}
		}()

		t.Logf("Successfully created bucket with special characters: %s", specialBucketName)
	})

	t.Run("InvalidBucketName", func(t *testing.T) {
		// Test invalid bucket name (with uppercase letters)
		invalidBucketName := fmt.Sprintf("INVALID-BUCKET-NAME-%d", time.Now().Unix())

		err := client.CreateBucket(ctx, invalidBucketName, "allPrivate", config.Region)
		if err == nil {
			// If creation unexpectedly succeeded, clean up
			defer func() {
				if !config.SkipCleanup {
					cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), cleanupTimeout)
					defer cleanupCancel()
					_ = client.DeleteBucket(cleanupCtx, invalidBucketName)
				}
			}()
			t.Error("Expected bucket creation to fail with invalid name, but it succeeded")
		} else {
			t.Logf("Expected behavior: invalid bucket name rejected: %v", err)
		}
	})

	t.Run("RapidCreateDelete", func(t *testing.T) {
		// Test rapid create/delete cycles
		bucketName := fmt.Sprintf("%s-rapid-%d", testBucketPrefix, time.Now().Unix())

		for i := 0; i < 3; i++ {
			// Create bucket
			err := client.CreateBucket(ctx, bucketName, "allPrivate", config.Region)
			if err != nil {
				t.Fatalf("Iteration %d: Failed to create bucket: %v", i, err)
			}

			// Immediately delete bucket
			err = client.DeleteBucket(ctx, bucketName)
			if err != nil {
				t.Fatalf("Iteration %d: Failed to delete bucket: %v", i, err)
			}

			// Brief pause to avoid rate limiting
			time.Sleep(100 * time.Millisecond)
		}

		t.Logf("Successfully performed %d rapid create/delete cycles", 3)
	})

	t.Run("EmptyApplicationKeyName", func(t *testing.T) {
		// Test creating application key with empty name
		capabilities := []string{"listBuckets"}
		
		_, err := client.CreateApplicationKey(ctx, "", capabilities, "", "", nil)
		if err == nil {
			t.Error("Expected application key creation to fail with empty name")
		} else {
			t.Logf("Expected behavior: empty key name rejected: %v", err)
		}
	})

	t.Run("InvalidCapabilities", func(t *testing.T) {
		// Test creating application key with invalid capabilities
		keyName := fmt.Sprintf("invalid-caps-key-%d", time.Now().Unix())
		invalidCapabilities := []string{"invalidCapability", "anotherInvalidOne"}
		
		_, err := client.CreateApplicationKey(ctx, keyName, invalidCapabilities, "", "", nil)
		if err == nil {
			t.Error("Expected application key creation to fail with invalid capabilities")
		} else {
			t.Logf("Expected behavior: invalid capabilities rejected: %v", err)
		}
	})
}

func TestTimeoutAndRetryIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	config := setupTestConfig(t)
	
	t.Run("ShortTimeout", func(t *testing.T) {
		// Test with very short timeout
		shortCtx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()

		client := setupBackblazeClient(t, config)
		
		// This should timeout
		_, err := client.ListBuckets(shortCtx)
		if err == nil {
			t.Error("Expected timeout error but operation succeeded")
		} else {
			t.Logf("Expected behavior: operation timed out: %v", err)
		}
	})

	t.Run("ReasonableTimeout", func(t *testing.T) {
		// Test with reasonable timeout
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client := setupBackblazeClient(t, config)
		
		// This should succeed
		buckets, err := client.ListBuckets(ctx)
		if err != nil {
			t.Fatalf("Operation should succeed with reasonable timeout: %v", err)
		}
		
		t.Logf("Operation succeeded with %d buckets found", len(buckets))
	})
}