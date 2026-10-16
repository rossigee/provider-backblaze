# Configuration

Guide for configuring the Backblaze provider.

## ProviderConfig

Create a ProviderConfig to configure connection settings:

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

## Authentication

The provider uses Backblaze B2 API keys. Create a secret with your credentials:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: backblaze-creds
  namespace: crossplane-system
type: Opaque
data:
  # Base64 encoded Application Key ID
  applicationKeyId: <base64-encoded-key-id>
  # Base64 encoded Application Key secret
  applicationKey: <base64-encoded-key-secret>
```

## Bucket Configuration

Specify bucket settings in your managed resources:

```yaml
apiVersion: backblaze.m.crossplane.io/v1beta1
kind: Bucket
metadata:
  name: my-bucket
  namespace: default
spec:
  forProvider:
    bucketName: my-bucket
    region: us-west-001
    bucketType: allPrivate
  providerConfigRef:
    name: default
```
