# Development Guide

This guide covers development setup, building, testing, and contributing to the Backblaze provider.

## Table of Contents
- [Development Environment](#development-environment)
- [Building the Provider](#building-the-provider)
- [Running Locally](#running-locally)
- [Testing](#testing)
- [Adding New Resources](#adding-new-resources)
- [Debugging](#debugging)
- [Release Process](#release-process)

## Development Environment

### Prerequisites

- Go 1.21+
- Docker
- Kind or another Kubernetes cluster
- Kubectl
- Crossplane CLI (optional but recommended)
- Backblaze B2 account for testing

### Setup

1. **Clone the repository**
   ```bash
   git clone https://github.com/rossigee/provider-backblaze
   cd provider-backblaze
   ```

2. **Install dependencies**
   ```bash
   go mod download
   ```

3. **Install code generation tools**
   ```bash
   go install -tags generate sigs.k8s.io/controller-tools/cmd/controller-gen@v0.13.0
   go install -tags generate github.com/crossplane/crossplane-tools/cmd/angryjet@master
   ```

4. **Setup a local Kubernetes cluster**
   ```bash
   kind create cluster --name crossplane-dev
   ```

5. **Install Crossplane**
   ```bash
   kubectl create namespace crossplane-system
   
   helm repo add crossplane-stable https://charts.crossplane.io/stable
   helm repo update
   
   helm install crossplane \
     crossplane-stable/crossplane \
     --namespace crossplane-system \
     --set args='{"--debug","--enable-management-policies"}'
   ```

## Building the Provider

### Generate Code

Always regenerate code after modifying API types:

```bash
make generate
```

This generates:
- CRD YAML files in `package/crds/`
- DeepCopy methods (`zz_generated.deepcopy.go`)
- Managed resource methods (`zz_generated.managed.go`)

### Build Binary

```bash
make build
```

The binary will be at `_output/bin/provider`

### Build Docker Image

```bash
# Build for current platform
make docker-build

# Build for multiple platforms
make docker-build PLATFORMS="linux_amd64 linux_arm64"
```

### Build Provider Package

```bash
make xpkg.build
```

The package will be at `_output/xpkg/`

## Running Locally

### Option 1: Out-of-Cluster

1. **Export kubeconfig**
   ```bash
   export KUBECONFIG=~/.kube/config
   ```

2. **Create Backblaze credentials secret**
   ```bash
   kubectl create secret generic backblaze-creds \
     --from-literal=credentials='{"applicationKeyId":"K005xxxxxxxxxxxxx","applicationKey":"xxxxxxxxxxxxxxxxxx","region":"us-west-001"}' \
     -n crossplane-system
   ```

3. **Create ProviderConfig**
   ```bash
   kubectl apply -f examples/provider/config.yaml
   ```

4. **Run the provider**
   ```bash
   make run
   ```

### Option 2: In-Cluster with Kind

1. **Build and load image**
   ```bash
   make docker-build
   kind load docker-image ghcr.io/rossigee/provider-backblaze:latest --name crossplane-dev
   ```

2. **Install provider**
   ```yaml
   apiVersion: pkg.crossplane.io/v1
   kind: Provider
   metadata:
     name: provider-backblaze
   spec:
     package: ghcr.io/rossigee/provider-backblaze:latest
     packagePullPolicy: Never  # Use local image
   ```

## Testing

### Unit Tests

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run specific package tests
go test ./internal/clients -v

# Run with race detection
go test -race ./...
```

### Integration Tests

Create a test environment with real Backblaze B2 credentials:

```bash
# 1. Set environment variables
export B2_APPLICATION_KEY_ID="K005xxxxxxxxxxxxx"
export B2_APPLICATION_KEY="xxxxxxxxxxxxxxxxxx"
export B2_REGION="us-west-001"

# 2. Apply test credentials
kubectl create secret generic backblaze-test-creds \
  --from-literal=credentials='{"applicationKeyId":"'$B2_APPLICATION_KEY_ID'","applicationKey":"'$B2_APPLICATION_KEY'","region":"'$B2_REGION'"}' \
  -n crossplane-system

# 3. Apply test resources
kubectl apply -f examples/provider/config.yaml
kubectl apply -f examples/bucket/bucket.yaml

# 4. Check resource status
kubectl describe bucket.bucket.backblaze.crossplane.io example-bucket
```

### E2E Tests

```bash
# Run e2e tests (requires real Backblaze B2 access)
export B2_APPLICATION_KEY_ID="your-key-id"
export B2_APPLICATION_KEY="your-application-key"
export B2_REGION="us-west-001"
go test -tags=e2e ./test/e2e/...
```

## Adding New Resources

### 1. Define API Types

Create new types in `apis/<group>/v1/types.go`:

```go
// UserParameters are the configurable fields of a User (Application Key)
type UserParameters struct {
    KeyName      string   `json:"keyName"`
    Capabilities []string `json:"capabilities"`
    BucketID     *string  `json:"bucketId,omitempty"`
    NamePrefix   *string  `json:"namePrefix,omitempty"`
}

// UserObservation are the observable fields of a User
type UserObservation struct {
    ApplicationKeyID string `json:"applicationKeyId,omitempty"`
    ApplicationKey   string `json:"applicationKey,omitempty"`
}

// UserSpec defines the desired state of a User
type UserSpec struct {
    xpv1.ResourceSpec `json:",inline"`
    ForProvider       UserParameters `json:"forProvider"`
}

// UserStatus represents the observed state of a User
type UserStatus struct {
    xpv1.ResourceStatus `json:",inline"`
    AtProvider          UserObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// User is a managed resource representing a Backblaze B2 Application Key
type User struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   UserSpec   `json:"spec"`
    Status UserStatus `json:"status,omitempty"`
}
```

### 2. Add Controller

Create controller in `internal/controller/user/user.go`:

```go
func Setup(mgr ctrl.Manager, o controller.Options) error {
    name := managed.ControllerName(v1.UserGroupKind)
    
    r := managed.NewReconciler(mgr,
        resource.ManagedKind(v1.UserGroupVersionKind),
        managed.WithExternalConnecter(&connector{
            kube:         mgr.GetClient(),
            usage:        resource.NewProviderConfigUsageTracker(mgr.GetClient(), &v1beta1.ProviderConfigUsage{}),
            newServiceFn: clients.NewBackblazeClient,
        }),
        managed.WithLogger(o.Logger.WithValues("controller", name)),
        managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
    )
    
    return ctrl.NewControllerManagedBy(mgr).
        Named(name).
        For(&v1.User{}).
        Complete(r)
}
```

### 3. Implement External Client

Add methods to `internal/clients/backblaze.go` for B2 API operations:

```go
// User management methods
func (c *BackblazeClient) CreateApplicationKey(keyName string, capabilities []string, bucketID, namePrefix *string) (*ApplicationKey, error) {
    // Implement B2 API call to create application key
}

func (c *BackblazeClient) ListApplicationKeys() ([]ApplicationKey, error) {
    // Implement B2 API call to list application keys
}

func (c *BackblazeClient) DeleteApplicationKey(keyID string) error {
    // Implement B2 API call to delete application key
}
```

### 4. Register Controller

Add to `internal/controller/controller.go`:

```go
func Setup(mgr ctrl.Manager, o controller.Options) error {
    for _, setup := range []func(ctrl.Manager, controller.Options) error{
        config.Setup,
        bucket.Setup,
        user.Setup,    // Add new controller
        policy.Setup,  // Future controller
    } {
        if err := setup(mgr, o); err != nil {
            return err
        }
    }
    return nil
}
```

### 5. Generate and Test

```bash
make generate
go test ./internal/controller/user
make run
```

## Debugging

### Enable Debug Logging

```bash
# When running locally
make run ARGS="--debug"

# In-cluster
kubectl edit deployment/provider-backblaze-*
# Add --debug to container args
```

### Common Issues

1. **CRD Installation Issues**
   ```bash
   # Reinstall CRDs
   kubectl delete crd buckets.bucket.backblaze.crossplane.io
   make install
   ```

2. **RBAC Issues**
   ```bash
   # Check provider service account permissions
   kubectl describe clusterrole provider-backblaze-*
   ```

3. **B2 API Client Issues**
   ```bash
   # Test B2 API connectivity
   b2 authorize-account $B2_APPLICATION_KEY_ID $B2_APPLICATION_KEY
   b2 list-buckets
   ```

4. **S3 Compatibility Issues**
   ```bash
   # Test S3-compatible endpoint
   aws s3 ls --endpoint-url=https://s3.us-west-001.backblazeb2.com
   ```

### Debugging Tools

```bash
# Watch provider logs
kubectl logs -f -n crossplane-system deployment/provider-backblaze-*

# Describe problematic resources
kubectl describe bucket.bucket.backblaze.crossplane.io my-bucket

# Check events
kubectl get events --field-selector involvedObject.name=my-bucket

# Enable verbose API logging
export B2_DEBUG=true
make run
```

### Backblaze B2 Specific Debugging

```bash
# Check B2 account info
b2 get-account-info

# List all buckets
b2 list-buckets

# Check bucket details
b2 get-bucket $BUCKET_NAME

# Test S3 compatibility
aws configure set aws_access_key_id $B2_APPLICATION_KEY_ID
aws configure set aws_secret_access_key $B2_APPLICATION_KEY
aws s3 ls --endpoint-url=https://s3.$B2_REGION.backblazeb2.com
```

## Release Process

### 1. Update Version

```bash
# Update version in:
# - Makefile
# - package/crossplane.yaml
# - examples/provider/install.yaml
```

### 2. Generate Artifacts

```bash
make generate
make build
make xpkg.build
```

### 3. Run Tests

```bash
make test
make integration-test  # Requires B2 credentials
```

### 4. Tag Release

```bash
git tag v0.5.0
git push origin v0.5.0
```

### 5. Build and Push Images

```bash
make docker-build docker-push REGISTRY=ghcr.io/rossigee
make xpkg.build xpkg.push REGISTRY=ghcr.io/rossigee
```

### 6. Create GitHub Release

Include:
- Changelog
- Breaking changes
- Migration guide (if applicable)
- B2 compatibility notes

## Code Style

### Go Guidelines

- Follow [Effective Go](https://golang.org/doc/effective_go.html)
- Use `gofmt` and `goimports`
- Add comments for exported types and functions
- Keep functions focused and small
- Handle errors explicitly

### Commit Messages

Follow conventional commits:
- `feat:` New features
- `fix:` Bug fixes
- `docs:` Documentation changes
- `test:` Test additions/changes
- `refactor:` Code refactoring
- `chore:` Build process, dependencies

### Pull Request Process

1. Fork the repository
2. Create a feature branch
3. Make changes and add tests
4. Run `make reviewable` (formats, tests, builds)
5. Submit PR with clear description
6. Address review feedback

## Backblaze B2 Specific Development

### B2 API Integration

- Use official B2 API for user/key management
- Use S3-compatible API for bucket operations
- Handle B2-specific authentication patterns
- Implement proper rate limiting

### S3 Compatibility

- Set `S3ForcePathStyle: true` for AWS SDK
- Use correct regional endpoints
- Handle B2-specific bucket naming rules
- Support B2 bucket types (allPrivate, allPublic, etc.)

### Testing with B2

- Use test account with limited permissions
- Create test buckets with predictable names
- Clean up resources after tests
- Handle B2 API rate limits gracefully

### Resource Design

- Expose B2-specific features (lifecycle, CORS)
- Maintain S3 compatibility where possible
- Handle B2 naming restrictions
- Support B2-specific capabilities

## Useful Commands

```bash
# Format code
go fmt ./...

# Run linter
golangci-lint run

# Update dependencies
go mod tidy

# Verify module
go mod verify

# Clean build artifacts
make clean

# Full pre-commit check
make reviewable

# Test B2 connectivity
b2 authorize-account $B2_APPLICATION_KEY_ID $B2_APPLICATION_KEY

# Generate example manifests
make examples-generate

# Install examples
make examples-install

# Clean up examples
make examples-clean
```

## Architecture Notes

### Client Layer

The provider uses a dual-client approach:
- **S3 Client**: AWS SDK for bucket operations (create, delete, list)
- **B2 Native Client**: Direct B2 API for user/key management

### Resource Hierarchy

```
ProviderConfig (v1beta1)
├── Bucket (v1) - S3-compatible bucket management
├── User (v1) - B2 application key management
└── Policy (v1) - S3-compatible bucket policies
```

### Authentication Flow

1. Provider reads credentials from ProviderConfig secret
2. S3 client configured with B2 endpoint and credentials
3. B2 native client uses same credentials for API calls
4. Regional endpoint resolution handled automatically

### Error Handling

- B2 API errors mapped to Crossplane conditions
- S3 compatibility layer handles endpoint differences
- Retry logic implemented for transient failures
- Rate limiting respected for B2 API calls