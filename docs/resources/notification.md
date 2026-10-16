# BucketNotification

**API Version**: `backblaze.m.crossplane.io/v1beta1`

Manages a Backblaze B2 bucket event notification rule (`b2_create_event_notification` / `b2_update_event_notification` / `b2_delete_event_notification`). B2 delivers the subscribed events to the configured HTTPS webhook.

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `forProvider.bucketId` | string | no | B2 bucket ID (preferred; B2 addresses notification rules by bucket ID). One of `bucketId`/`bucketName` is required |
| `forProvider.bucketName` | string | no | Bucket name, resolved via `b2_list_buckets` when `bucketId` is unset |
| `forProvider.name` | string | yes | Rule name, unique per bucket |
| `forProvider.events` | array | yes | Subscribed events: `b2:ObjectCreated`, `b2:ObjectDeleted`, `b2:ObjectHidden`, `b2:ObjectNotHidden` |
| `forProvider.webhookUrl` | string | yes | HTTPS endpoint B2 delivers to |
| `forProvider.description` | string | no | Human-readable note |
| `forProvider.disabled` | bool | no | Suspend delivery without removing the rule |

## Example

```yaml
apiVersion: backblaze.m.crossplane.io/v1beta1
kind: BucketNotification
metadata:
  name: my-bucket-events
  namespace: default
spec:
  forProvider:
    bucketName: my-unique-bucket-name-12345
    name: uploads-hook
    events:
      - b2:ObjectCreated
      - b2:ObjectDeleted
    webhookUrl: https://example.com/b2-events
  providerConfigRef:
    name: default
```

See also: `examples/notification.yaml`.

## Behavior

- **Create**: Resolves the bucket, then creates the rule. The external name is set to `<bucketId>/<ruleName>`.
- **Update**: Drift in name, events, webhook URL, description, or `disabled` is patched in place via `b2_update_event_notification`. Set `managementPolicies: ["Observe"]` to disable writes.
- **Delete**: Removes the rule via `b2_delete_event_notification`. Already-absent rules never block deletion.

## Status Fields

- `status.atProvider.notificationId` — B2 rule ID
- `status.atProvider.bucketId` — Target bucket ID
- `status.atProvider.events` — Subscribed events
