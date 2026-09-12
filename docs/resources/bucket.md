# Bucket

**API Version**: `backblaze.m.crossplane.io/v1beta1`

Manages a Backblaze B2 storage bucket via the S3-compatible API.

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `forProvider.bucketName` | string | yes | Bucket name, globally unique across all B2 |
| `forProvider.bucketType` | string | no | `allPrivate` (default) or `allPublic` |
| `forProvider.region` | string | no | B2 region, defaults to `us-west-001` |
| `forProvider.bucketDeletionPolicy` | string | no | `DeleteIfEmpty` or `DeleteAll` (accepted but not yet enforced, see Behavior) |
| `forProvider.lifecycleRules` | array | no | Lifecycle rules (accepted but not yet applied, see Behavior) |
| `forProvider.corsRules` | array | no | CORS rules (accepted but not yet applied, see Behavior) |

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
  providerConfigRef:
    name: default
```

## Behavior

- **Create**: Creates the bucket with the given type via `CreateBucket`.
- **Update**: Not implemented; spec changes after creation are ignored until the resource is recreated.
- **Delete**: Deletes the bucket via `DeleteBucket`. The bucket must already be empty; `bucketDeletionPolicy: DeleteAll` is accepted but object emptying is not yet implemented.
- `lifecycleRules` and `corsRules` are stored in the spec but are not applied to the B2 bucket yet.

## Status Fields

- `status.atProvider.bucketName` — Bucket name
- `status.atProvider.bucketId` — B2 bucket ID (populated when observed)
- `status.atProvider.accountId` — Owning account ID
- `status.atProvider.region` — Bucket region
