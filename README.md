# provider-backblaze

[![CI](https://img.shields.io/github/actions/workflow/status/rossigee/provider-backblaze/ci.yml?branch=master)][build]
[![Version](https://img.shields.io/github/v/release/rossigee/provider-backblaze)][releases]
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

[build]: https://github.com/rossigee/provider-backblaze/actions/workflows/ci.yml
[releases]: https://github.com/rossigee/provider-backblaze/releases

## Overview

A [Crossplane](https://crossplane.io/) provider for [Backblaze B2](https://www.backblaze.com/b2/cloud-storage.html) cloud storage. It uses Backblaze B2's S3-compatible API to manage buckets, application keys, and access policies declaratively through Kubernetes custom resources.

All resources are **namespaced** (`backblaze.m.crossplane.io/v1beta1`) for Crossplane v2 multi-tenancy.

## Container Registry

- **Primary**: `ghcr.io/rossigee/provider-backblaze:v0.19.0`

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
  package: ghcr.io/rossigee/provider-backblaze:v0.19.0
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
apiVersion: backblaze.m.crossplane.io/v1beta1
kind: ProviderConfig
metadata:
  name: default
  namespace: crossplane-system
spec:
  backblazeRegion: us-west-001
  credentials:
    source: Secret
    secretRef:
      namespace: crossplane-system
      name: backblaze-creds
```

## Usage

```yaml
apiVersion: backblaze.m.crossplane.io/v1beta1
kind: Bucket
metadata:
  name: my-storage
  namespace: default
spec:
  forProvider:
    bucketName: my-unique-bucket-name
    region: us-west-001
    bucketType: allPrivate
  providerConfigRef:
    name: default
```

```yaml
apiVersion: backblaze.m.crossplane.io/v1beta1
kind: User
metadata:
  name: read-only-key
  namespace: default
spec:
  forProvider:
    keyName: "read-only-application-key"
    capabilities:
      - "listFiles"
      - "readFiles"
  writeConnectionSecretToRef:
    name: read-only-key-secret
  providerConfigRef:
    name: default
```

## Resource Types

| Resource | API Group | Description |
|----------|-----------|-------------|
| Bucket | `backblaze.m.crossplane.io/v1beta1` | Buckets (lifecycle/CORS accepted, not yet applied) |
| User | `backblaze.m.crossplane.io/v1beta1` | Application keys (bucket/prefix restrictions accepted, not yet enforced) |
| Policy | `backblaze.m.crossplane.io/v1beta1` | Policy documents, validated and recorded in status only |
| ProviderConfig | `backblaze.m.crossplane.io/v1beta1` | Provider credentials and region configuration |

See [docs/index.md](docs/index.md) for the full reference and [API coverage gaps](docs/index.md#api-coverage-gaps).

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

## Implementation

This provider is a native Crossplane controller that directly implements the provider APIs without using Terraform or upjet scaffolding. This approach yields smaller binaries, simpler code, and reduced dependencies.
