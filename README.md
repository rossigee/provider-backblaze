# provider-backblaze

[![CI](https://img.shields.io/github/actions/workflow/status/rossigee/provider-backblaze/ci.yml?branch=master)][build]
[![Version](https://img.shields.io/github/v/release/rossigee/provider-backblaze)][releases]
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

[build]: https://github.com/rossigee/provider-backblaze/actions/workflows/ci.yml
[releases]: https://github.com/rossigee/provider-backblaze/releases

## Overview

A [Crossplane](https://crossplane.io/) provider for [Backblaze B2](https://www.backblaze.com/b2/cloud-storage.html) cloud storage. It uses Backblaze B2's S3-compatible API to manage buckets, application keys, and access policies declaratively through Kubernetes custom resources.

Resources are currently **cluster-scoped** (`backblaze.crossplane.io/v1`, `scope=Cluster`) — a namespaced v2-style migration has not yet been implemented for this provider.

## Container Registry

- **Primary**: `ghcr.io/rossigee/provider-backblaze:v0.13.2`

## Features

- **S3-Compatible**: uses Backblaze B2's S3-compatible API for maximum tooling compatibility
- **Bucket lifecycle**: automatic file lifecycle rules and CORS configuration
- **Application keys**: fine-grained capabilities, bucket-specific and file-prefix restrictions, automatic secret generation
- **Access policies**: simple bucket-level permission shortcuts or full S3-compatible JSON policy documents

## Getting Started

### Prerequisites

- Kubernetes with Crossplane installed
- A Backblaze B2 account with an application key (create one at [Backblaze B2 Console](https://secure.backblaze.com/b2_buckets.htm) → App Keys)

### Installation

```bash
kubectl apply -f - <<EOF
apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: provider-backblaze
spec:
  package: ghcr.io/rossigee/provider-backblaze:v0.13.2
EOF
```

### Configuration

```bash
kubectl create secret generic backblaze-creds \
  --namespace crossplane-system \
  --from-literal=applicationKeyId="your-key-id" \
  --from-literal=applicationKey="your-application-key"
```

```yaml
apiVersion: backblaze.crossplane.io/v1beta1
kind: ProviderConfig
metadata:
  name: default
spec:
  backblazeRegion: us-west-001
  credentials:
    source: Secret
    apiSecretRef:
      namespace: crossplane-system
      name: backblaze-creds
```

## Usage

```yaml
apiVersion: backblaze.crossplane.io/v1
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

```yaml
apiVersion: backblaze.crossplane.io/v1
kind: User
metadata:
  name: read-only-key
spec:
  forProvider:
    keyName: "read-only-application-key"
    capabilities:
      - "listFiles"
      - "readFiles"
    bucketId: "your-bucket-id"
  providerConfigRef:
    name: default
```

## Resource Types

| Resource | API Group | Description |
|----------|-----------|-------------|
| Bucket | `backblaze.crossplane.io/v1` | Buckets, with lifecycle rules and CORS |
| User | `backblaze.crossplane.io/v1` | Application keys (capabilities, bucket/prefix restrictions) |
| Policy | `backblaze.crossplane.io/v1` | S3-compatible access policies (simple bucket allow or raw JSON) |
| ProviderConfig | `backblaze.crossplane.io/v1beta1` | Provider credentials and region configuration |

## Development

```bash
# Build
make build

# Test
make test

# Lint
make lint

# Generate
make generate
```

## Contributing

Issues and pull requests are welcome at [github.com/rossigee/provider-backblaze](https://github.com/rossigee/provider-backblaze).

## License

provider-backblaze is under the Apache 2.0 license.
