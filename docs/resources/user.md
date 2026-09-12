# User

**API Version**: `backblaze.m.crossplane.io/v1beta1`

Manages a Backblaze B2 application key. Generated credentials are written to `spec.writeConnectionSecretToRef` in the resource's namespace.

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `forProvider.keyName` | string | yes | Human-readable key name |
| `forProvider.capabilities` | array | yes | B2 capabilities (e.g. `listBuckets`, `readFiles`, `writeFiles`) |
| `forProvider.bucketId` | string | no | Restrict key to one bucket (accepted but not yet enforced, see Behavior) |
| `forProvider.namePrefix` | string | no | Restrict key to a file prefix (accepted but not yet enforced, see Behavior) |
| `forProvider.validDurationInSeconds` | integer | no | Key lifetime, max 1000 days |
| `writeConnectionSecretToRef.name` | string | no | Secret to receive `applicationKeyId` and `applicationKey` |

## Example

```yaml
apiVersion: backblaze.m.crossplane.io/v1beta1
kind: User
metadata:
  name: my-app-key
  namespace: default
spec:
  forProvider:
    keyName: my-application-key
    capabilities:
      - listBuckets
      - listFiles
      - readFiles
  writeConnectionSecretToRef:
    name: my-app-key-secret
  providerConfigRef:
    name: default
```

## Behavior

- **Create**: Creates the application key via `b2_create_key` and writes credentials to the connection secret.
- **Update**: Not implemented; spec changes after creation are ignored until the resource is recreated.
- **Delete**: Deletes the application key and its connection secret.
- `bucketId` and `namePrefix` restrictions are stored in the spec but are not passed to the B2 API yet; keys are created unrestricted.

## Status Fields

- `status.atProvider.applicationKeyId` — Created key ID
- `status.atProvider.accountId` — Owning account ID
- `status.atProvider.capabilities` — Granted capabilities
- `status.atProvider.expirationTimestamp` — Expiry, when `validDurationInSeconds` is set
