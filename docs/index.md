# Provider Backblaze Documentation

A Crossplane provider for managing Backblaze B2 storage resources.

## Quick Links

- [Development](development.md) — Building, testing, and contributing

## Resource Documentation

Resources are documented in the API types. Individual resource documentation will be added to the [resources/](resources/) folder.

### Storage Resources

| Resource | v1 API Group | v1beta1 API Group | Description |
|----------|--------------|-------------------|-------------|
| Bucket | `backblaze.crossplane.io/v1` | `bucket.backblaze.m.crossplane.io/v1beta1` | B2 bucket management |
| Policy | `backblaze.crossplane.io/v1` | `policy.backblaze.m.crossplane.io/v1beta1` | Bucket policies |
| User | `backblaze.crossplane.io/v1` | `user.backblaze.m.crossplane.io/v1beta1` | Application key management |
| ProviderConfig | `backblaze.crossplane.io/v1beta1` | — | Provider credentials and region |

## API Versions

- **v1**: Cluster-scoped resources (legacy)
- **v1beta1**: Namespaced resources with Crossplane v2 multi-tenancy support

See the [Migration Guide](crossplane-v2-migration.md) for details on upgrading to v1beta1.
