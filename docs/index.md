# Provider Backblaze Documentation

A Crossplane v2 provider for managing Backblaze B2 storage resources. All resources are namespaced (`backblaze.m.crossplane.io/v1beta1`) with full multi-tenancy support.

## Quick Links

- [Configuration](configuration.md) — Authentication and connection setup
- [Getting Started](getting-started.md) — Installation and first resources
- [Development](development.md) — Building, testing, and contributing

## Resource Documentation

| Resource | API Group | Description |
|----------|-----------|-------------|
| [Bucket](resources/bucket.md) | `backblaze.m.crossplane.io/v1beta1` | B2 bucket management |
| [User](resources/user.md) | `backblaze.m.crossplane.io/v1beta1` | Application key management |
| [Policy](resources/policy.md) | `backblaze.m.crossplane.io/v1beta1` | S3-compatible policy documents (status-only) |
| ProviderConfig | `backblaze.m.crossplane.io/v1beta1` | Provider credentials and region |

## API Coverage Gaps

B2 capabilities not yet exposed by this provider:

- **Bucket updates**: `lifecycleRules` and `corsRules` are accepted in spec but never applied; spec changes after creation are ignored (no update path).
- **Bucket deletion**: `bucketDeletionPolicy: DeleteAll` is accepted but objects are not emptied before `DeleteBucket`; buckets must already be empty.
- **Key restrictions**: `User` `bucketId`/`namePrefix` are accepted but not passed to `b2_create_key`; keys are created unrestricted.
- **Policy enforcement**: `Policy` is validated and recorded in status only; B2 has no bucket-policy API and nothing is applied externally.
- **Missing resources**: file/large-file management, bucket versioning/SSE/ObjectLock settings, and cross-account replication are not modeled.
