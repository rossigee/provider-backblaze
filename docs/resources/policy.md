# Policy

**API Version**: `backblaze.m.crossplane.io/v1beta1`

Manages an S3-compatible bucket policy document attached to a Backblaze B2 bucket (`GetBucketPolicy` / `PutBucketPolicy` / `DeleteBucketPolicy`).

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `forProvider.allowBucket` | string | no | Shorthand: generate an allow-all policy for this bucket (mutually exclusive with `rawPolicy`); also selects the target bucket |
| `forProvider.rawPolicy` | string | no | Full policy document as JSON (mutually exclusive with `allowBucket`); target bucket is `policyName` / resource name |
| `forProvider.policyName` | string | no | Target bucket name when `rawPolicy` is used; defaults to the resource name |
| `forProvider.description` | string | no | Human-readable description (cosmetic, stored in spec only) |

Exactly one of `allowBucket` or `rawPolicy` must be set.

## Example

```yaml
apiVersion: backblaze.m.crossplane.io/v1beta1
kind: Policy
metadata:
  name: my-bucket-allow-all-policy
  namespace: default
spec:
  forProvider:
    allowBucket: my-unique-bucket
    policyName: AllowAllPolicy
  providerConfigRef:
    name: default
```

## Behavior

- **Create/Update**: Validates parameters, verifies the target bucket exists, then applies the document via `PutBucketPolicy`. Drift (including out-of-band edits in the B2 console) is corrected on every reconcile; comparison is semantic (whitespace/key-order insensitive). Set `managementPolicies: ["Observe"]` to disable writes.
- **Delete**: Removes the bucket policy via `DeleteBucketPolicy`. Already-absent policies are treated as success and never block Kubernetes garbage collection.

## Status Fields

- `status.atProvider.policyName` — Target bucket name
- `status.atProvider.policyDocument` — Applied policy document
- `status.atProvider.policyId` — Bucket name (bucket policies are bucket-scoped in B2, there is no separate ID)
- `status.atProvider.creationTime` — First-reconcile timestamp
