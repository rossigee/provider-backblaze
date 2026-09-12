# Policy

**API Version**: `backblaze.m.crossplane.io/v1beta1`

Records an S3-compatible access-policy document against a bucket. Backblaze B2 has no native bucket-policy API, so the document is validated and stored in status only; nothing is applied externally.

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `forProvider.allowBucket` | string | no | Shorthand: generate an allow-all policy for this bucket (mutually exclusive with `rawPolicy`) |
| `forProvider.rawPolicy` | string | no | Full policy document as JSON (mutually exclusive with `allowBucket`) |
| `forProvider.policyName` | string | no | Policy name, defaults to the resource name |
| `forProvider.description` | string | no | Human-readable description |

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

- **Create**: Validates the parameters (exactly one of `allowBucket`/`rawPolicy`, valid JSON) and records the document in status. No external call is made.
- **Update**: Not implemented; spec changes after creation are ignored until the resource is recreated.
- **Delete**: No external call; status-only resource.

## Status Fields

- `status.atProvider.policyName` — Policy name
- `status.atProvider.policyDocument` — Validated policy document
- `status.atProvider.policyId` — Generated local ID
- `status.atProvider.creationTime` — Creation timestamp
