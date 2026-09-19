# User

**API Version**: `backblaze.m.crossplane.io/v1beta1`

Manages a Backblaze B2 application key. Generated credentials are written to `spec.writeConnectionSecretToRef` in the resource's namespace (v2 `LocalSecretReference`, same-namespace only by API design).

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `forProvider.keyName` | string | yes | Human-readable key name (mutable via `b2_update_key`) |
| `forProvider.capabilities` | array | yes | B2 capabilities (e.g. `listBuckets`, `readFiles`, `writeFiles`); drift corrected live |
| `forProvider.bucketId` | string | no | Restrict key to one bucket; passed to `b2_create_key` and corrected via `b2_update_key` |
| `forProvider.namePrefix` | string | no | Restrict key to a file prefix; passed to `b2_create_key` and corrected via `b2_update_key` |
| `forProvider.validDurationInSeconds` | integer | no | Key lifetime, max 1000 days; re-sent on update when specified |
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

- **Create**: Creates the application key via `b2_create_key` (with `bucketId`/`namePrefix` restrictions) and writes credentials to the connection secret via idempotent `CreateOrUpdate` with a controller owner reference, so `kubectl delete user` garbage-collects the secret.
- **Update**: Spec drift in key name, capabilities, restrictions, and validity is applied live via `b2_update_key` without invalidating the existing secret value. Out-of-band deletion in B2 is detected via `b2_list_keys` and triggers recreation. A manually deleted connection secret surfaces a clear error (B2 never re-returns the secret value, so the User must be recreated to rotate).
- **Delete**: Deletes the application key via `b2_delete_key` and its connection secret. Already-absent keys never block deletion. Failures emit Kubernetes Warning events.
