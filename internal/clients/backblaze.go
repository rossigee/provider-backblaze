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
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/pkg/errors"

	"io"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1beta1 "github.com/rossigee/provider-backblaze/apis/v1beta1"
)

const (
	// SecretKeyApplicationKeyID is the key in the secret containing the Backblaze application key ID
	SecretKeyApplicationKeyID = "applicationKeyId"
	// SecretKeyApplicationKey is the key in the secret containing the Backblaze application key
	SecretKeyApplicationKey = "applicationKey"

	// Default Backblaze B2 regions and their S3-compatible endpoints
	DefaultRegion         = "us-west-001"
	DefaultEndpointFormat = "https://s3.%s.backblazeb2.com"
)

// B2AuthorizeAccountURL is the well-known B2 native endpoint for
// authorizing an account. It is a var (not const) so tests can re-point it
// at an httptest server.
var B2AuthorizeAccountURL = "https://api.backblazeb2.com/b2api/v3/b2_authorize_account"

// BackblazeClient represents a client for Backblaze B2 using S3-compatible API and native B2 API
type BackblazeClient struct {
	S3Client *s3.Client
	Region   string
	Endpoint string

	// B2 Native API support
	HTTPClient       *http.Client
	ApplicationKeyID string
	ApplicationKey   string
	AuthToken        string
	APIURL           string
	DownloadURL      string
	S3APIURL         string
	AccountID        string
	tokenExpiration  time.Time
}

// Config contains configuration for connecting to Backblaze B2
type Config struct {
	ApplicationKeyID string
	ApplicationKey   string
	Region           string
}

// NewBackblazeClient creates a new Backblaze B2 client using S3-compatible API.
// It calls b2_authorize_account first to obtain the real S3 endpoint (s3ApiUrl) and
// native API endpoint (apiUrl) from B2, so the S3 client uses the correct per-account
// endpoint rather than a guessed regional address.
func NewBackblazeClient(cfg Config) (*BackblazeClient, error) {
	if cfg.ApplicationKeyID == "" || cfg.ApplicationKey == "" {
		return nil, errors.New("applicationKeyId and applicationKey are required")
	}

	if cfg.Region == "" {
		cfg.Region = DefaultRegion
	}

	// Create the HTTP client for the native B2 API
	httpClient := &http.Client{Timeout: 30 * time.Second}

	// Authorize immediately so we get the real s3ApiUrl and apiUrl from B2.
	// This also populates AuthToken, APIURL, DownloadURL, S3APIURL, AccountID.
	client := &BackblazeClient{
		Region:           cfg.Region,
		HTTPClient:       httpClient,
		ApplicationKeyID: cfg.ApplicationKeyID,
		ApplicationKey:   cfg.ApplicationKey,
	}
	if err := client.authorizeAccount(context.Background()); err != nil {
		return nil, errors.Wrap(err, "b2_authorize_account failed")
	}

	// Build the S3 client targeting the actual s3ApiUrl returned by B2.
	awsCfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.ApplicationKeyID,
			cfg.ApplicationKey,
			"", // token not needed for Backblaze B2
		)),
		config.WithRegion(cfg.Region),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load AWS config")
	}

	s3Endpoint := client.S3APIURL
	if s3Endpoint == "" {
		s3Endpoint = fmt.Sprintf(DefaultEndpointFormat, cfg.Region)
	}

	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(s3Endpoint)
		o.UsePathStyle = true // Required for Backblaze B2
	})

	client.S3Client = s3Client
	client.Endpoint = s3Endpoint

	return client, nil
}

// GetProviderConfig extracts Backblaze configuration from a ProviderConfig
func GetProviderConfig(ctx context.Context, c client.Client, pc *v1beta1.ProviderConfig) (*Config, error) {
	cfg := &Config{
		Region: pc.Spec.BackblazeRegion,
	}

	switch pc.Spec.Credentials.Source {
	case "Secret":
		if pc.Spec.Credentials.SecretRef == nil || pc.Spec.Credentials.SecretRef.Name == "" {
			return nil, errors.New("secretRef.name is required when source is Secret")
		}

		secret := &corev1.Secret{}
		if err := c.Get(ctx, client.ObjectKey{
			Namespace: pc.Spec.Credentials.SecretRef.Namespace,
			Name:      pc.Spec.Credentials.SecretRef.Name,
		}, secret); err != nil {
			return nil, errors.Wrap(err, "failed to get credentials secret")
		}

		keyIDBytes, exists := secret.Data[SecretKeyApplicationKeyID]
		if !exists {
			return nil, errors.Errorf("secret %s/%s does not contain %s",
				secret.Namespace, secret.Name, SecretKeyApplicationKeyID)
		}
		cfg.ApplicationKeyID = string(keyIDBytes)

		keyBytes, exists := secret.Data[SecretKeyApplicationKey]
		if !exists {
			return nil, errors.Errorf("secret %s/%s does not contain %s",
				secret.Namespace, secret.Name, SecretKeyApplicationKey)
		}
		cfg.ApplicationKey = string(keyBytes)

	default:
		return nil, errors.Errorf("unsupported credentials source: %s", pc.Spec.Credentials.Source)
	}

	return cfg, nil
}

// CreateBucket creates a new bucket in Backblaze B2
func (c *BackblazeClient) CreateBucket(ctx context.Context, bucketName, bucketType, region string) error {
	input := &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	}

	// Set the region constraint if different from client region
	if region != "" && region != c.Region {
		input.CreateBucketConfiguration = &types.CreateBucketConfiguration{
			LocationConstraint: types.BucketLocationConstraint(region),
		}
	}

	_, err := c.S3Client.CreateBucket(ctx, input)
	if err != nil {
		return errors.Wrap(err, "failed to create bucket")
	}

	return nil
}

// DeleteBucket deletes a bucket from Backblaze B2
func (c *BackblazeClient) DeleteBucket(ctx context.Context, bucketName string) error {
	input := &s3.DeleteBucketInput{
		Bucket: aws.String(bucketName),
	}

	_, err := c.S3Client.DeleteBucket(ctx, input)
	if err != nil {
		return errors.Wrap(err, "failed to delete bucket")
	}

	return nil
}

// BucketExists checks if a bucket exists in Backblaze B2
func (c *BackblazeClient) BucketExists(ctx context.Context, bucketName string) (bool, error) {
	input := &s3.HeadBucketInput{
		Bucket: aws.String(bucketName),
	}

	_, err := c.S3Client.HeadBucket(ctx, input)
	if err != nil {
		if isS3NotFound(err) {
			return false, nil
		}
		return false, errors.Wrap(err, "failed to check bucket existence")
	}

	return true, nil
}

// GetBucketLocation returns the region of a bucket
func (c *BackblazeClient) GetBucketLocation(ctx context.Context, bucketName string) (string, error) {
	input := &s3.GetBucketLocationInput{
		Bucket: aws.String(bucketName),
	}

	result, err := c.S3Client.GetBucketLocation(ctx, input)
	if err != nil {
		return "", errors.Wrap(err, "failed to get bucket location")
	}

	return string(result.LocationConstraint), nil
}

// ListBuckets lists all buckets accessible with the current credentials
func (c *BackblazeClient) ListBuckets(ctx context.Context) ([]types.Bucket, error) {
	result, err := c.S3Client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list buckets")
	}

	return result.Buckets, nil
}

// DeleteAllObjectsInBucket deletes all objects in a bucket (for DeleteAll policy)
func (c *BackblazeClient) DeleteAllObjectsInBucket(ctx context.Context, bucketName string) error {
	// List all objects
	listInput := &s3.ListObjectsV2Input{
		Bucket: aws.String(bucketName),
	}

	for {
		result, err := c.S3Client.ListObjectsV2(ctx, listInput)
		if err != nil {
			return errors.Wrap(err, "failed to list objects")
		}

		if len(result.Contents) == 0 {
			break
		}

		// Delete objects in batch
		objects := make([]types.ObjectIdentifier, len(result.Contents))
		for i, obj := range result.Contents {
			objects[i] = types.ObjectIdentifier{
				Key: obj.Key,
			}
		}

		deleteInput := &s3.DeleteObjectsInput{
			Bucket: aws.String(bucketName),
			Delete: &types.Delete{
				Objects: objects,
			},
		}

		_, err = c.S3Client.DeleteObjects(ctx, deleteInput)
		if err != nil {
			return errors.Wrap(err, "failed to delete objects")
		}

		// Check if there are more objects to delete
		if result.IsTruncated == nil || !*result.IsTruncated {
			break
		}
		listInput.ContinuationToken = result.NextContinuationToken
	}

	return nil
}

// isS3NotFound reports whether err is an AWS S3 404-equivalent error carrying
// the standard NoSuch{Bucket,Key,...} error code. Uses smithy.APIError so
// wrapped errors (which is what the SDK returns) are matched reliably instead
// of being compared against err.Error() strings.
func isS3NotFound(err error) bool {
	var apiErr smithy.APIError
	if err == nil || !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.ErrorCode() {
	case "NotFound", "NoSuchBucket", "NoSuchKey", "NoSuchBucketPolicy",
		"NoSuchLifecycleConfiguration", "NoSuchCORSConfiguration":
		return true
	}
	return false
}

// GetExternalName extracts the external name from a managed resource
func GetExternalName(obj resource.Managed) string {
	return obj.GetAnnotations()[ExternalNameAnnotation]
}

// SetExternalName sets the external name annotation on a managed resource
func SetExternalName(obj resource.Managed, name string) {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[ExternalNameAnnotation] = name
	obj.SetAnnotations(annotations)
}

const (
	// ExternalNameAnnotation is the annotation used to store the external name
	ExternalNameAnnotation = "crossplane.io/external-name"
)

// B2 API Request/Response types

// B2AuthorizeAccountRequest represents the request to authorize account
type B2AuthorizeAccountRequest struct {
	KeyID          string `json:"keyId"`
	ApplicationKey string `json:"applicationKey"`
}

// B2AuthorizeAccountResponse represents the response from authorize account.
// B2 returns a nested structure: { accountId, authorizationToken, apiInfo: { storageApi: { apiUrl, downloadUrl, s3ApiUrl, ... } } }
type B2AuthorizeAccountResponse struct {
	AccountID          string    `json:"accountId"`
	AuthorizationToken string    `json:"authorizationToken"`
	APIInfo            B2APIInfo `json:"apiInfo"`
}

// B2APIInfo is the apiInfo section of the authorize response
type B2APIInfo struct {
	StorageAPI B2StorageAPI `json:"storageApi"`
}

// B2StorageAPI contains the actual API endpoints
type B2StorageAPI struct {
	APIURL      string `json:"apiUrl"`
	DownloadURL string `json:"downloadUrl"`
	S3APIURL    string `json:"s3ApiUrl"`
}

// B2CreateKeyRequest represents the request to create an application key
type B2CreateKeyRequest struct {
	AccountID              string   `json:"accountId"`
	Capabilities           []string `json:"capabilities"`
	KeyName                string   `json:"keyName"`
	ValidDurationInSeconds *int     `json:"validDurationInSeconds,omitempty"`
	BucketID               string   `json:"bucketId,omitempty"`
	NamePrefix             string   `json:"namePrefix,omitempty"`
}

// B2CreateKeyResponse represents the response from create key
type B2CreateKeyResponse struct {
	ApplicationKeyID    string   `json:"applicationKeyId"`
	ApplicationKey      string   `json:"applicationKey"`
	KeyName             string   `json:"keyName"`
	Capabilities        []string `json:"capabilities"`
	AccountID           string   `json:"accountId"`
	ExpirationTimestamp *int64   `json:"expirationTimestamp,omitempty"`
	BucketID            string   `json:"bucketId,omitempty"`
	NamePrefix          string   `json:"namePrefix,omitempty"`
}

// B2UpdateKeyRequest represents the request to update an application key.
// b2_update_key allows live edits to capabilities, bucketId, namePrefix and
// validDurationInSeconds. The application key secret value is never returned
// after creation, so this does not invalidate secrets in flight.
type B2UpdateKeyRequest struct {
	ApplicationKeyID string   `json:"applicationKeyId"`
	KeyName          string   `json:"keyName,omitempty"`
	Capabilities     []string `json:"capabilities,omitempty"`
	BucketID         string   `json:"bucketId,omitempty"`
	NamePrefix       string   `json:"namePrefix,omitempty"`
	// ValidDurationInSeconds must be a positive integer less than 86400000
	// (1000 days), or omitted entirely. To remove an existing expiration, send
	// 0. b2_update_key rejects other values.
	ValidDurationInSeconds *int `json:"validDurationInSeconds,omitempty"`
}

// B2DeleteKeyRequest represents the request to delete an application key
type B2DeleteKeyRequest struct {
	ApplicationKeyID string `json:"applicationKeyId"`
}

// B2ListKeysRequest represents the request to list application keys
type B2ListKeysRequest struct {
	AccountID             string `json:"accountId"`
	MaxKeyCount           int    `json:"maxKeyCount,omitempty"`
	StartApplicationKeyID string `json:"startApplicationKeyId,omitempty"`
}

// B2ListKeysResponse represents the response from list keys
type B2ListKeysResponse struct {
	Keys []struct {
		ApplicationKeyID    string   `json:"applicationKeyId"`
		KeyName             string   `json:"keyName"`
		Capabilities        []string `json:"capabilities"`
		AccountID           string   `json:"accountId"`
		ExpirationTimestamp *int64   `json:"expirationTimestamp,omitempty"`
		BucketID            string   `json:"bucketId,omitempty"`
		NamePrefix          string   `json:"namePrefix,omitempty"`
	} `json:"keys"`
	NextApplicationKeyID string `json:"nextApplicationKeyId,omitempty"`
}

// B2 API Methods

// authorizeAccount authorizes with B2 API and gets account info
// B2 Native API uses HTTP Basic Auth in the Authorization header
func (c *BackblazeClient) authorizeAccount(ctx context.Context) error {
	// Check if we already have a valid token
	if c.AuthToken != "" && time.Now().Before(c.tokenExpiration) {
		return nil
	}

	// B2 Native API uses Basic Auth in header: "applicationKeyId:applicationKey" base64 encoded
	creds := c.ApplicationKeyID + ":" + c.ApplicationKey
	encoded := base64.StdEncoding.EncodeToString([]byte(creds))

	httpReq, err := http.NewRequestWithContext(ctx, "GET", B2AuthorizeAccountURL, nil)
	if err != nil {
		return errors.Wrap(err, "failed to create HTTP request")
	}

	httpReq.Header.Set("Authorization", "Basic "+encoded)

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return errors.Wrap(err, "failed to execute HTTP request")
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return errors.Errorf("authorize account failed with status %d: %s", resp.StatusCode, string(body))
	}

	var authResp B2AuthorizeAccountResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return errors.Wrap(err, "failed to decode authorize response")
	}

	c.AuthToken = authResp.AuthorizationToken
	c.APIURL = authResp.APIInfo.StorageAPI.APIURL
	c.DownloadURL = authResp.APIInfo.StorageAPI.DownloadURL
	c.S3APIURL = authResp.APIInfo.StorageAPI.S3APIURL
	c.AccountID = authResp.AccountID
	// B2 tokens typically last 24 hours, but we'll refresh after 12 hours to be safe
	c.tokenExpiration = time.Now().Add(12 * time.Hour)

	return nil
}

// doWithReauth runs an authenticated B2 native request with two layers of
// resilience on top of the bare http.Client:
//
//   - 401 forces a re-authorize, retry once with the freshly-issued token.
//   - 5xx / connection errors retry up to 3 times with bounded exponential
//     backoff (~250 ms, 500 ms, 1 s) so transient B2 outages don't bubble up
//     as reconcile failures.
//
// Other status codes (4xx other than 401) are returned to the caller as-is.
func (c *BackblazeClient) doWithReauth(ctx context.Context, req *http.Request) (*http.Response, error) {
	const maxAttempts = 3
	backoff := 250 * time.Millisecond
	var lastResp *http.Response
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		r, err := c.HTTPClient.Do(req.Clone(ctx))
		if err != nil {
			lastErr = err
			if attempt == maxAttempts {
				return nil, err
			}
			if !sleepCtx(ctx, backoff) {
				return nil, err
			}
			backoff *= 2
			continue
		}
		if r.StatusCode == http.StatusUnauthorized && attempt < maxAttempts {
			_ = r.Body.Close()
			c.AuthToken = ""
			c.APIURL = ""
			c.tokenExpiration = time.Time{}
			if authErr := c.authorizeAccount(ctx); authErr != nil {
				return nil, errors.Wrap(authErr, "auth refresh after 401 failed")
			}
			req.Header.Set("Authorization", c.AuthToken)
			continue
		}
		if r.StatusCode >= 500 && attempt < maxAttempts {
			_ = r.Body.Close()
			if !sleepCtx(ctx, backoff) {
				return nil, ctx.Err()
			}
			backoff *= 2
			continue
		}
		return r, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return lastResp, nil
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// CreateApplicationKey creates a new application key in Backblaze B2
func (c *BackblazeClient) CreateApplicationKey(ctx context.Context, keyName string, capabilities []string, bucketID, namePrefix string, validDurationInSeconds *int) (*B2CreateKeyResponse, error) {
	if err := c.authorizeAccount(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to authorize account")
	}

	req := B2CreateKeyRequest{
		AccountID:              c.AccountID,
		KeyName:                keyName,
		Capabilities:           capabilities,
		ValidDurationInSeconds: validDurationInSeconds,
		BucketID:               bucketID,
		NamePrefix:             namePrefix,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal create key request")
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.APIURL+"/b2api/v3/b2_create_key", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create HTTP request")
	}

	httpReq.Header.Set("Authorization", c.AuthToken)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.doWithReauth(ctx, httpReq)
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute HTTP request")
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, errors.Errorf("create key failed with status %d: %s", resp.StatusCode, string(body))
	}

	var createResp B2CreateKeyResponse
	if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
		return nil, errors.Wrap(err, "failed to decode create key response")
	}

	return &createResp, nil
}

// DeleteApplicationKey deletes an application key from Backblaze B2
func (c *BackblazeClient) DeleteApplicationKey(ctx context.Context, applicationKeyID string) error {
	if err := c.authorizeAccount(ctx); err != nil {
		return errors.Wrap(err, "failed to authorize account")
	}

	req := B2DeleteKeyRequest{
		ApplicationKeyID: applicationKeyID,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return errors.Wrap(err, "failed to marshal delete key request")
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.APIURL+"/b2api/v3/b2_delete_key", bytes.NewBuffer(reqBody))
	if err != nil {
		return errors.Wrap(err, "failed to create HTTP request")
	}

	httpReq.Header.Set("Authorization", c.AuthToken)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.doWithReauth(ctx, httpReq)
	if err != nil {
		return errors.Wrap(err, "failed to execute HTTP request")
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return errors.Errorf("delete key failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetApplicationKey retrieves an application key by ID from Backblaze B2
func (c *BackblazeClient) GetApplicationKey(ctx context.Context, applicationKeyID string) (*B2CreateKeyResponse, error) {
	if err := c.authorizeAccount(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to authorize account")
	}

	req := B2ListKeysRequest{
		AccountID:   c.AccountID,
		MaxKeyCount: 100, // We'll search through keys
	}

	for {
		reqBody, err := json.Marshal(req)
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshal list keys request")
		}

		httpReq, err := http.NewRequestWithContext(ctx, "POST", c.APIURL+"/b2api/v3/b2_list_keys", bytes.NewBuffer(reqBody))
		if err != nil {
			return nil, errors.Wrap(err, "failed to create HTTP request")
		}

		httpReq.Header.Set("Authorization", c.AuthToken)
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := c.doWithReauth(ctx, httpReq)
		if err != nil {
			return nil, errors.Wrap(err, "failed to execute HTTP request")
		}
		defer func() {
			_ = resp.Body.Close()
		}()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, errors.Errorf("list keys failed with status %d: %s", resp.StatusCode, string(body))
		}

		var listResp B2ListKeysResponse
		if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
			return nil, errors.Wrap(err, "failed to decode list keys response")
		}

		// Search for the key in the current batch
		for _, key := range listResp.Keys {
			if key.ApplicationKeyID == applicationKeyID {
				return &B2CreateKeyResponse{
					ApplicationKeyID:    key.ApplicationKeyID,
					ApplicationKey:      "", // Not returned in list operations for security
					KeyName:             key.KeyName,
					Capabilities:        key.Capabilities,
					AccountID:           key.AccountID,
					ExpirationTimestamp: key.ExpirationTimestamp,
					BucketID:            key.BucketID,
					NamePrefix:          key.NamePrefix,
				}, nil
			}
		}

		// If there are more keys to check, continue
		if listResp.NextApplicationKeyID == "" {
			break
		}
		req.StartApplicationKeyID = listResp.NextApplicationKeyID
	}

	return nil, ErrApplicationKeyNotFound
}

// UpdateApplicationKey edits the live application key via b2_update_key.
// Only fields that are non-zero are sent; the application's secret value is
// never touched, so existing connection secrets in the cluster remain valid.
func (c *BackblazeClient) UpdateApplicationKey(ctx context.Context, req B2UpdateKeyRequest) (*B2CreateKeyResponse, error) {
	if req.ApplicationKeyID == "" {
		return nil, errors.New("applicationKeyId is required")
	}
	if err := c.authorizeAccount(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to authorize account")
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal b2_update_key request")
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.APIURL+"/b2api/v3/b2_update_key", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create HTTP request")
	}
	httpReq.Header.Set("Authorization", c.AuthToken)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.doWithReauth(ctx, httpReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, errors.Errorf("b2_update_key failed with status %d: %s", resp.StatusCode, string(body))
	}

	var out B2CreateKeyResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, errors.Wrap(err, "failed to decode b2_update_key response")
	}
	return &out, nil
}

// S3 Bucket Policy Methods

// ErrBucketPolicyNotFound is returned when no bucket policy is configured for the named bucket.
var ErrBucketPolicyNotFound = errors.New("bucket policy not found")

// ErrApplicationKeyNotFound is returned when no application key with the
// given ID exists on the account.
var ErrApplicationKeyNotFound = errors.New("application key not found")

// GetBucketPolicy retrieves the policy for a bucket
func (c *BackblazeClient) GetBucketPolicy(ctx context.Context, bucketName string) (string, error) {
	input := &s3.GetBucketPolicyInput{
		Bucket: aws.String(bucketName),
	}

	result, err := c.S3Client.GetBucketPolicy(ctx, input)
	if err != nil {
		if isS3NotFound(err) || err.Error() == "NoSuchBucketPolicy" {
			return "", ErrBucketPolicyNotFound
		}
		return "", errors.Wrap(err, "failed to get bucket policy")
	}

	if result.Policy == nil {
		return "", ErrBucketPolicyNotFound
	}

	return *result.Policy, nil
}

// PutBucketPolicy applies a policy to a bucket
func (c *BackblazeClient) PutBucketPolicy(ctx context.Context, bucketName, policy string) error {
	input := &s3.PutBucketPolicyInput{
		Bucket: aws.String(bucketName),
		Policy: aws.String(policy),
	}

	_, err := c.S3Client.PutBucketPolicy(ctx, input)
	if err != nil {
		return errors.Wrap(err, "failed to put bucket policy")
	}

	return nil
}

// DeleteBucketPolicy removes the policy from a bucket
func (c *BackblazeClient) DeleteBucketPolicy(ctx context.Context, bucketName string) error {
	input := &s3.DeleteBucketPolicyInput{
		Bucket: aws.String(bucketName),
	}

	_, err := c.S3Client.DeleteBucketPolicy(ctx, input)
	if err != nil {
		if isS3NotFound(err) || err.Error() == "NoSuchBucketPolicy" {
			return ErrBucketPolicyNotFound
		}
		return errors.Wrap(err, "failed to delete bucket policy")
	}

	return nil
}

// Bucket Lifecycle Configuration (Backblaze S3-compatible subset)

// GetBucketLifecycleConfiguration reads the bucket's lifecycle configuration.
// Returns ErrBucketLifecycleNotFound if no lifecycle rules are configured.
func (c *BackblazeClient) GetBucketLifecycleConfiguration(ctx context.Context, bucketName string) ([]types.LifecycleRule, error) {
	input := &s3.GetBucketLifecycleConfigurationInput{
		Bucket: aws.String(bucketName),
	}

	result, err := c.S3Client.GetBucketLifecycleConfiguration(ctx, input)
	if err != nil {
		if isS3NotFound(err) || err.Error() == "NoSuchLifecycleConfiguration" {
			return nil, ErrBucketLifecycleNotFound
		}
		return nil, errors.Wrap(err, "failed to get bucket lifecycle configuration")
	}
	return result.Rules, nil
}

// PutBucketLifecycleConfiguration replaces the bucket's lifecycle configuration.
func (c *BackblazeClient) PutBucketLifecycleConfiguration(ctx context.Context, bucketName string, rules []types.LifecycleRule) error {
	input := &s3.PutBucketLifecycleConfigurationInput{
		Bucket:                 aws.String(bucketName),
		LifecycleConfiguration: &types.BucketLifecycleConfiguration{Rules: rules},
	}
	if _, err := c.S3Client.PutBucketLifecycleConfiguration(ctx, input); err != nil {
		return errors.Wrap(err, "failed to put bucket lifecycle configuration")
	}
	return nil
}

// DeleteBucketLifecycleConfiguration removes all lifecycle rules from the bucket.
func (c *BackblazeClient) DeleteBucketLifecycleConfiguration(ctx context.Context, bucketName string) error {
	input := &s3.DeleteBucketLifecycleInput{
		Bucket: aws.String(bucketName),
	}
	if _, err := c.S3Client.DeleteBucketLifecycle(ctx, input); err != nil {
		if isS3NotFound(err) || err.Error() == "NoSuchLifecycleConfiguration" {
			return ErrBucketLifecycleNotFound
		}
		return errors.Wrap(err, "failed to delete bucket lifecycle configuration")
	}
	return nil
}

// ErrBucketLifecycleNotFound is returned when no lifecycle configuration exists for the bucket.
var ErrBucketLifecycleNotFound = errors.New("bucket lifecycle configuration not found")

// Bucket CORS Configuration (Backblaze S3-compatible)

// GetBucketCors reads the bucket's CORS configuration. Returns ErrBucketCorsNotFound
// if no CORS rules are configured.
func (c *BackblazeClient) GetBucketCors(ctx context.Context, bucketName string) ([]types.CORSRule, error) {
	input := &s3.GetBucketCorsInput{
		Bucket: aws.String(bucketName),
	}
	result, err := c.S3Client.GetBucketCors(ctx, input)
	if err != nil {
		if isS3NotFound(err) || err.Error() == "NoSuchCORSConfiguration" {
			return nil, ErrBucketCorsNotFound
		}
		return nil, errors.Wrap(err, "failed to get bucket cors configuration")
	}
	return result.CORSRules, nil
}

// PutBucketCors replaces the bucket's CORS configuration.
func (c *BackblazeClient) PutBucketCors(ctx context.Context, bucketName string, rules []types.CORSRule) error {
	input := &s3.PutBucketCorsInput{
		Bucket:            aws.String(bucketName),
		CORSConfiguration: &types.CORSConfiguration{CORSRules: rules},
	}
	if _, err := c.S3Client.PutBucketCors(ctx, input); err != nil {
		return errors.Wrap(err, "failed to put bucket cors configuration")
	}
	return nil
}

// DeleteBucketCors removes the CORS configuration from the bucket.
func (c *BackblazeClient) DeleteBucketCors(ctx context.Context, bucketName string) error {
	input := &s3.DeleteBucketCorsInput{
		Bucket: aws.String(bucketName),
	}
	if _, err := c.S3Client.DeleteBucketCors(ctx, input); err != nil {
		if isS3NotFound(err) || err.Error() == "NoSuchCORSConfiguration" {
			return ErrBucketCorsNotFound
		}
		return errors.Wrap(err, "failed to delete bucket cors configuration")
	}
	return nil
}

// ErrBucketCorsNotFound is returned when no CORS configuration exists for the bucket.
var ErrBucketCorsNotFound = errors.New("bucket cors configuration not found")

// B2 native bucket management (used for BucketType and bucket metadata)

// B2UpdateBucketRequest represents the request to update an existing bucket.
// Only fields that should be changed need to be set.
type B2UpdateBucketRequest struct {
	AccountID                   string                  `json:"accountId"`
	BucketID                    string                  `json:"bucketId"`
	BucketType                  string                  `json:"bucketType,omitempty"`
	BucketInfo                  *B2BucketInfo           `json:"bucketInfo,omitempty"`
	LifecycleRules              []B2NativeLifecycleRule `json:"lifecycleRules,omitempty"`
	CORSRules                   []B2NativeCORSRule      `json:"corsRules,omitempty"`
	DefaultRetention            *B2FileLockRetention    `json:"defaultRetention,omitempty"`
	DefaultServerSideEncryption *B2NativeSSESettings    `json:"defaultServerSideEncryption,omitempty"`
	FileLockEnabled             *bool                   `json:"fileLockEnabled,omitempty"`
}

// B2BucketInfo is a map of bucket-level metadata returned by B2.
type B2BucketInfo map[string]string

// B2NativeLifecycleRule is the JSON shape B2 expects for lifecycle rules.
type B2NativeLifecycleRule struct {
	FileNamePrefix            string `json:"fileNamePrefix,omitempty"`
	DaysFromUploadingToHiding *int   `json:"daysFromUploadingToHiding,omitempty"`
	DaysFromHidingToDeleting  *int   `json:"daysFromHidingToDeleting,omitempty"`
}

// B2NativeCORSRule is the JSON shape B2 expects for CORS rules.
type B2NativeCORSRule struct {
	CorsRuleName   string   `json:"corsRuleName"`
	AllowedOrigins []string `json:"allowedOrigins"`
	AllowedHeaders []string `json:"allowedHeaders,omitempty"`
	AllowedMethods []string `json:"allowedMethods"`
	ExposeHeaders  []string `json:"exposeHeaders,omitempty"`
	MaxAgeSeconds  *int     `json:"maxAgeSeconds,omitempty"`
}

// B2FileLockRetention corresponds to b2 file lock retention settings.
type B2FileLockRetention struct {
	Mode   string `json:"mode"`
	Period int    `json:"period"`
}

// B2NativeSSESettings is the B2 server-side encryption default.
type B2NativeSSESettings struct {
	Mode      string `json:"mode"`      // "SSE-B2" or "SSE-C"
	Algorithm string `json:"algorithm"` // "AES256"
}

// B2UpdateBucketRequest represents the request to update an existing bucket.
// Only fields that should be changed need to be set.

// B2UpdateBucketResponse is the response shape for b2_update_bucket.
type B2UpdateBucketResponse struct {
	AccountID                   string                  `json:"accountId"`
	BucketID                    string                  `json:"bucketId"`
	BucketName                  string                  `json:"bucketName"`
	BucketType                  string                  `json:"bucketType"`
	BucketInfo                  B2BucketInfo            `json:"bucketInfo"`
	LifecycleRules              []B2NativeLifecycleRule `json:"lifecycleRules"`
	CORSRules                   []B2NativeCORSRule      `json:"corsRules"`
	DefaultRetention            *B2FileLockRetention    `json:"defaultRetention,omitempty"`
	DefaultServerSideEncryption *B2NativeSSESettings    `json:"defaultServerSideEncryption,omitempty"`
	FileLockEnabled             *bool                   `json:"fileLockEnabled,omitempty"`
}

// B2ListBucketsResponse is the shape returned by b2_list_buckets.
type B2ListBucketsResponse struct {
	Buckets []struct {
		AccountID                   string                  `json:"accountId"`
		BucketID                    string                  `json:"bucketId"`
		BucketName                  string                  `json:"bucketName"`
		BucketType                  string                  `json:"bucketType"`
		BucketInfo                  B2BucketInfo            `json:"bucketInfo"`
		LifecycleRules              []B2NativeLifecycleRule `json:"lifecycleRules"`
		CORSRules                   []B2NativeCORSRule      `json:"corsRules"`
		DefaultRetention            *B2FileLockRetention    `json:"defaultRetention,omitempty"`
		DefaultServerSideEncryption *B2NativeSSESettings    `json:"defaultServerSideEncryption,omitempty"`
		FileLockEnabled             *bool                   `json:"fileLockEnabled,omitempty"`
	} `json:"buckets"`
}

// B2UpdateBucket updates an existing bucket. Currently used to set BucketType
// (e.g. to flip a private bucket to allPublic) which cannot be done at
// create time via the S3-compatible API.
func (c *BackblazeClient) B2UpdateBucket(ctx context.Context, req B2UpdateBucketRequest) (*B2UpdateBucketResponse, error) {
	if err := c.authorizeAccount(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to authorize account")
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal b2_update_bucket request")
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.APIURL+"/b2api/v3/b2_update_bucket", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create HTTP request")
	}
	httpReq.Header.Set("Authorization", c.AuthToken)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.doWithReauth(ctx, httpReq)
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute HTTP request")
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, errors.Errorf("b2_update_bucket failed with status %d: %s", resp.StatusCode, string(body))
	}

	var out B2UpdateBucketResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, errors.Wrap(err, "failed to decode b2_update_bucket response")
	}
	return &out, nil
}

// B2ListBuckets calls b2_list_buckets and returns the native B2 bucket view
// (incl. ID, type, lifecycle, CORS). Used for observation/drift detection
// where the S3 path doesn't give us enough info.
func (c *BackblazeClient) B2ListBuckets(ctx context.Context) (*B2ListBucketsResponse, error) {
	if err := c.authorizeAccount(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to authorize account")
	}

	reqBody, err := json.Marshal(struct {
		AccountID string `json:"accountId"`
	}{AccountID: c.AccountID})
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal b2_list_buckets request")
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.APIURL+"/b2api/v3/b2_list_buckets", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create HTTP request")
	}
	httpReq.Header.Set("Authorization", c.AuthToken)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.doWithReauth(ctx, httpReq)
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute HTTP request")
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, errors.Errorf("b2_list_buckets failed with status %d: %s", resp.StatusCode, string(body))
	}

	var out B2ListBucketsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, errors.Wrap(err, "failed to decode b2_list_buckets response")
	}
	return &out, nil
}

// B2CreateBucketRequest is the request body for b2_create_bucket.
type B2CreateBucketRequest struct {
	AccountID  string        `json:"accountId"`
	BucketName string        `json:"bucketName"`
	BucketType string        `json:"bucketType"`
	BucketInfo *B2BucketInfo `json:"bucketInfo,omitempty"`
}

// B2CreateBucketResponse is the response body for b2_create_bucket. Includes
// the canonical BucketID we need to make subsequent updates.
type B2CreateBucketResponse struct {
	AccountID  string       `json:"accountId"`
	BucketID   string       `json:"bucketId"`
	BucketName string       `json:"bucketName"`
	BucketType string       `json:"bucketType"`
	BucketInfo B2BucketInfo `json:"bucketInfo"`
}

// B2CreateBucket creates a bucket via the native API (so we get the
// BucketID back, unlike the S3_CreateBucket form).
func (c *BackblazeClient) B2CreateBucket(ctx context.Context, req B2CreateBucketRequest) (*B2CreateBucketResponse, error) {
	if err := c.authorizeAccount(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to authorize account")
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal b2_create_bucket request")
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.APIURL+"/b2api/v3/b2_create_bucket", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create HTTP request")
	}
	httpReq.Header.Set("Authorization", c.AuthToken)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.doWithReauth(ctx, httpReq)
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute HTTP request")
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, errors.Errorf("b2_create_bucket failed with status %d: %s", resp.StatusCode, string(body))
	}

	var out B2CreateBucketResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, errors.Wrap(err, "failed to decode b2_create_bucket response")
	}
	return &out, nil
}

// Bucket event notification methods (b2_create / list / delete event notification).

// B2EventNotification is the B2 notification rule representation.
type B2EventNotification struct {
	AccountID      string   `json:"accountId"`
	BucketID       string   `json:"bucketId"`
	NotificationID string   `json:"notificationId,omitempty"`
	Name           string   `json:"name"`
	Events         []string `json:"events"`
	WebhookURL     string   `json:"webhookUrl"`
	Description    string   `json:"description,omitempty"`
	Disabled       bool     `json:"disabled,omitempty"`
}

// B2CreateEventNotificationRequest is the request body for
// b2_create_event_notification.
type B2CreateEventNotificationRequest struct {
	AccountID   string   `json:"accountId"`
	BucketID    string   `json:"bucketId"`
	Name        string   `json:"name"`
	Events      []string `json:"events"`
	WebhookURL  string   `json:"webhookUrl"`
	Description string   `json:"description,omitempty"`
	Disabled    bool     `json:"disabled,omitempty"`
}

// B2ListEventNotificationsRequest is the request body for
// b2_list_event_notifications.
type B2ListEventNotificationsRequest struct {
	AccountID string `json:"accountId"`
	BucketID  string `json:"bucketId"`
}

// B2ListEventNotificationsResponse is the response body.
type B2ListEventNotificationsResponse struct {
	Notifications []B2EventNotification `json:"notifications"`
}

// B2DeleteEventNotificationRequest is the request body for
// b2_delete_event_notification.
type B2DeleteEventNotificationRequest struct {
	AccountID      string `json:"accountId"`
	BucketID       string `json:"bucketId"`
	NotificationID string `json:"notificationId"`
}

// B2UpdateEventNotificationRequest is the request body for
// b2_update_event_notification. Mutates an existing rule in place.
type B2UpdateEventNotificationRequest struct {
	AccountID      string   `json:"accountId"`
	BucketID       string   `json:"bucketId"`
	NotificationID string   `json:"notificationId"`
	Name           string   `json:"name"`
	Events         []string `json:"events"`
	WebhookURL     string   `json:"webhookUrl"`
	Description    string   `json:"description,omitempty"`
	Disabled       bool     `json:"disabled,omitempty"`
}

func (c *BackblazeClient) B2CreateEventNotification(ctx context.Context, req B2CreateEventNotificationRequest) (*B2EventNotification, error) {
	if err := c.authorizeAccount(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to authorize account")
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal b2_create_event_notification request")
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.APIURL+"/b2api/v3/b2_create_event_notification", bytes.NewBuffer(body))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create HTTP request")
	}
	httpReq.Header.Set("Authorization", c.AuthToken)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.doWithReauth(ctx, httpReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, errors.Errorf("b2_create_event_notification failed: %d %s", resp.StatusCode, string(respBody))
	}
	var out B2EventNotification
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, errors.Wrap(err, "failed to decode b2_create_event_notification response")
	}
	return &out, nil
}

func (c *BackblazeClient) B2ListEventNotifications(ctx context.Context, req B2ListEventNotificationsRequest) (*B2ListEventNotificationsResponse, error) {
	if err := c.authorizeAccount(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to authorize account")
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal b2_list_event_notifications request")
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.APIURL+"/b2api/v3/b2_list_event_notifications", bytes.NewBuffer(body))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create HTTP request")
	}
	httpReq.Header.Set("Authorization", c.AuthToken)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.doWithReauth(ctx, httpReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, errors.Errorf("b2_list_event_notifications failed: %d %s", resp.StatusCode, string(respBody))
	}
	var out B2ListEventNotificationsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, errors.Wrap(err, "failed to decode b2_list_event_notifications response")
	}
	return &out, nil
}

func (c *BackblazeClient) B2UpdateEventNotification(ctx context.Context, req B2UpdateEventNotificationRequest) (*B2EventNotification, error) {
	if err := c.authorizeAccount(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to authorize account")
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal b2_update_event_notification request")
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.APIURL+"/b2api/v3/b2_update_event_notification", bytes.NewBuffer(body))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create HTTP request")
	}
	httpReq.Header.Set("Authorization", c.AuthToken)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.doWithReauth(ctx, httpReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, errors.Errorf("b2_update_event_notification failed: %d %s", resp.StatusCode, string(respBody))
	}
	var out B2EventNotification
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, errors.Wrap(err, "failed to decode b2_update_event_notification response")
	}
	return &out, nil
}

func (c *BackblazeClient) B2DeleteEventNotification(ctx context.Context, req B2DeleteEventNotificationRequest) error {
	if err := c.authorizeAccount(ctx); err != nil {
		return errors.Wrap(err, "failed to authorize account")
	}
	body, err := json.Marshal(req)
	if err != nil {
		return errors.Wrap(err, "failed to marshal b2_delete_event_notification request")
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.APIURL+"/b2api/v3/b2_delete_event_notification", bytes.NewBuffer(body))
	if err != nil {
		return errors.Wrap(err, "failed to create HTTP request")
	}
	httpReq.Header.Set("Authorization", c.AuthToken)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.doWithReauth(ctx, httpReq)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return errors.Errorf("b2_delete_event_notification failed: %d %s", resp.StatusCode, string(respBody))
	}
	return nil
}
