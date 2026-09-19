# Bucket

**API Version**: `backblaze.m.crossplane.io/v1beta1`

Manages a Backblaze B2 storage bucket. Creation uses the native `b2_create_bucket` API (so the canonical bucket ID is known immediately); type, lifecycle, CORS, tags, encryption, and file-lock settings are reconciled via `b2_update_bucket` / the S3-compatible API.

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `forProvider.bucketName` | string | yes | Bucket name, globally unique across all B2 |
| `forProvider.bucketType` | string | no | `allPrivate` (default) or `allPublic`; drift is corrected via `b2_update_bucket` |
| `forProvider.region` | string | no | B2 region, defaults to `us-west-001` |
| `forProvider.bucketDeletionPolicy` | string | no | `DeleteIfEmpty` (default behavior) or `DeleteAll` (empties objects first) |
| `forProvider.lifecycleRules` | array | no | Lifecycle rules applied via `b2_update_bucket`; supports both `daysFromUploadingToHiding` and `daysFromHidingToDeleting` |
| `forProvider.corsRules` | array | no | CORS rules applied via S3 `PutBucketCors` |
| `forProvider.bucketInfo` | map | no | Free-form key/value metadata (cost-tracking tags, labels) applied via `b2_update_bucket` |
| `forProvider.defaultServerSideEncryption` | object | no | Default encryption-at-rest (`mode: SSE-B2`, `algorithm: AES256`); cannot be unset once configured |
| `forProvider.fileLockEnabled` | bool | no | Enables B2 file lock; required before `defaultRetention` is honoured |
| `forProvider.defaultRetention` | object | no | Default file-lock retention (`mode: governance\|compliance`, `period` in seconds); requires `fileLockEnabled: true` |

## Example

```yaml
apiVersion: backblaze.m.crossplane.io/v1beta1
kind: Bucket
metadata:
  name: my-storage-bucket
  namespace: default
spec:
  forProvider:
    bucketName: my-unique-bucket-name-12345
    region: us-west-001
    bucketType: allPrivate
    bucketDeletionPolicy: DeleteAll
    lifecycleRules:
      - fileNamePrefix: logs/
        daysFromUploadingToHiding: 30
        daysFromHidingToDeleting: 90
    corsRules:
      - corsRuleName: AllowWeb
        allowedOrigins: ["https://example.com"]
        allowedMethods: ["GET", "HEAD"]
    bucketInfo:
      env: prod
  providerConfigRef:
    name: default
```

## Behavior

- **Create**: Creates the bucket via native `b2_create_bucket` with the requested type and tags.
- **Update**: Drift in `bucketType`, `lifecycleRules`, `bucketInfo`, `defaultServerSideEncryption`, `fileLockEnabled`, and `defaultRetention` is corrected via `b2_update_bucket`; CORS drift via S3 `PutBucketCors`/`DeleteBucketCors`. Set `managementPolicies: ["Observe"]` to disable updates.
- **Delete**: With `DeleteAll`, objects are emptied first via `DeleteAllObjectsInBucket`. With `DeleteIfEmpty` (or unset), the bucket must already be empty; otherwise the resource reports `Ready=False` with reason `BucketNotEmpty` and is left for manual cleanup.

## Status Fields

- `status.atProvider.bucketName` — Bucket name
- `status.atProvider.bucketId` — B2 bucket ID
- `status.atProvider.accountId` — Owning account ID
- `status.atProvider.region` — Bucket region
