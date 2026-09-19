# Provider Backblaze Documentation

A Crossplane v2 provider for managing Backblaze B2 storage resources. All resources are namespaced (`backblaze.m.crossplane.io/v1beta1`) with full multi-tenancy support.

## Quick Links

- [Configuration](configuration.md) — Authentication and connection setup
- [Getting Started](getting-started.md) — Installation and first resources
- [Development](development.md) — Building, testing, and contributing

## Resource Documentation

| Resource | API Group | Description |
|----------|-----------|-------------|
| [Bucket](resources/bucket.md) | `backblaze.m.crossplane.io/v1beta1` | B2 bucket management (type, lifecycle, CORS, tags, SSE, file lock) |
| [User](resources/user.md) | `backblaze.m.crossplane.io/v1beta1` | Application key management with live updates |
| [Policy](resources/policy.md) | `backblaze.m.crossplane.io/v1beta1` | S3-compatible bucket policies, applied to B2 |
| [BucketNotification](resources/notification.md) | `backblaze.m.crossplane.io/v1beta1` | B2 event notification rules (webhooks) |
| ProviderConfig | `backblaze.m.crossplane.io/v1beta1` | Provider credentials and region |

## API Coverage Gaps

B2 capabilities not yet exposed by this provider:

- **File management**: individual file/large-file upload, download, hide, and delete are intentionally out of scope for this provider.
- **Cross-account replication**: B2 Cloud Replication rules are not modeled.
- **SSE-C**: customer-key encryption settings are accepted in the type but cannot be applied declaratively through `b2_update_bucket`; `SSE-B2` is fully supported.
