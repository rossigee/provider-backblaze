# Getting Started

Guide to getting started with the provider.

## Installation

Install the provider:

```bash
kubectl apply -f - <<EOF
apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: provider-backblaze
spec:
  package: ghcr.io/rossigee/provider-backblaze:v0.17.0
EOF
```

## Prerequisites

- Kubernetes cluster with Crossplane installed
- A Backblaze B2 account with an application key

## Quick Start

1. Create credentials secret:

```bash
kubectl create secret generic backblaze-creds \
  --namespace crossplane-system \
  --from-literal=applicationKeyId="your-key-id" \
  --from-literal=applicationKey="your-application-key"
```

2. Create a ProviderConfig:

```yaml
apiVersion: backblaze.crossplane.io/v1beta1
kind: ProviderConfig
metadata:
  name: default
spec:
  backblazeRegion: us-west-001
  credentials:
    source: Secret
    secretRef:
      namespace: crossplane-system
      name: backblaze-creds
```

3. Create a Bucket:

```yaml
apiVersion: bucket.backblaze.crossplane.io/v1
kind: Bucket
metadata:
  name: my-bucket
spec:
  forProvider:
    bucketName: my-unique-bucket-name
    region: us-west-001
    bucketType: allPrivate
  providerConfigRef:
    name: default
```
