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
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/pkg/resource"

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
)

// BackblazeClient represents a client for Backblaze B2 using S3-compatible API
type BackblazeClient struct {
	S3Client *s3.S3
	Region   string
	Endpoint string
}

// Config contains configuration for connecting to Backblaze B2
type Config struct {
	ApplicationKeyID string
	ApplicationKey   string
	Region           string
	EndpointURL      string
}

// NewBackblazeClient creates a new Backblaze B2 client using S3-compatible API
func NewBackblazeClient(cfg Config) (*BackblazeClient, error) {
	if cfg.ApplicationKeyID == "" || cfg.ApplicationKey == "" {
		return nil, errors.New("applicationKeyId and applicationKey are required")
	}

	if cfg.Region == "" {
		cfg.Region = DefaultRegion
	}

	endpoint := cfg.EndpointURL
	if endpoint == "" {
		endpoint = fmt.Sprintf(DefaultEndpointFormat, cfg.Region)
	}

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
		S3Client: s3.New(sess),
		Region:   cfg.Region,
		Endpoint: endpoint,
	}, nil
}

// GetProviderConfig extracts Backblaze configuration from a ProviderConfig
func GetProviderConfig(ctx context.Context, c client.Client, pc *v1beta1.ProviderConfig) (*Config, error) {
	cfg := &Config{
		Region:      pc.Spec.BackblazeRegion,
		EndpointURL: pc.Spec.EndpointURL,
	}

	switch pc.Spec.Credentials.Source {
	case "Secret":
		if pc.Spec.Credentials.APISecretRef.Name == "" {
			return nil, errors.New("apiSecretRef.name is required when source is Secret")
		}

		secret := &corev1.Secret{}
		if err := c.Get(ctx, client.ObjectKey{
			Namespace: pc.Spec.Credentials.APISecretRef.Namespace,
			Name:      pc.Spec.Credentials.APISecretRef.Name,
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
