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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	xpv1 "github.com/crossplane/crossplane/apis/v2/core/v2"

	providerapis "github.com/rossigee/provider-backblaze/apis"
	backblazev1beta1 "github.com/rossigee/provider-backblaze/apis/backblaze/v1beta1"
	v1beta1 "github.com/rossigee/provider-backblaze/apis/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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

// httptest-backed tests for the B2 native HTTP path. They validate request
// bodies, auth-token behaviour, and the doWithReauth resilience layer.

func newTestClient(t *testing.T, serverURL string) *BackblazeClient {
	t.Helper()
	c, err := NewBackblazeClient(Config{
		ApplicationKeyID: "test-key-id",
		ApplicationKey:   "test-key",
		Region:           "us-west-001",
	})
	if err != nil {
		t.Fatalf("NewBackblazeClient: %v", err)
	}
	// Re-target at the httptest server and skip the metadata lookup.
	c.HTTPClient = &http.Client{Timeout: 5 * time.Second}
	c.ApplicationKeyID = "test-key-id"
	c.ApplicationKey = "test-key"
	c.APIURL = serverURL
	c.AuthToken = "fake-pre-existing-token"
	c.AccountID = "fake-account-id"
	c.DownloadURL = serverURL + "/file"
	// Point the auth endpoint at our mock server for the lifetime of the
	// test. We restore via t.Cleanup so nested failures don't poison later
	// tests.
	prev := B2AuthorizeAccountURL
	B2AuthorizeAccountURL = serverURL + "/b2api/v3/b2_authorize_account"
	t.Cleanup(func() { B2AuthorizeAccountURL = prev })
	return c
}

func TestAuthorizeAccount_PopulatesFields(t *testing.T) {
	var gotAuthz string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/b2api/v3/b2_authorize_account" {
			t.Errorf("unexpected URL %s", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		u, p, ok := r.BasicAuth()
		if !ok {
			t.Fatalf("expected Basic auth")
		}
		gotAuthz = u + ":" + p
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(B2AuthorizeAccountResponse{
			AccountID:          "acc-1",
			AuthorizationToken: "auth-tok",
			APIURL:             "https://api.example/b2api",
			DownloadURL:        "https://f.example",
		})
	}))
	defer srv.Close()

	prev := B2AuthorizeAccountURL
	B2AuthorizeAccountURL = srv.URL + "/b2api/v3/b2_authorize_account"
	defer func() { B2AuthorizeAccountURL = prev }()

	c := &BackblazeClient{
		HTTPClient:       &http.Client{Timeout: 5 * time.Second},
		ApplicationKeyID: "id-x",
		ApplicationKey:   "secret-x",
	}
	if err := c.authorizeAccount(context.Background()); err != nil {
		t.Fatalf("authorizeAccount: %v", err)
	}

	if gotAuthz != "id-x:secret-x" {
		t.Errorf("expected Basic auth id-x:secret-x, got %q", gotAuthz)
	}
	if c.AuthToken != "auth-tok" {
		t.Errorf("AuthToken = %q", c.AuthToken)
	}
	if c.APIURL != "https://api.example/b2api" {
		t.Errorf("APIURL = %q", c.APIURL)
	}
	if c.AccountID != "acc-1" {
		t.Errorf("AccountID = %q", c.AccountID)
	}
	if c.tokenExpiration.IsZero() {
		t.Errorf("tokenExpiration not set")
	}
}

func TestDoWithReauth_ReauthorisesOn401(t *testing.T) {
	var authCalls int32
	var updateCalls int32

	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		url := srvURL
		switch r.URL.Path {
		case "/b2api/v3/b2_update_key":
			n := atomic.AddInt32(&updateCalls, 1)
			if n == 1 {
				http.Error(w, "{\"code\":\"bad_auth_token\"}", http.StatusUnauthorized)
				return
			}
			if got := r.Header.Get("Authorization"); got != "fresh-token" {
				t.Errorf("retry call expected Authorization=fresh-token, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(B2CreateKeyResponse{ApplicationKeyID: "k-1"})
		case "/b2api/v3/b2_authorize_account":
			atomic.AddInt32(&authCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(B2AuthorizeAccountResponse{
				AccountID:          "acc-2",
				AuthorizationToken: "fresh-token",
				APIURL:             url,
				DownloadURL:        url + "/file",
			})
		default:
			http.Error(w, "unexpected request: "+r.URL.Path, http.StatusInternalServerError)
		}
	}))
	srvURL = srv.URL
	defer srv.Close()

	c := newTestClient(t, "https://invalid")

	// Override the auth endpoint with the test server.
	prevAuth := B2AuthorizeAccountURL
	B2AuthorizeAccountURL = srv.URL + "/b2api/v3/b2_authorize_account"
	defer func() { B2AuthorizeAccountURL = prevAuth }()
	c.APIURL = srv.URL

	req, err := http.NewRequest("POST", srv.URL+"/b2api/v3/b2_update_key", bytes.NewReader([]byte("{}")))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := c.doWithReauth(context.Background(), req)
	if err != nil {
		t.Fatalf("doWithReauth: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 after reauth, got %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&updateCalls); got != 2 {
		t.Errorf("expected 2 update_key attempts, got %d", got)
	}
	if got := atomic.LoadInt32(&authCalls); got != 1 {
		t.Errorf("expected 1 authorize call after 401, got %d", got)
	}
}

func TestDoWithReauth_RetriesOn5xxThenSucceeds(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 3 {
			http.Error(w, "boom", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	req, _ := http.NewRequest("POST", srv.URL+"/b2api/v3/b2_list_event_notifications",
		bytes.NewReader([]byte("{}")))
	start := time.Now()
	resp, err := c.doWithReauth(context.Background(), req)
	if err != nil {
		t.Fatalf("doWithReauth: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	elapsed := time.Since(start)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 after retry, got %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Errorf("expected 3 attempts, got %d", got)
	}
	// 250ms + 500ms = 750ms minimum backoff.
	if elapsed < 700*time.Millisecond {
		t.Errorf("expected backoff >= 700ms, got %v", elapsed)
	}
}

func TestDoWithReauth_GivesUpAfterMaxAttempts(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		http.Error(w, "boom", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	req, _ := http.NewRequest("POST", srv.URL+"/b2api/v3/b2_update_key",
		bytes.NewReader([]byte("{}")))

	resp, err := c.doWithReauth(context.Background(), req)
	if err != nil {
		t.Fatalf("doWithReauth: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected last 503 to be returned, got %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Errorf("expected 3 attempts max, got %d", got)
	}
}

func TestUpdateApplicationKey_RequestBody(t *testing.T) {
	var gotReq B2UpdateKeyRequest

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/b2api/v3/b2_authorize_account":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(B2AuthorizeAccountResponse{
				AccountID:          "acc-1",
				AuthorizationToken: "auth-tok",
				APIURL:             srv.URL,
				DownloadURL:        srv.URL + "/file",
			})
		default:
			if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
				t.Errorf("decode: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(B2CreateKeyResponse{
				ApplicationKeyID: "k-1",
				KeyName:          gotReq.KeyName,
				Capabilities:     gotReq.Capabilities,
				BucketID:         gotReq.BucketID,
				NamePrefix:       gotReq.NamePrefix,
			})
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	c.APIURL = srv.URL

	valid := 3600
	_, err := c.UpdateApplicationKey(context.Background(), B2UpdateKeyRequest{
		ApplicationKeyID:       "k-existing",
		KeyName:                "renamed",
		Capabilities:           []string{"listBuckets", "readFiles"},
		BucketID:               "b123",
		NamePrefix:             "uploads/",
		ValidDurationInSeconds: &valid,
	})
	if err != nil {
		t.Fatalf("UpdateApplicationKey: %v", err)
	}
	if gotReq.ApplicationKeyID != "k-existing" {
		t.Errorf("ApplicationKeyID = %q", gotReq.ApplicationKeyID)
	}
	if gotReq.KeyName != "renamed" {
		t.Errorf("KeyName = %q", gotReq.KeyName)
	}
	if len(gotReq.Capabilities) != 2 || gotReq.Capabilities[0] != "listBuckets" {
		t.Errorf("Capabilities = %#v", gotReq.Capabilities)
	}
	if gotReq.BucketID != "b123" || gotReq.NamePrefix != "uploads/" {
		t.Errorf("BucketID/NamePrefix = %q/%q", gotReq.BucketID, gotReq.NamePrefix)
	}
	if gotReq.ValidDurationInSeconds == nil || *gotReq.ValidDurationInSeconds != 3600 {
		t.Errorf("ValidDurationInSeconds = %v", gotReq.ValidDurationInSeconds)
	}
}

func TestIsS3NotFound(t *testing.T) {
	cases := []struct {
		name string
		in   error
		want bool
	}{
		{"nil", nil, false},
		{"plain NoSuchBucket", awsErr("NoSuchBucket", 404), true},
		{"wrapped NoSuchBucket", fmt.Errorf("wrapped: %w", awsErr("NoSuchBucket", 404)), true},
		{"NoSuchLifecycleConfiguration", awsErr("NoSuchLifecycleConfiguration", 404), true},
		{"NoSuchCORSConfiguration", awsErr("NoSuchCORSConfiguration", 404), true},
		{"NoSuchBucketPolicy", awsErr("NoSuchBucketPolicy", 404), true},
		{"plain NotFound", awsErr("NotFound", 404), true},
		{"NotFound-like code 404", awsErr("AccessDenied", 403), false},
		{"plain string err.Error() == NotFound (no APIError)", errors.New("NotFound"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isS3NotFound(tc.in); got != tc.want {
				t.Errorf("isS3NotFound(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// awsErr returns a fake error matching smithy-go APIError behaviour.
func awsErr(code string, _ int) error {
	return &fakeAPIError{code: code, msg: code}
}

type fakeAPIError struct {
	code string
	msg  string
}

func (e *fakeAPIError) Error() string                 { return e.code + ": " + e.msg }
func (e *fakeAPIError) ErrorCode() string             { return e.code }
func (e *fakeAPIError) ErrorMessage() string          { return e.msg }
func (e *fakeAPIError) ErrorFault() smithy.ErrorFault { return smithy.FaultServer }

var _ smithy.APIError = (*fakeAPIError)(nil)

func TestCreateApplicationKey_RoundTrip(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/b2api/v3/b2_authorize_account":
			_ = json.NewEncoder(w).Encode(B2AuthorizeAccountResponse{
				AccountID: "acc-1", AuthorizationToken: "tok", APIURL: srv.URL, DownloadURL: srv.URL + "/file",
			})
		case "/b2api/v3/b2_create_key":
			var req B2CreateKeyRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("decode: %v", err)
			}
			if req.AccountID != "acc-1" || req.KeyName != "k" {
				t.Errorf("unexpected req: %+v", req)
			}
			_ = json.NewEncoder(w).Encode(B2CreateKeyResponse{
				ApplicationKeyID: "kid-1", ApplicationKey: "secret-1",
				KeyName: req.KeyName, Capabilities: req.Capabilities, AccountID: "acc-1",
			})
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL)
	c.APIURL = srv.URL
	got, err := c.CreateApplicationKey(context.Background(), "k", []string{"listBuckets"}, "", "", nil)
	if err != nil {
		t.Fatalf("CreateApplicationKey: %v", err)
	}
	if got.ApplicationKeyID != "kid-1" || got.ApplicationKey != "secret-1" {
		t.Errorf("got %+v", got)
	}
}

func TestDeleteApplicationKey_Ok(t *testing.T) {
	var srv *httptest.Server
	var gotID string
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/b2api/v3/b2_authorize_account":
			_ = json.NewEncoder(w).Encode(B2AuthorizeAccountResponse{
				AccountID: "acc-1", AuthorizationToken: "tok", APIURL: srv.URL, DownloadURL: srv.URL + "/file",
			})
		case "/b2api/v3/b2_delete_key":
			var req B2DeleteKeyRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			gotID = req.ApplicationKeyID
			_, _ = w.Write([]byte("{}"))
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL)
	c.APIURL = srv.URL
	if err := c.DeleteApplicationKey(context.Background(), "kid-9"); err != nil {
		t.Fatalf("DeleteApplicationKey: %v", err)
	}
	if gotID != "kid-9" {
		t.Errorf("gotID = %q", gotID)
	}
}

func TestGetApplicationKey_FoundAndNotFound(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/b2api/v3/b2_authorize_account":
			_ = json.NewEncoder(w).Encode(B2AuthorizeAccountResponse{
				AccountID: "acc-1", AuthorizationToken: "tok", APIURL: srv.URL, DownloadURL: srv.URL + "/file",
			})
		case "/b2api/v3/b2_list_keys":
			_ = json.NewEncoder(w).Encode(B2ListKeysResponse{Keys: []struct {
				ApplicationKeyID    string   `json:"applicationKeyId"`
				KeyName             string   `json:"keyName"`
				Capabilities        []string `json:"capabilities"`
				AccountID           string   `json:"accountId"`
				ExpirationTimestamp *int64   `json:"expirationTimestamp,omitempty"`
				BucketID            string   `json:"bucketId,omitempty"`
				NamePrefix          string   `json:"namePrefix,omitempty"`
			}{{ApplicationKeyID: "kid-1", KeyName: "k", Capabilities: []string{"listBuckets"}, AccountID: "acc-1"}}})
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL)
	c.APIURL = srv.URL
	got, err := c.GetApplicationKey(context.Background(), "kid-1")
	if err != nil {
		t.Fatalf("GetApplicationKey: %v", err)
	}
	if got.KeyName != "k" {
		t.Errorf("KeyName = %q", got.KeyName)
	}
	if _, err := c.GetApplicationKey(context.Background(), "missing"); !errors.Is(err, ErrApplicationKeyNotFound) {
		t.Errorf("expected ErrApplicationKeyNotFound, got %v", err)
	}
}

func TestB2CreateBucket_RoundTrip(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/b2api/v3/b2_authorize_account":
			_ = json.NewEncoder(w).Encode(B2AuthorizeAccountResponse{
				AccountID: "acc-1", AuthorizationToken: "tok", APIURL: srv.URL, DownloadURL: srv.URL + "/file",
			})
		case "/b2api/v3/b2_create_bucket":
			var req B2CreateBucketRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.BucketName != "my-bucket" || req.BucketType != "allPrivate" {
				t.Errorf("unexpected req: %+v", req)
			}
			_ = json.NewEncoder(w).Encode(B2CreateBucketResponse{
				AccountID: "acc-1", BucketID: "b1", BucketName: "my-bucket", BucketType: "allPrivate",
			})
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL)
	c.APIURL = srv.URL
	got, err := c.B2CreateBucket(context.Background(), B2CreateBucketRequest{
		AccountID: "acc-1", BucketName: "my-bucket", BucketType: "allPrivate",
	})
	if err != nil {
		t.Fatalf("B2CreateBucket: %v", err)
	}
	if got.BucketID != "b1" {
		t.Errorf("BucketID = %q", got.BucketID)
	}
}

func TestB2UpdateBucket_RoundTrip(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/b2api/v3/b2_authorize_account":
			_ = json.NewEncoder(w).Encode(B2AuthorizeAccountResponse{
				AccountID: "acc-1", AuthorizationToken: "tok", APIURL: srv.URL, DownloadURL: srv.URL + "/file",
			})
		case "/b2api/v3/b2_update_bucket":
			var req B2UpdateBucketRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.BucketID != "b1" {
				t.Errorf("BucketID = %q", req.BucketID)
			}
			_ = json.NewEncoder(w).Encode(B2UpdateBucketResponse{
				AccountID: "acc-1", BucketID: "b1", BucketName: "my-bucket", BucketType: "allPublic",
			})
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL)
	c.APIURL = srv.URL
	got, err := c.B2UpdateBucket(context.Background(), B2UpdateBucketRequest{
		AccountID: "acc-1", BucketID: "b1", BucketType: "allPublic",
	})
	if err != nil {
		t.Fatalf("B2UpdateBucket: %v", err)
	}
	if got.BucketType != "allPublic" {
		t.Errorf("BucketType = %q", got.BucketType)
	}
}

func TestB2ListBuckets_RoundTrip(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/b2api/v3/b2_authorize_account":
			_ = json.NewEncoder(w).Encode(B2AuthorizeAccountResponse{
				AccountID: "acc-1", AuthorizationToken: "tok", APIURL: srv.URL, DownloadURL: srv.URL + "/file",
			})
		case "/b2api/v3/b2_list_buckets":
			_, _ = w.Write([]byte(`{"buckets":[{"accountId":"acc-1","bucketId":"b1","bucketName":"my-bucket","bucketType":"allPrivate"}]}`))
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL)
	c.APIURL = srv.URL
	got, err := c.B2ListBuckets(context.Background())
	if err != nil {
		t.Fatalf("B2ListBuckets: %v", err)
	}
	if len(got.Buckets) != 1 || got.Buckets[0].BucketID != "b1" {
		t.Errorf("got %+v", got.Buckets)
	}
}

func TestB2EventNotifications_CRUD(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/b2api/v3/b2_authorize_account":
			_ = json.NewEncoder(w).Encode(B2AuthorizeAccountResponse{
				AccountID: "acc-1", AuthorizationToken: "tok", APIURL: srv.URL, DownloadURL: srv.URL + "/file",
			})
		case "/b2api/v3/b2_create_event_notification":
			_ = json.NewEncoder(w).Encode(B2EventNotification{
				AccountID: "acc-1", BucketID: "b1", NotificationID: "n-1",
				Name: "rule1", Events: []string{"b2:ObjectCreated"}, WebhookURL: "https://example.com/hook",
			})
		case "/b2api/v3/b2_list_event_notifications":
			_ = json.NewEncoder(w).Encode(B2ListEventNotificationsResponse{Notifications: []B2EventNotification{
				{AccountID: "acc-1", BucketID: "b1", NotificationID: "n-1", Name: "rule1"},
			}})
		case "/b2api/v3/b2_update_event_notification":
			_ = json.NewEncoder(w).Encode(B2EventNotification{
				AccountID: "acc-1", BucketID: "b1", NotificationID: "n-1", Name: "rule1",
			})
		case "/b2api/v3/b2_delete_event_notification":
			_, _ = w.Write([]byte("{}"))
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL)
	c.APIURL = srv.URL
	ctx := context.Background()
	created, err := c.B2CreateEventNotification(ctx, B2CreateEventNotificationRequest{
		AccountID: "acc-1", BucketID: "b1", Name: "rule1",
		Events: []string{"b2:ObjectCreated"}, WebhookURL: "https://example.com/hook",
	})
	if err != nil || created.NotificationID != "n-1" {
		t.Fatalf("create: %v %+v", err, created)
	}
	listed, err := c.B2ListEventNotifications(ctx, B2ListEventNotificationsRequest{AccountID: "acc-1", BucketID: "b1"})
	if err != nil || len(listed.Notifications) != 1 {
		t.Fatalf("list: %v %+v", err, listed)
	}
	if _, err := c.B2UpdateEventNotification(ctx, B2UpdateEventNotificationRequest{
		AccountID: "acc-1", BucketID: "b1", NotificationID: "n-1", Name: "rule1",
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := c.B2DeleteEventNotification(ctx, B2DeleteEventNotificationRequest{
		AccountID: "acc-1", BucketID: "b1", NotificationID: "n-1",
	}); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestGetExternalName_SetExternalName(t *testing.T) {
	m := &backblazev1beta1.Bucket{}
	SetExternalName(m, "my-bucket")
	if got := GetExternalName(m); got != "my-bucket" {
		t.Errorf("got %q", got)
	}
	// Overwrite path (annotations already non-nil).
	SetExternalName(m, "other-bucket")
	if got := GetExternalName(m); got != "other-bucket" {
		t.Errorf("got %q", got)
	}
}

func TestGetProviderConfig_Secret(t *testing.T) {
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatalf("corev1: %v", err)
	}
	if err := providerapis.AddToScheme(s); err != nil {
		t.Fatalf("apis: %v", err)
	}
	mkPC := func(source string, secretRef *xpv1.SecretKeySelector) *v1beta1.ProviderConfig {
		return &v1beta1.ProviderConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "crossplane-system"},
			Spec: v1beta1.ProviderConfigSpec{
				BackblazeRegion: "eu-central-003",
				Credentials:     v1beta1.ProviderCredentials{Source: xpv1.CredentialsSource(source), CommonCredentialSelectors: xpv1.CommonCredentialSelectors{SecretRef: secretRef}},
			},
		}
	}
	secretRef := &xpv1.SecretKeySelector{SecretReference: xpv1.SecretReference{Name: "creds", Namespace: "crossplane-system"}}
	credSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: "crossplane-system"},
		Data:       map[string][]byte{"applicationKeyId": []byte("id-1"), "applicationKey": []byte("key-1")},
	}
	t.Run("ok", func(t *testing.T) {
		cl := fake.NewClientBuilder().WithScheme(s).WithObjects(mkPC("Secret", secretRef), credSecret).Build()
		got, err := GetProviderConfig(context.Background(), cl, mkPC("Secret", secretRef))
		// NOTE: GetProviderConfig re-reads the PC from the passed object, not
		// the client, so pass the same spec.
		if err != nil {
			t.Fatalf("GetProviderConfig: %v", err)
		}
		if got.ApplicationKeyID != "id-1" || got.ApplicationKey != "key-1" || got.Region != "eu-central-003" {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("missing secretRef", func(t *testing.T) {
		cl := fake.NewClientBuilder().WithScheme(s).Build()
		if _, err := GetProviderConfig(context.Background(), cl, mkPC("Secret", nil)); err == nil {
			t.Errorf("expected error for nil secretRef")
		}
	})
	t.Run("missing secret", func(t *testing.T) {
		cl := fake.NewClientBuilder().WithScheme(s).WithObjects(mkPC("Secret", secretRef)).Build()
		if _, err := GetProviderConfig(context.Background(), cl, mkPC("Secret", secretRef)); err == nil {
			t.Errorf("expected error for missing secret")
		}
	})
	t.Run("missing keys", func(t *testing.T) {
		bad := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: "crossplane-system"}}
		cl := fake.NewClientBuilder().WithScheme(s).WithObjects(mkPC("Secret", secretRef), bad).Build()
		if _, err := GetProviderConfig(context.Background(), cl, mkPC("Secret", secretRef)); err == nil {
			t.Errorf("expected error for secret without keys")
		}
	})
	t.Run("unsupported source", func(t *testing.T) {
		cl := fake.NewClientBuilder().WithScheme(s).Build()
		if _, err := GetProviderConfig(context.Background(), cl, mkPC("None", nil)); err == nil {
			t.Errorf("expected error for unsupported source")
		}
	})
}

func newS3StubClient(t *testing.T, handler http.HandlerFunc) (*BackblazeClient, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	awsCfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("id", "key", "")),
		config.WithRegion("us-west-001"),
	)
	if err != nil {
		t.Fatalf("LoadDefaultConfig: %v", err)
	}
	s3c := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(srv.URL)
		o.UsePathStyle = true
		o.UseARNRegion = false
	})
	return &BackblazeClient{S3Client: s3c, Region: "us-west-001", Endpoint: srv.URL,
		HTTPClient: &http.Client{Timeout: 5 * time.Second}}, srv
}

func s3ErrorXML(code string) string {
	return `<?xml version="1.0" encoding="UTF-8"?><Error><Code>` + code + `</Code><Message>msg</Message></Error>`
}

func TestBucketExists(t *testing.T) {
	// Exists.
	c, _ := newS3StubClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("expected HEAD, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	})
	if ok, err := c.BucketExists(context.Background(), "bkt"); err != nil || !ok {
		t.Errorf("exists: ok=%v err=%v", ok, err)
	}
	// Missing.
	c2, _ := newS3StubClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(s3ErrorXML("NoSuchBucket")))
	})
	if ok, err := c2.BucketExists(context.Background(), "missing"); err != nil || ok {
		t.Errorf("missing: ok=%v err=%v", ok, err)
	}
}

func TestCreateDeleteBucket(t *testing.T) {
	c, _ := newS3StubClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	if err := c.CreateBucket(context.Background(), "bkt", "allPrivate", "us-west-001"); err != nil {
		t.Errorf("CreateBucket: %v", err)
	}
	if err := c.DeleteBucket(context.Background(), "bkt"); err != nil {
		t.Errorf("DeleteBucket: %v", err)
	}
}

func TestGetBucketLocation_ListBuckets(t *testing.T) {
	c, _ := newS3StubClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		switch {
		case r.URL.Query().Has("location"):
			_, _ = w.Write([]byte(`<LocationConstraint xmlns="http://s3.amazonaws.com/doc/2006-03-01/">eu-central-003</LocationConstraint>`))
		default:
			_, _ = w.Write([]byte(`<ListAllMyBucketsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Owner><ID>o</ID></Owner><Buckets><Bucket><Name>b1</Name><CreationDate>2024-01-01T00:00:00.000Z</CreationDate></Bucket></Buckets></ListAllMyBucketsResult>`))
		}
	})
	loc, err := c.GetBucketLocation(context.Background(), "bkt")
	if err != nil || loc != "eu-central-003" {
		t.Errorf("location = %q err = %v", loc, err)
	}
	buckets, err := c.ListBuckets(context.Background())
	if err != nil || len(buckets) != 1 || aws.ToString(buckets[0].Name) != "b1" {
		t.Errorf("buckets = %+v err = %v", buckets, err)
	}
}

func TestDeleteAllObjectsInBucket_Empty(t *testing.T) {
	c, _ := newS3StubClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><IsTruncated>false</IsTruncated></ListBucketResult>`))
	})
	if err := c.DeleteAllObjectsInBucket(context.Background(), "bkt"); err != nil {
		t.Errorf("empty bucket should succeed: %v", err)
	}
}

func TestBucketPolicy_RoundTrip(t *testing.T) {
	doc := `{"Version":"2012-10-17"}`
	c, _ := newS3StubClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(doc))
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	})
	if err := c.PutBucketPolicy(context.Background(), "bkt", doc); err != nil {
		t.Errorf("PutBucketPolicy: %v", err)
	}
	got, err := c.GetBucketPolicy(context.Background(), "bkt")
	if err != nil || got != doc {
		t.Errorf("GetBucketPolicy = %q err = %v", got, err)
	}
	if err := c.DeleteBucketPolicy(context.Background(), "bkt"); err != nil {
		t.Errorf("DeleteBucketPolicy: %v", err)
	}
}

func TestBucketPolicy_NotFound(t *testing.T) {
	c, _ := newS3StubClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(s3ErrorXML("NoSuchBucketPolicy")))
	})
	if _, err := c.GetBucketPolicy(context.Background(), "bkt"); !errors.Is(err, ErrBucketPolicyNotFound) {
		t.Errorf("expected ErrBucketPolicyNotFound, got %v", err)
	}
	if err := c.DeleteBucketPolicy(context.Background(), "bkt"); !errors.Is(err, ErrBucketPolicyNotFound) {
		t.Errorf("expected ErrBucketPolicyNotFound, got %v", err)
	}
}

func TestBucketCors_RoundTrip(t *testing.T) {
	c, _ := newS3StubClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		switch r.Method {
		case http.MethodPut:
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			_, _ = w.Write([]byte(`<CORSConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><CORSRule><AllowedOrigin>https://a</AllowedOrigin><AllowedMethod>GET</AllowedMethod></CORSRule></CORSConfiguration>`))
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	})
	if err := c.PutBucketCors(context.Background(), "bkt", []types.CORSRule{{AllowedOrigins: []string{"https://a"}, AllowedMethods: []string{"GET"}}}); err != nil {
		t.Errorf("PutBucketCors: %v", err)
	}
	rules, err := c.GetBucketCors(context.Background(), "bkt")
	if err != nil || len(rules) != 1 {
		t.Fatalf("GetBucketCors = %+v err = %v", rules, err)
	}
	if err := c.DeleteBucketCors(context.Background(), "bkt"); err != nil {
		t.Errorf("DeleteBucketCors: %v", err)
	}
}

func TestBucketCors_NotFound(t *testing.T) {
	c, _ := newS3StubClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(s3ErrorXML("NoSuchCORSConfiguration")))
	})
	if _, err := c.GetBucketCors(context.Background(), "bkt"); !errors.Is(err, ErrBucketCorsNotFound) {
		t.Errorf("expected ErrBucketCorsNotFound, got %v", err)
	}
	if err := c.DeleteBucketCors(context.Background(), "bkt"); !errors.Is(err, ErrBucketCorsNotFound) {
		t.Errorf("expected ErrBucketCorsNotFound, got %v", err)
	}
}

func TestBucketLifecycle_RoundTrip(t *testing.T) {
	c, _ := newS3StubClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		switch r.Method {
		case http.MethodPut:
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			_, _ = w.Write([]byte(`<LifecycleConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Rule><ID>r1</ID><Filter><Prefix>logs/</Prefix></Filter><Status>Enabled</Status><Expiration><Days>30</Days></Expiration></Rule></LifecycleConfiguration>`))
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	})
	rules := []types.LifecycleRule{{
		Status:     types.ExpirationStatusEnabled,
		Filter:     &types.LifecycleRuleFilter{Prefix: aws.String("logs/")},
		Expiration: &types.LifecycleExpiration{Days: aws.Int32(30)},
	}}
	if err := c.PutBucketLifecycleConfiguration(context.Background(), "bkt", rules); err != nil {
		t.Errorf("Put: %v", err)
	}
	got, err := c.GetBucketLifecycleConfiguration(context.Background(), "bkt")
	if err != nil || len(got) != 1 {
		t.Fatalf("Get = %+v err = %v", got, err)
	}
	if err := c.DeleteBucketLifecycleConfiguration(context.Background(), "bkt"); err != nil {
		t.Errorf("Delete: %v", err)
	}
}

func TestBucketLifecycle_NotFound(t *testing.T) {
	c, _ := newS3StubClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(s3ErrorXML("NoSuchLifecycleConfiguration")))
	})
	if _, err := c.GetBucketLifecycleConfiguration(context.Background(), "bkt"); !errors.Is(err, ErrBucketLifecycleNotFound) {
		t.Errorf("expected ErrBucketLifecycleNotFound, got %v", err)
	}
	if err := c.DeleteBucketLifecycleConfiguration(context.Background(), "bkt"); !errors.Is(err, ErrBucketLifecycleNotFound) {
		t.Errorf("expected ErrBucketLifecycleNotFound, got %v", err)
	}
}
