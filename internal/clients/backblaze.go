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
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"

	"github.com/rossigee/provider-backblaze/apis/v1beta1"
)

const (
	// SecretKeyApplicationKeyID is the key in the secret containing the Backblaze application key ID
	SecretKeyApplicationKeyID = "applicationKeyId"
	// SecretKeyApplicationKey is the key in the secret containing the Backblaze application key
	SecretKeyApplicationKey = "applicationKey"

	// Default Backblaze B2 regions and their S3-compatible endpoints
	DefaultRegion         = "us-west-001"
	DefaultEndpointFormat = "https://s3.%s.backblazeb2.com"

	// Backblaze B2 Native API constants
	B2AuthorizeAccountURL = "https://api.backblazeb2.com/b2api/v3/b2_authorize_account"
	B2CreateKeyURL        = "https://api.backblazeb2.com/b2api/v3/b2_create_key"
	B2DeleteKeyURL        = "https://api.backblazeb2.com/b2api/v3/b2_delete_key"
	B2ListKeysURL         = "https://api.backblazeb2.com/b2api/v3/b2_list_keys"
)

// BackblazeClient represents a client for Backblaze B2 using S3-compatible API and native B2 API
type BackblazeClient struct {
	S3Client *s3.S3
	Region   string
	Endpoint string

	// B2 Native API support
	HTTPClient        *http.Client
	ApplicationKeyID  string
	ApplicationKey    string
	AuthToken         string
	APIURL            string
	DownloadURL       string
	AccountID         string
	tokenExpiration   time.Time
}

// Config contains configuration for connecting to Backblaze B2
type Config struct {
	ApplicationKeyID string
	ApplicationKey   string
	Region           string
}

// NewBackblazeClient creates a new Backblaze B2 client using S3-compatible API
func NewBackblazeClient(cfg Config) (*BackblazeClient, error) {
	if cfg.ApplicationKeyID == "" || cfg.ApplicationKey == "" {
		return nil, errors.New("applicationKeyId and applicationKey are required")
	}

	if cfg.Region == "" {
		cfg.Region = DefaultRegion
	}

	endpoint := fmt.Sprintf(DefaultEndpointFormat, cfg.Region)

	awsConfig := &aws.Config{
		Credentials: credentials.NewStaticCredentials(
			cfg.ApplicationKeyID,
			cfg.ApplicationKey,
			"", // token not needed for Backblaze B2
		),
		Region:           aws.String(cfg.Region),
		Endpoint:         aws.String(endpoint),
		S3ForcePathStyle: aws.Bool(true), // Required for Backblaze B2
	}

	sess, err := session.NewSession(awsConfig)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create AWS session")
	}

	return &BackblazeClient{
		S3Client:         s3.New(sess),
		Region:           cfg.Region,
		Endpoint:         endpoint,
		HTTPClient:       &http.Client{Timeout: 30 * time.Second},
		ApplicationKeyID: cfg.ApplicationKeyID,
		ApplicationKey:   cfg.ApplicationKey,
	}, nil
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
		input.CreateBucketConfiguration = &s3.CreateBucketConfiguration{
			LocationConstraint: aws.String(region),
		}
	}

	_, err := c.S3Client.CreateBucketWithContext(ctx, input)
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

	_, err := c.S3Client.DeleteBucketWithContext(ctx, input)
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

	_, err := c.S3Client.HeadBucketWithContext(ctx, input)
	if err != nil {
		if isNotFoundError(err) {
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

	result, err := c.S3Client.GetBucketLocationWithContext(ctx, input)
	if err != nil {
		return "", errors.Wrap(err, "failed to get bucket location")
	}

	location := ""
	if result.LocationConstraint != nil {
		location = *result.LocationConstraint
	}

	return location, nil
}

// ListBuckets lists all buckets accessible with the current credentials
func (c *BackblazeClient) ListBuckets(ctx context.Context) ([]*s3.Bucket, error) {
	result, err := c.S3Client.ListBucketsWithContext(ctx, &s3.ListBucketsInput{})
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
		result, err := c.S3Client.ListObjectsV2WithContext(ctx, listInput)
		if err != nil {
			return errors.Wrap(err, "failed to list objects")
		}

		if len(result.Contents) == 0 {
			break
		}

		// Delete objects in batch
		objects := make([]*s3.ObjectIdentifier, len(result.Contents))
		for i, obj := range result.Contents {
			objects[i] = &s3.ObjectIdentifier{
				Key: obj.Key,
			}
		}

		deleteInput := &s3.DeleteObjectsInput{
			Bucket: aws.String(bucketName),
			Delete: &s3.Delete{
				Objects: objects,
			},
		}

		_, err = c.S3Client.DeleteObjectsWithContext(ctx, deleteInput)
		if err != nil {
			return errors.Wrap(err, "failed to delete objects")
		}

		// Check if there are more objects to delete
		if !aws.BoolValue(result.IsTruncated) {
			break
		}
		listInput.ContinuationToken = result.NextContinuationToken
	}

	return nil
}

// isNotFoundError checks if an error is a "not found" error
func isNotFoundError(err error) bool {
	// This is a simplified check - in production, you'd want more robust error checking
	return err != nil && (err.Error() == "NotFound" ||
		err.Error() == "NoSuchBucket")
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
	ApplicationKeyID string `json:"applicationKeyId"`
	ApplicationKey   string `json:"applicationKey"`
}

// B2AuthorizeAccountResponse represents the response from authorize account
type B2AuthorizeAccountResponse struct {
	AccountID          string `json:"accountId"`
	AuthorizationToken string `json:"authorizationToken"`
	APIURL             string `json:"apiUrl"`
	DownloadURL        string `json:"downloadUrl"`
}

// B2CreateKeyRequest represents the request to create an application key
type B2CreateKeyRequest struct {
	AccountID               string   `json:"accountId"`
	Capabilities            []string `json:"capabilities"`
	KeyName                 string   `json:"keyName"`
	ValidDurationInSeconds  *int     `json:"validDurationInSeconds,omitempty"`
	BucketID                string   `json:"bucketId,omitempty"`
	NamePrefix              string   `json:"namePrefix,omitempty"`
}

// B2CreateKeyResponse represents the response from create key
type B2CreateKeyResponse struct {
	ApplicationKeyID        string    `json:"applicationKeyId"`
	ApplicationKey          string    `json:"applicationKey"`
	KeyName                 string    `json:"keyName"`
	Capabilities            []string  `json:"capabilities"`
	AccountID               string    `json:"accountId"`
	ExpirationTimestamp     *int64    `json:"expirationTimestamp,omitempty"`
	BucketID                string    `json:"bucketId,omitempty"`
	NamePrefix              string    `json:"namePrefix,omitempty"`
}

// B2DeleteKeyRequest represents the request to delete an application key
type B2DeleteKeyRequest struct {
	ApplicationKeyID string `json:"applicationKeyId"`
}

// B2ListKeysRequest represents the request to list application keys
type B2ListKeysRequest struct {
	AccountID  string `json:"accountId"`
	MaxKeyCount int   `json:"maxKeyCount,omitempty"`
	StartApplicationKeyID string `json:"startApplicationKeyId,omitempty"`
}

// B2ListKeysResponse represents the response from list keys
type B2ListKeysResponse struct {
	Keys []struct {
		ApplicationKeyID        string   `json:"applicationKeyId"`
		KeyName                 string   `json:"keyName"`
		Capabilities            []string `json:"capabilities"`
		AccountID               string   `json:"accountId"`
		ExpirationTimestamp     *int64   `json:"expirationTimestamp,omitempty"`
		BucketID                string   `json:"bucketId,omitempty"`
		NamePrefix              string   `json:"namePrefix,omitempty"`
	} `json:"keys"`
	NextApplicationKeyID string `json:"nextApplicationKeyId,omitempty"`
}

// B2 API Methods

// authorizeAccount authorizes with B2 API and gets account info
func (c *BackblazeClient) authorizeAccount(ctx context.Context) error {
	// Check if we already have a valid token
	if c.AuthToken != "" && time.Now().Before(c.tokenExpiration) {
		return nil
	}

	req := B2AuthorizeAccountRequest{
		ApplicationKeyID: c.ApplicationKeyID,
		ApplicationKey:   c.ApplicationKey,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return errors.Wrap(err, "failed to marshal authorize request")
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", B2AuthorizeAccountURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return errors.Wrap(err, "failed to create HTTP request")
	}

	httpReq.Header.Set("Content-Type", "application/json")

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
	c.APIURL = authResp.APIURL
	c.DownloadURL = authResp.DownloadURL
	c.AccountID = authResp.AccountID
	// B2 tokens typically last 24 hours, but we'll refresh after 12 hours to be safe
	c.tokenExpiration = time.Now().Add(12 * time.Hour)

	return nil
}

// CreateApplicationKey creates a new application key in Backblaze B2
func (c *BackblazeClient) CreateApplicationKey(ctx context.Context, keyName string, capabilities []string, bucketID, namePrefix string, validDurationInSeconds *int) (*B2CreateKeyResponse, error) {
	if err := c.authorizeAccount(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to authorize account")
	}

	req := B2CreateKeyRequest{
		AccountID:               c.AccountID,
		KeyName:                 keyName,
		Capabilities:            capabilities,
		ValidDurationInSeconds:  validDurationInSeconds,
		BucketID:                bucketID,
		NamePrefix:              namePrefix,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal create key request")
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", B2CreateKeyURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create HTTP request")
	}

	httpReq.Header.Set("Authorization", c.AuthToken)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(httpReq)
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

	httpReq, err := http.NewRequestWithContext(ctx, "POST", B2DeleteKeyURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return errors.Wrap(err, "failed to create HTTP request")
	}

	httpReq.Header.Set("Authorization", c.AuthToken)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(httpReq)
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

		httpReq, err := http.NewRequestWithContext(ctx, "POST", B2ListKeysURL, bytes.NewBuffer(reqBody))
		if err != nil {
			return nil, errors.Wrap(err, "failed to create HTTP request")
		}

		httpReq.Header.Set("Authorization", c.AuthToken)
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := c.HTTPClient.Do(httpReq)
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

	return nil, errors.New("application key not found")
}

// S3 Bucket Policy Methods

// GetBucketPolicy retrieves the policy for a bucket
func (c *BackblazeClient) GetBucketPolicy(ctx context.Context, bucketName string) (string, error) {
	input := &s3.GetBucketPolicyInput{
		Bucket: aws.String(bucketName),
	}

	result, err := c.S3Client.GetBucketPolicyWithContext(ctx, input)
	if err != nil {
		if isNotFoundError(err) || err.Error() == "NoSuchBucketPolicy" {
			return "", errors.New("bucket policy not found")
		}
		return "", errors.Wrap(err, "failed to get bucket policy")
	}

	if result.Policy == nil {
		return "", errors.New("bucket policy not found")
	}

	return *result.Policy, nil
}

// PutBucketPolicy applies a policy to a bucket
func (c *BackblazeClient) PutBucketPolicy(ctx context.Context, bucketName, policy string) error {
	input := &s3.PutBucketPolicyInput{
		Bucket: aws.String(bucketName),
		Policy: aws.String(policy),
	}

	_, err := c.S3Client.PutBucketPolicyWithContext(ctx, input)
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

	_, err := c.S3Client.DeleteBucketPolicyWithContext(ctx, input)
	if err != nil {
		if isNotFoundError(err) || err.Error() == "NoSuchBucketPolicy" {
			return errors.New("bucket policy not found")
		}
		return errors.Wrap(err, "failed to delete bucket policy")
	}

	return nil
}
