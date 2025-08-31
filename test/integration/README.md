# Integration Tests for Provider Backblaze

This directory contains integration tests that validate the provider against real Backblaze B2 services.

## Setup

### Prerequisites

1. **Backblaze B2 Account**: You need a Backblaze B2 account with application keys
2. **Application Keys**: Create application keys in your Backblaze B2 console
3. **Go 1.21+**: Required to run the tests
4. **Network Access**: Tests require internet connectivity to reach Backblaze B2 APIs

### Environment Variables

Set the following environment variables before running tests:

```bash
export B2_APPLICATION_KEY_ID="K005xxxxxxxxxxxxx"     # Your application key ID
export B2_APPLICATION_KEY="xxxxxxxxxxxxxxxxxx"       # Your application key secret
export B2_REGION="us-west-001"                       # Optional: defaults to us-west-001
export SKIP_CLEANUP="false"                          # Optional: set to "true" to skip cleanup for debugging
```

**Important**: Use test credentials with limited permissions to avoid accidental data loss.

## Running Tests

### All Integration Tests

```bash
go test -v ./test/integration/...
```

### Specific Test Functions

```bash
# Run only bucket lifecycle tests
go test -v ./test/integration/ -run TestBucketLifecycleIntegration

# Run only application key tests  
go test -v ./test/integration/ -run TestApplicationKeyLifecycleIntegration

# Run only multi-region tests
go test -v ./test/integration/ -run TestMultiRegionBucketIntegration
```

### Short Mode (Skip Integration Tests)

```bash
go test -short ./test/integration/...
```

### Benchmarks

```bash
go test -bench=. ./test/integration/...
```

## Test Categories

### Core Functionality Tests

- **`TestBackblazeClientIntegration`**: Basic client connection and authentication
- **`TestBucketLifecycleIntegration`**: Complete bucket lifecycle (create, exists, delete)
- **`TestApplicationKeyLifecycleIntegration`**: Application key management
- **`TestBucketPolicyIntegration`**: S3-compatible bucket policies

### Advanced Feature Tests

- **`TestMultiRegionBucketIntegration`**: Cross-region bucket operations
- **`TestConcurrentBucketOperations`**: Concurrent operation safety
- **`TestBucketPolicyAdvancedIntegration`**: Complex policy scenarios
- **`TestBucketS3CompatibilityIntegration`**: S3 API compatibility
- **`TestApplicationKeyCapabilitiesIntegration`**: Fine-grained key permissions
- **`TestBucketRegionValidationIntegration`**: Region validation and endpoint generation

### Error Handling & Edge Cases

- **`TestErrorHandlingIntegration`**: Error conditions and recovery
- **`TestEdgeCasesIntegration`**: Boundary conditions and invalid inputs
- **`TestTimeoutAndRetryIntegration`**: Timeout handling and retry logic

### Authentication Tests

- **`TestB2AuthenticationIntegration`**: Both S3-compatible and B2 native API authentication

## Test Configuration

### Timeouts

- **Test Timeout**: 5 minutes per test
- **Cleanup Timeout**: 30 seconds for cleanup operations

### Resource Naming

All test resources use the prefix `provider-backblaze-test` with timestamps to ensure uniqueness and avoid conflicts.

### Cleanup Behavior

- **Default**: All test resources are cleaned up automatically
- **Debug Mode**: Set `SKIP_CLEANUP=true` to preserve resources for inspection
- **Failed Tests**: Resources may be left behind if tests fail unexpectedly

## Troubleshooting

### Common Issues

1. **Authentication Errors**
   ```
   Error: Failed to create Backblaze client: applicationKeyId and applicationKey are required
   ```
   **Solution**: Ensure `B2_APPLICATION_KEY_ID` and `B2_APPLICATION_KEY` environment variables are set.

2. **Rate Limiting**
   ```
   Error: API rate limit exceeded
   ```
   **Solution**: B2 has API rate limits. Wait a few minutes and retry.

3. **Network Timeouts**
   ```
   Error: context deadline exceeded
   ```
   **Solution**: Check internet connectivity and B2 service status.

4. **Permission Denied**
   ```
   Error: insufficient permissions for operation
   ```
   **Solution**: Ensure your application keys have the required capabilities.

### Required Capabilities

Your application keys should have these capabilities for full test coverage:

- `listBuckets` - List buckets in the account
- `listFiles` - List files in buckets
- `readFiles` - Download files from buckets
- `shareFiles` - Create download URLs
- `writeFiles` - Upload files to buckets
- `deleteFiles` - Delete files from buckets
- `writeBuckets` - Create and modify buckets
- `deleteKeys` - Delete application keys (for key management tests)
- `writeKeys` - Create application keys (for key management tests)

### Debug Mode

Enable debug mode to preserve test resources for inspection:

```bash
export SKIP_CLEANUP=true
go test -v ./test/integration/ -run TestBucketLifecycleIntegration
```

Remember to manually clean up resources afterward:

```bash
# List buckets to see test resources
b2 list-buckets | grep provider-backblaze-test

# Clean up buckets
b2 delete-bucket provider-backblaze-test-xxxxx

# List application keys
b2 list-keys

# Clean up application keys  
b2 delete-key keyId
```

## Performance Benchmarks

The integration tests include benchmarks for common operations:

- `BenchmarkBackblazeOperations/ListBuckets`: Measures bucket listing performance
- `BenchmarkBackblazeOperations/BucketExists`: Measures bucket existence check performance

Run benchmarks with:

```bash
go test -bench=BenchmarkBackblazeOperations ./test/integration/...
```

## Security Considerations

- **Test Credentials**: Always use dedicated test credentials with minimal permissions
- **Resource Isolation**: Test resources are isolated using unique naming conventions
- **Cleanup**: Automatic cleanup prevents resource accumulation
- **No Secrets**: Tests do not log or expose credential information

## Contributing

When adding new integration tests:

1. Follow the existing naming convention: `TestFeatureNameIntegration`
2. Include proper cleanup functions to prevent resource leaks
3. Add appropriate timeout handling
4. Test both success and failure scenarios
5. Update this README with new test descriptions

## Cost Considerations

Integration tests create and delete B2 resources, which may incur minimal costs:

- **Bucket Operations**: Usually free within B2 limits
- **Application Keys**: Free to create and delete
- **API Calls**: Minimal cost for API operations
- **Storage**: Tests don't upload significant data

The costs should be negligible for testing purposes, but monitor your B2 usage if running tests frequently.