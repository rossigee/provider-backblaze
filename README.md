# Provider Backblaze

`provider-backblaze` is a [Crossplane](https://crossplane.io/) provider that enables infrastructure management for [Backblaze B2](https://www.backblaze.com/b2/cloud-storage.html) cloud storage.

## Overview

This provider allows you to declaratively manage Backblaze B2 resources through Kubernetes custom resources. It uses Backblaze B2's S3-compatible API to provide seamless integration with existing S3 tooling while accessing Backblaze's cost-effective cloud storage.

## Features

- **S3-Compatible**: Uses Backblaze B2's S3-compatible API for maximum compatibility
- **Cost-Effective**: Leverage Backblaze's competitive pricing for cloud storage
- **Declarative Management**: Manage resources through Kubernetes YAML manifests
- **Lifecycle Management**: Automatic file lifecycle rules and bucket management
- **Security**: Fine-grained application keys with specific permissions and restrictions

## Supported Resources

### Bucket (`bucket.backblaze.crossplane.io/v1`)

Manage Backblaze B2 storage buckets with:
- Public or private access levels
- Lifecycle rules for automatic file management
- CORS configuration for web applications
- Flexible deletion policies

### User (`user.backblaze.crossplane.io/v1`)

Create and manage Backblaze B2 application keys with:
- Fine-grained capabilities (read, write, delete, etc.)
- Bucket-specific restrictions
- File prefix restrictions
- Automatic secret generation for application integration

### Policy (`policy.backblaze.crossplane.io/v1`)

Manage S3-compatible access policies for:
- Simple bucket-level permissions
- Complex JSON-based policies
- Integration with existing S3 policy tools

## Quick Start

### 1. Install the Provider

```bash
kubectl apply -f - <<EOF
apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: provider-backblaze
spec:
  package: ghcr.io/rossigee/provider-backblaze:latest
EOF
```

### 2. Create Backblaze B2 Credentials

First, create application keys in your Backblaze B2 console:

1. Go to [Backblaze B2 Console](https://secure.backblaze.com/b2_buckets.htm)
2. Navigate to "App Keys" → "Add a New Application Key"
3. Choose capabilities and restrictions as needed
4. Note the **Application Key ID** and **Application Key**

### 3. Configure Provider Credentials

```bash
# Create the credentials secret
kubectl create secret generic backblaze-creds \
  --namespace crossplane-system \
  --from-literal=applicationKeyId="your-key-id" \
  --from-literal=applicationKey="your-application-key"

# Apply the provider configuration
kubectl apply -f examples/providerconfig.yaml
```

### 4. Create Your First Bucket

```bash
kubectl apply -f examples/bucket.yaml
```

## Examples

### Basic Private Bucket

```yaml
apiVersion: bucket.backblaze.crossplane.io/v1
kind: Bucket
metadata:
  name: my-storage
spec:
  forProvider:
    bucketName: my-unique-bucket-name
    region: us-west-001
    bucketType: allPrivate
    bucketDeletionPolicy: DeleteIfEmpty
  providerConfigRef:
    name: default
```

### Application Key with Restricted Access

```yaml
apiVersion: user.backblaze.crossplane.io/v1
kind: User
metadata:
  name: read-only-key
spec:
  forProvider:
    keyName: "read-only-application-key"
    capabilities:
    - "listFiles"
    - "readFiles"
    bucketID: "your-bucket-id"
    writeSecretToRef:
      name: read-only-credentials
      namespace: default
  providerConfigRef:
    name: default
```

### Bucket with Lifecycle Rules

```yaml
apiVersion: bucket.backblaze.crossplane.io/v1
kind: Bucket
metadata:
  name: auto-cleanup-bucket
spec:
  forProvider:
    bucketName: my-lifecycle-bucket
    region: us-west-001
    lifecycleRules:
    - fileNamePrefix: "logs/"
      daysFromUploadingToHiding: 30
      daysFromHidingToDeleting: 90
    - fileNamePrefix: "temp/"
      daysFromUploadingToHiding: 1
      daysFromHidingToDeleting: 7
  providerConfigRef:
    name: default
```

## Configuration

### Provider Configuration

The `ProviderConfig` configures authentication and region settings:

```yaml
apiVersion: backblaze.crossplane.io/v1beta1
kind: ProviderConfig
metadata:
  name: default
spec:
  backblazeRegion: us-west-001  # Required: B2 region
  endpointURL: ""               # Optional: custom endpoint
  credentials:
    source: Secret
    apiSecretRef:
      namespace: crossplane-system
      name: backblaze-creds
```

### Supported Regions

Common Backblaze B2 regions:
- `us-west-001` (US West - Oregon)
- `us-west-002` (US West - California)  
- `eu-central-003` (EU - Amsterdam)

### Application Key Capabilities

Available capabilities for User resources:
- `listKeys` - List application keys
- `writeKeys` - Create application keys
- `deleteKeys` - Delete application keys
- `listBuckets` - List buckets
- `writeBuckets` - Create/modify buckets
- `listFiles` - List files in buckets
- `readFiles` - Download files
- `shareFiles` - Create download URLs
- `writeFiles` - Upload files
- `deleteFile` - Delete files

## Compatibility

This provider leverages Backblaze B2's S3-compatible API, making it compatible with:
- AWS S3 client libraries
- S3-compatible tools and workflows
- Existing S3 bucket policies and configurations

Key differences from AWS S3:
- Backblaze-specific regions and endpoints
- Different pricing model (pay per GB stored + bandwidth)
- Application keys instead of IAM users
- Bucket names must be globally unique across all Backblaze B2

## Development

### Building from Source

```bash
git clone https://github.com/rossigee/provider-backblaze
cd provider-backblaze
make build
```

### Running Tests

```bash
make test
```

### Building Container Image

```bash
make docker-build
```

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for details.

## License

This project is licensed under the Apache 2.0 License - see the [LICENSE](LICENSE) file for details.

## Support

- **Issues**: [GitHub Issues](https://github.com/rossigee/provider-backblaze/issues)  
- **Discussions**: [GitHub Discussions](https://github.com/rossigee/provider-backblaze/discussions)
- **Crossplane Slack**: [#providers channel](https://crossplane.slack.com/channels/providers)

## Roadmap

- [ ] User controller implementation for application key management
- [ ] Policy controller implementation for S3-compatible policies  
- [ ] Advanced B2-specific features (versioning, encryption)
- [ ] Integration tests with real Backblaze B2 environment
- [ ] Terraform import compatibility
- [ ] Cross-region replication support