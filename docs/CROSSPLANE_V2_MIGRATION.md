# Crossplane v2 Migration Guide for provider-backblaze

This document provides a comprehensive guide for migrating to and using Crossplane v2 features with provider-backblaze.

## Overview

Provider-backblaze now fully supports Crossplane v2 with **dual-scope resource support**:

- **v1 APIs**: Cluster-scoped resources (legacy support)
- **v1beta1 APIs**: Namespaced resources with `.m.` API groups (Crossplane v2 recommended)

## Key Benefits of v2 Migration

### 🏢 **Multi-Tenancy & Namespace Isolation**
- Resources are isolated by namespace
- Multiple teams can use the same resource names in different namespaces
- Better RBAC control at the namespace level

### 🔐 **Enhanced Security**
- Fine-grained permissions per namespace
- Secrets stay within namespace boundaries
- Reduced cluster-wide access requirements

### 🚀 **Modern Crossplane Features**
- Native support for Crossplane v2.0+ features
- Better integration with Crossplane composition functions
- Improved resource lifecycle management

## API Version Comparison

| Aspect | v1 (Legacy) | v1beta1 (v2 Recommended) |
|--------|-------------|---------------------------|
| **Scope** | Cluster-scoped | Namespaced |
| **API Group** | `backblaze.crossplane.io` | `*.backblaze.m.crossplane.io` |
| **Multi-tenancy** | ❌ | ✅ |
| **RBAC** | Cluster-wide | Namespace-level |
| **Isolation** | None | Namespace boundaries |

## Migration Strategies

### Strategy 1: Gradual Migration (Recommended)

Deploy new resources using v1beta1 APIs while keeping existing v1 resources:

```yaml
# New resources - use v1beta1 (recommended)
apiVersion: bucket.backblaze.m.crossplane.io/v1beta1
kind: Bucket
metadata:
  name: new-bucket
  namespace: my-team
spec:
  forProvider:
    bucketName: my-team-new-bucket
    region: us-west-001
---
# Existing resources - keep v1 until convenient to migrate
apiVersion: bucket.backblaze.crossplane.io/v1
kind: Bucket
metadata:
  name: legacy-bucket
spec:
  forProvider:
    bucketName: legacy-bucket
    region: us-west-001
```

### Strategy 2: Full Migration

1. **Export existing resources**: `kubectl get buckets.backblaze.crossplane.io -o yaml > v1-backup.yaml`
2. **Delete v1 resources**: `kubectl delete -f v1-resources.yaml`
3. **Create v1beta1 equivalents** with namespace assignments
4. **Update any references** in compositions or applications

## Resource Migration Examples

### Bucket Migration

#### Before (v1 - cluster-scoped)
```yaml
apiVersion: bucket.backblaze.crossplane.io/v1
kind: Bucket
metadata:
  name: company-data-bucket
spec:
  providerConfigRef:
    name: default
  forProvider:
    bucketName: company-data-bucket-unique
    region: us-west-001
    bucketType: allPrivate
    bucketDeletionPolicy: DeleteIfEmpty
  deletionPolicy: Delete
```

#### After (v1beta1 - namespaced)
```yaml
apiVersion: bucket.backblaze.m.crossplane.io/v1beta1
kind: Bucket
metadata:
  name: company-data-bucket
  namespace: storage-team    # 🆕 Namespace isolation
spec:
  providerConfigRef:
    name: default
  forProvider:
    bucketName: company-data-bucket-unique
    region: us-west-001
    bucketType: allPrivate
    bucketDeletionPolicy: DeleteIfEmpty
  deletionPolicy: Delete
```

### User (Application Key) Migration

#### Before (v1 - cluster-scoped)
```yaml
apiVersion: user.backblaze.crossplane.io/v1
kind: User
metadata:
  name: app-reader-key
spec:
  providerConfigRef:
    name: default
  forProvider:
    keyName: application-reader
    capabilities:
      - listFiles
      - readFiles
    writeSecretToRef:
      name: backblaze-app-key
      namespace: default
```

#### After (v1beta1 - namespaced)
```yaml
apiVersion: user.backblaze.m.crossplane.io/v1beta1
kind: User
metadata:
  name: app-reader-key
  namespace: my-app          # 🆕 Namespace isolation
spec:
  providerConfigRef:
    name: default
  forProvider:
    keyName: application-reader
    capabilities:
      - listFiles
      - readFiles
    writeSecretToRef:
      name: backblaze-app-key
      namespace: my-app       # 🆕 Secret stays in same namespace
```

### Policy Migration

#### Before (v1 - cluster-scoped)
```yaml
apiVersion: policy.backblaze.crossplane.io/v1
kind: Policy
metadata:
  name: bucket-access-policy
spec:
  providerConfigRef:
    name: default
  forProvider:
    allowBucket: my-bucket
    policyName: bucket-reader
    description: Allow read access to specific bucket
```

#### After (v1beta1 - namespaced)
```yaml
apiVersion: policy.backblaze.m.crossplane.io/v1beta1
kind: Policy
metadata:
  name: bucket-access-policy
  namespace: my-app          # 🆕 Namespace isolation
spec:
  providerConfigRef:
    name: default
  forProvider:
    allowBucket: my-bucket
    policyName: bucket-reader
    description: Allow read access to specific bucket
```

## RBAC Configuration for v2

### Namespace-Scoped Permissions
```yaml
# Allow team to manage Backblaze resources in their namespace only
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  namespace: team-storage
  name: backblaze-manager
rules:
- apiGroups: ["bucket.backblaze.m.crossplane.io"]
  resources: ["buckets"]
  verbs: ["get", "list", "create", "update", "patch", "delete"]
- apiGroups: ["user.backblaze.m.crossplane.io"]
  resources: ["users"]
  verbs: ["get", "list", "create", "update", "patch", "delete"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: team-storage-backblaze
  namespace: team-storage
subjects:
- kind: User
  name: storage-team-lead
roleRef:
  kind: Role
  name: backblaze-manager
  apiGroup: rbac.authorization.k8s.io
```

### Cross-Namespace Resource Viewing
```yaml
# Allow read-only access across multiple namespaces
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: backblaze-viewer
rules:
- apiGroups: ["bucket.backblaze.m.crossplane.io"]
  resources: ["buckets"]
  verbs: ["get", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: platform-team-backblaze-viewer
subjects:
- kind: Group
  name: platform-team
roleRef:
  kind: ClusterRole
  name: backblaze-viewer
  apiGroup: rbac.authorization.k8s.io
```

## Composition Updates for v2

### v1beta1 Composite Resource Definition (XRD)
```yaml
apiVersion: apiextensions.crossplane.io/v1
kind: CompositeResourceDefinition
metadata:
  name: xbuckets.platform.company.com
spec:
  group: platform.company.com
  names:
    kind: XBucket
    plural: xbuckets
  claimNames:
    kind: BucketClaim
    plural: bucketclaims
  versions:
  - name: v1alpha1
    served: true
    referenceable: true
    schema:
      openAPIV3Schema:
        type: object
        properties:
          spec:
            type: object
            properties:
              parameters:
                type: object
                properties:
                  bucketName:
                    type: string
                  region:
                    type: string
                    default: "us-west-001"
          status:
            type: object
```

### v1beta1 Composition
```yaml
apiVersion: apiextensions.crossplane.io/v1
kind: Composition
metadata:
  name: backblaze-bucket-composition
spec:
  compositeTypeRef:
    apiVersion: platform.company.com/v1alpha1
    kind: XBucket

  resources:
  - name: bucket
    base:
      # 🆕 Use v1beta1 namespaced API
      apiVersion: bucket.backblaze.m.crossplane.io/v1beta1
      kind: Bucket
      metadata:
        # 🆕 Namespace will be set from claim namespace
        namespace: ""
      spec:
        providerConfigRef:
          name: default
        forProvider:
          region: us-west-001
          bucketType: allPrivate

    patches:
    - fromFieldPath: metadata.name
      toFieldPath: metadata.name
    - fromFieldPath: metadata.namespace  # 🆕 Pass namespace from claim
      toFieldPath: metadata.namespace
    - fromFieldPath: spec.parameters.bucketName
      toFieldPath: spec.forProvider.bucketName
    - fromFieldPath: spec.parameters.region
      toFieldPath: spec.forProvider.region
```

### Namespaced Bucket Claim
```yaml
apiVersion: platform.company.com/v1alpha1
kind: BucketClaim
metadata:
  name: my-app-storage
  namespace: my-app      # 🆕 Claim and bucket both in same namespace
spec:
  parameters:
    bucketName: my-app-unique-bucket
    region: us-west-001
  compositionRef:
    name: backblaze-bucket-composition
```

## Troubleshooting v2 Migration

### Common Issues

#### 1. Namespace Not Specified
**Error**: `the namespace of the provided object does not match the namespace on the request`

**Solution**: Ensure all v1beta1 resources have a `namespace` specified:
```yaml
metadata:
  name: my-resource
  namespace: my-namespace  # ← Required for v1beta1
```

#### 2. Wrong API Group
**Error**: `no matches for kind "Bucket" in version "backblaze.crossplane.io/v1beta1"`

**Solution**: Use the correct API group with `.m.` pattern:
```yaml
# ❌ Wrong
apiVersion: backblaze.crossplane.io/v1beta1

# ✅ Correct
apiVersion: bucket.backblaze.m.crossplane.io/v1beta1
```

#### 3. RBAC Permission Denied
**Error**: `buckets.bucket.backblaze.m.crossplane.io is forbidden`

**Solution**: Update RBAC rules for new API groups:
```yaml
rules:
- apiGroups: ["bucket.backblaze.m.crossplane.io"]  # ← Note .m.
  resources: ["buckets"]
  verbs: ["get", "list", "create", "update", "patch", "delete"]
```

### Validation Commands

```bash
# Check provider supports both versions
kubectl get crd | grep backblaze

# Expected output should include both:
# buckets.backblaze.crossplane.io (v1)
# buckets.bucket.backblaze.m.crossplane.io (v1beta1)

# Validate namespaced resource creation
kubectl apply --dry-run=server -f - <<EOF
apiVersion: bucket.backblaze.m.crossplane.io/v1beta1
kind: Bucket
metadata:
  name: test-bucket
  namespace: default
spec:
  forProvider:
    bucketName: test-bucket
    region: us-west-001
EOF

# List resources in specific namespace
kubectl get buckets.bucket.backblaze.m.crossplane.io -n my-namespace
```

## Best Practices

### 🎯 **Namespace Strategy**
- Use meaningful namespace names: `team-storage`, `app-prod`, `app-staging`
- Map namespaces to teams, applications, or environments
- Consider using namespace prefixes for bucket names to avoid conflicts

### 🔐 **Security**
- Grant minimal RBAC permissions per namespace
- Use separate provider configs per namespace if needed
- Keep secrets within resource namespaces

### 📝 **Resource Naming**
- Use consistent naming conventions across namespaces
- Include environment/team identifiers in resource names
- Document resource ownership and purpose

### 🔄 **Migration Planning**
- Start with non-production environments
- Test RBAC changes thoroughly
- Plan for gradual migration over time
- Keep v1 resources until migration is complete

## Feature Compatibility Matrix

| Feature | v1 Support | v1beta1 Support | Notes |
|---------|------------|-----------------|-------|
| Basic bucket operations | ✅ | ✅ | Full compatibility |
| Application key management | ✅ | ✅ | Secrets stay in namespace |
| Policy management | ✅ | ✅ | Namespace-scoped policies |
| Lifecycle rules | ✅ | ✅ | All B2 features supported |
| CORS configuration | ✅ | ✅ | Web application support |
| Provider configs | ✅ | ✅ | Shared across versions |
| Compositions | ✅ | ✅ | v1beta1 enables better isolation |

## Support and Migration Assistance

For questions about migrating to Crossplane v2:

1. **Validate your setup**: Run `test/validate_v2_deployment.sh`
2. **Test changes**: Use `--dry-run=server` to validate resources
3. **Check examples**: Review `/examples/v1beta1/` directory
4. **Integration tests**: Run tests with `go test ./test/integration/...`

The provider maintains backward compatibility, so you can migrate at your own pace while taking advantage of v2 features immediately for new resources.