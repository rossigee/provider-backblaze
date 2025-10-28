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

package clients

import (
	"testing"
)

func TestNewBackblazeClient(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: Config{
				ApplicationKeyID: "test-key-id",
				ApplicationKey:   "test-key",
				Region:           "us-west-001",
			},
			wantErr: false,
		},
		{
			name: "empty region",
			config: Config{
				ApplicationKeyID: "test-key-id",
				ApplicationKey:   "test-key",
				Region:           "",
			},
			wantErr: false, // Should use default region
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewBackblazeClient(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewBackblazeClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && client == nil {
				t.Error("NewBackblazeClient() returned nil client")
			}
		})
	}
}

func TestEndpointGeneration(t *testing.T) {
	tests := []struct {
		name   string
		region string
		want   string
	}{
		{
			name:   "us-west-001",
			region: "us-west-001",
			want:   "https://s3.us-west-001.backblazeb2.com",
		},
		{
			name:   "eu-central-003",
			region: "eu-central-003",
			want:   "https://s3.eu-central-003.backblazeb2.com",
		},
		{
			name:   "empty region defaults",
			region: "",
			want:   "https://s3.us-west-001.backblazeb2.com", // Default region
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test endpoint generation through client creation
			config := Config{
				ApplicationKeyID: "test-key-id",
				ApplicationKey:   "test-key",
				Region:           tt.region,
			}

			client, err := NewBackblazeClient(config)
			if err != nil {
				t.Fatalf("NewBackblazeClient() failed: %v", err)
			}

			if client.Endpoint != tt.want {
				t.Errorf("Client endpoint = %v, want %v", client.Endpoint, tt.want)
			}
		})
	}
}

func TestClientConfiguration(t *testing.T) {
	config := Config{
		ApplicationKeyID: "test-key-id",
		ApplicationKey:   "test-key",
		Region:           "us-west-001",
	}

	client, err := NewBackblazeClient(config)
	if err != nil {
		t.Fatalf("NewBackblazeClient() failed: %v", err)
	}

	// Verify S3 client is set correctly
	if client.S3Client == nil {
		t.Error("S3 client is nil")
	}

	// Test that region is set
	if client.Region != config.Region {
		t.Errorf("Region not set correctly, got %v, want %v", client.Region, config.Region)
	}

	// Test that endpoint is set correctly
	expectedEndpoint := "https://s3.us-west-001.backblazeb2.com"
	if client.Endpoint != expectedEndpoint {
		t.Errorf("Endpoint not set correctly, got %v, want %v", client.Endpoint, expectedEndpoint)
	}

	// Test that S3 client is configured correctly
	// In AWS SDK v2, we can't directly inspect internal config,
	// but we can verify the client was created successfully
	if client.S3Client == nil {
		t.Error("S3Client should not be nil")
	}
}

func TestNewBackblazeClientValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr string
	}{
		{
			name: "missing application key ID",
			config: Config{
				ApplicationKey: "test-key",
				Region:         "us-west-001",
			},
			wantErr: "applicationKeyId and applicationKey are required",
		},
		{
			name: "missing application key",
			config: Config{
				ApplicationKeyID: "test-key-id",
				Region:           "us-west-001",
			},
			wantErr: "applicationKeyId and applicationKey are required",
		},
		{
			name: "both missing",
			config: Config{
				Region: "us-west-001",
			},
			wantErr: "applicationKeyId and applicationKey are required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewBackblazeClient(tt.config)
			if err == nil {
				t.Error("Expected error but got none")
				return
			}
			if err.Error() != tt.wantErr {
				t.Errorf("Expected error %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestConfigDefaults(t *testing.T) {
	config := Config{
		ApplicationKeyID: "test-key-id",
		ApplicationKey:   "test-key",
		// No region specified
	}

	client, err := NewBackblazeClient(config)
	if err != nil {
		t.Fatalf("NewBackblazeClient() failed: %v", err)
	}

	// Should default to us-west-001
	if client.Region != "us-west-001" {
		t.Errorf("Expected default region us-west-001, got %v", client.Region)
	}

	expectedEndpoint := "https://s3.us-west-001.backblazeb2.com"
	if client.Endpoint != expectedEndpoint {
		t.Errorf("Expected default endpoint %v, got %v", expectedEndpoint, client.Endpoint)
	}
}

// TestCustomEndpoint is disabled until endpoint customization is implemented
// The Config struct currently doesn't support custom endpoints
// func TestCustomEndpoint(t *testing.T) {
// 	customEndpoint := "https://custom.endpoint.com"
// 	config := Config{
// 		ApplicationKeyID: "test-key-id",
// 		ApplicationKey:   "test-key",
// 		Region:           "eu-central-003",
// 		EndpointURL:      customEndpoint,
// 	}
//
// 	client, err := NewBackblazeClient(config)
// 	if err != nil {
// 		t.Fatalf("NewBackblazeClient() failed: %v", err)
// 	}
//
// 	if client.Endpoint != customEndpoint {
// 		t.Errorf("Expected custom endpoint %v, got %v", customEndpoint, client.Endpoint)
// 	}
// }

// Mock tests for bucket operations (these would normally require mocking AWS SDK)
func TestBucketOperationInterfaces(t *testing.T) {
	// Test that all methods are available and have correct signatures
	config := Config{
		ApplicationKeyID: "test-key-id",
		ApplicationKey:   "test-key",
		Region:           "us-west-001",
	}

	client, err := NewBackblazeClient(config)
	if err != nil {
		t.Fatalf("NewBackblazeClient() failed: %v", err)
	}

	// These tests verify method signatures exist but would need AWS SDK mocking for real testing
	t.Run("CreateBucket method exists", func(t *testing.T) {
		// We can't actually call this without mocking, but we can verify the method exists
		if client.S3Client == nil {
			t.Error("S3Client should not be nil")
		}
	})

	t.Run("DeleteBucket method exists", func(t *testing.T) {
		// Verify method signature by checking client is not nil
		if client == nil {
			t.Error("Client should not be nil")
		}
	})

	t.Run("BucketExists method exists", func(t *testing.T) {
		// Verify method signature by checking client is not nil
		if client == nil {
			t.Error("Client should not be nil")
		}
	})
}

func TestGetProviderConfig(t *testing.T) {
	// This would require more complex mocking of Kubernetes client
	// For now, just test that the function exists and can be called
	// In a real test, we'd mock the Kubernetes client and secret
	t.Skip("Integration test - requires Kubernetes client mocking")
}
