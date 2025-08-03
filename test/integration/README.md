# Integration Tests for Provider Backblaze

This directory contains integration tests that run against real Backblaze B2 infrastructure.

## Prerequisites

1. **Backblaze B2 Account**: You need a real Backblaze B2 account with an application key that has the following capabilities:
   - `listBuckets`
   - `writeBuckets` 
   - `listFiles`
   - `readFiles`
   - `shareFiles`
   - `writeFiles`
   - `deleteFiles`
   - `listKeys`
   - `writeKeys`
   - `deleteKeys`

2. **Environment Variables**: Set the following environment variables:
   ```bash
   export B2_APPLICATION_KEY_ID="your-application-key-id"
   export B2_APPLICATION_KEY="your-application-key"
   export B2_REGION="us-west-001"  # Optional, defaults to us-west-001
   ```

## Running Integration Tests

### Run All Integration Tests
```bash
# From the provider root directory
make test-integration
```

### Run Tests Manually
```bash
# Set environment variables
export B2_APPLICATION_KEY_ID="your-key-id"
export B2_APPLICATION_KEY="your-key"

# Run from project root
go test -v ./test/integration/... -timeout 10m
```

### Run Specific Test Suites
```bash
# Test only bucket operations
go test -v ./test/integration/... -run TestBucketLifecycle -timeout 5m

# Test only application key operations  
go test -v ./test/integration/... -run TestApplicationKeyLifecycle -timeout 5m

# Test only policy operations
go test -v ./test/integration/... -run TestBucketPolicyIntegration -timeout 5m

# Test error handling
go test -v ./test/integration/... -run TestErrorHandling -timeout 5m
```

### Run Performance Benchmarks
```bash
go test -v ./test/integration/... -bench=. -benchtime=10s -timeout 10m
```

## Test Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `B2_APPLICATION_KEY_ID` | Yes | - | Your Backblaze B2 application key ID |
| `B2_APPLICATION_KEY` | Yes | - | Your Backblaze B2 application key secret |
| `B2_REGION` | No | `us-west-001` | Backblaze B2 region for testing |
| `SKIP_CLEANUP` | No | `false` | Set to `true` to skip cleanup (useful for debugging) |

### Test Timeouts

- **Individual Test**: 5 minutes
- **Cleanup Operations**: 30 seconds
- **Full Test Suite**: 10 minutes

## Test Coverage

### Bucket Operations
- ✅ Create bucket with different types and regions
- ✅ Check bucket existence
- ✅ Get bucket location/region
- ✅ List buckets (verify our bucket appears)
- ✅ Delete bucket
- ✅ Delete bucket with objects (cleanup)

### Application Key Operations (B2 Native API)
- ✅ Create application key with specific capabilities
- ✅ Get application key details
- ✅ Delete application key
- ✅ Verify key is gone after deletion

### Bucket Policy Operations (S3 Compatible API)
- ✅ Put bucket policy (apply JSON policy document)
- ✅ Get bucket policy (retrieve and verify)
- ✅ Delete bucket policy
- ✅ Verify policy is gone after deletion

### Authentication & API Compatibility
- ✅ B2 Native API authentication (application keys)
- ✅ S3 Compatible API authentication (bucket operations)
- ✅ Dual API support verification

### Error Handling
- ✅ Non-existent bucket operations
- ✅ Non-existent application key operations
- ✅ Non-existent bucket policy operations
- ✅ Graceful error handling and appropriate error messages

### Performance Benchmarks
- ✅ ListBuckets operation performance
- ✅ BucketExists operation performance
- ✅ API response time measurements

## Safety Features

### Automatic Cleanup
- All tests create resources with unique names (timestamp-based)
- Automatic cleanup runs after each test
- Set `SKIP_CLEANUP=true` to disable cleanup for debugging

### Resource Naming
- Test buckets: `provider-backblaze-test-{timestamp}`
- Test keys: `test-key-{timestamp}` or `auth-test-key-{timestamp}`
- All resources are prefixed to avoid conflicts

### Isolation
- Each test run uses unique resource names
- Tests can run in parallel without conflicts
- No shared state between test runs

## Troubleshooting

### Common Issues

1. **Missing Credentials**
   ```
   Error: Skipping integration tests - B2_APPLICATION_KEY_ID and B2_APPLICATION_KEY environment variables must be set
   ```
   **Solution**: Set the required environment variables with your B2 credentials.

2. **Permission Denied**
   ```
   Error: Failed to create bucket: AccessDenied
   ```
   **Solution**: Ensure your application key has `writeBuckets` capability.

3. **Bucket Already Exists**
   ```
   Error: Failed to create bucket: BucketAlreadyExists
   ```
   **Solution**: This should not happen with timestamp-based naming. Try running tests again.

4. **Timeout Errors**
   ```
   Error: context deadline exceeded
   ```
   **Solution**: Increase timeout or check your network connection to Backblaze B2.

### Debug Mode

To debug failed tests without cleanup:
```bash
export SKIP_CLEANUP=true
go test -v ./test/integration/... -run TestBucketLifecycle
```

This will leave test resources for manual inspection.

### Manual Cleanup

If you need to manually clean up test resources:
```bash
# List buckets to find test buckets
aws s3 ls --endpoint-url=https://s3.us-west-001.backblazeb2.com

# Delete test bucket (replace with actual bucket name)
aws s3 rb s3://provider-backblaze-test-1234567890 --force --endpoint-url=https://s3.us-west-001.backblazeb2.com
```

## CI/CD Integration

### GitHub Actions Example
```yaml
- name: Run Integration Tests
  if: github.event_name == 'push' && github.ref == 'refs/heads/master'
  env:
    B2_APPLICATION_KEY_ID: ${{ secrets.B2_APPLICATION_KEY_ID }}
    B2_APPLICATION_KEY: ${{ secrets.B2_APPLICATION_KEY }}
    B2_REGION: us-west-001
  run: |
    make test-integration
```

### Local Development
For local development, create a `.env` file (add to `.gitignore`):
```bash
# .env file (DO NOT COMMIT)
B2_APPLICATION_KEY_ID=your-key-id
B2_APPLICATION_KEY=your-key
B2_REGION=us-west-001
```

Then source it before running tests:
```bash
source .env
make test-integration
```

## Contributing

When adding new integration tests:

1. **Follow the pattern**: Use the existing test structure and naming conventions
2. **Add cleanup**: Ensure all resources are cleaned up in defer functions
3. **Use timeouts**: Set appropriate timeouts for your operations
4. **Handle errors gracefully**: Test both success and failure scenarios
5. **Document expectations**: Add comments explaining what each test validates
6. **Update this README**: Document any new test capabilities or requirements