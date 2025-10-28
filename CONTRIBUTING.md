# Contributing to Provider Backblaze

We welcome contributions to the Backblaze provider for Crossplane! This document outlines the process for contributing and the standards we maintain.

## Development Setup

### Prerequisites

- Go 1.25.3 or later
- Docker
- Kubernetes cluster (kind, minikube, or similar for testing)
- kubectl configured to access your cluster

### Getting Started

1. Fork and clone the repository:
```bash
git clone https://github.com/yourusername/provider-backblaze.git
cd provider-backblaze
```

2. Install dependencies:
```bash
go mod download
```

3. Generate code and CRDs:
```bash
make generate
```

4. Build the provider:
```bash
make build
```

## Development Workflow

### Code Generation

This project uses extensive code generation for Kubernetes CRDs and related code. Always run code generation after modifying API types:

```bash
make generate
```

### Building

```bash
# Build binary
make build

# Build Docker image
make docker-build

# Build Crossplane package
make xpkg.build
```

### Testing

```bash
# Run unit tests
make test

# Run linting
make lint

# Run all CI checks
make ci-test
```

### Local Testing

1. Install CRDs to your cluster:
```bash
make install
```

2. Run the provider locally:
```bash
make run
```

3. In another terminal, apply example manifests:
```bash
kubectl apply -f examples/
```

## Code Standards

### Go Code Style

- Follow standard Go conventions
- Use `gofmt` for formatting
- Run `golangci-lint` for linting
- Maintain test coverage above 80%

### API Design

- Follow Kubernetes API conventions
- Use clear, descriptive field names
- Include comprehensive field documentation
- Validate all user inputs

### Controller Implementation

- Implement the standard Crossplane managed resource pattern
- Handle errors gracefully with appropriate status updates
- Use exponential backoff for retries
- Log important events and errors

## Commit Guidelines

### Commit Messages

Use conventional commit format:

```
<type>(<scope>): <description>

[optional body]

[optional footer(s)]
```

Types: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `chore`

Examples:
```
feat(bucket): add lifecycle rule support
fix(client): handle regional endpoint resolution
docs: update installation instructions
```

### Pull Requests

1. Create a feature branch from `master`
2. Make your changes with appropriate tests
3. Update documentation if needed
4. Ensure all CI checks pass
5. Submit a pull request with clear description

## Code Quality

### Pre-commit Hooks

We use pre-commit hooks to maintain code quality. Install them:

```bash
pip install pre-commit
pre-commit install
```

This will run:
- Go formatting and imports
- Unit tests for changed packages
- Security scanning
- YAML/JSON validation
- License header checking

### Required Checks

All pull requests must pass:
- ✅ All unit tests
- ✅ Linting (golangci-lint)
- ✅ Code generation is up to date
- ✅ Security scanning (no secrets)
- ✅ Documentation is updated

## Testing

### Unit Tests

- Write tests for all public functions
- Use table-driven tests where appropriate
- Mock external dependencies
- Test error conditions

### Integration Tests

For integration tests that require real Backblaze B2 credentials:

1. Set environment variables:
```bash
export B2_APPLICATION_KEY_ID="your-key-id"
export B2_APPLICATION_KEY="your-application-key"
export B2_REGION="us-west-001"
```

2. Run integration tests:
```bash
make test-integration
```

### Example Tests

Validate examples work correctly:
```bash
make examples-test
```

## Documentation

### API Documentation

- Document all API fields with godoc comments
- Include examples in field documentation
- Use clear, descriptive field names

### User Documentation

- Update README.md for user-facing changes
- Add examples for new features
- Update installation/configuration docs

### Developer Documentation

- Update CLAUDE.md for implementation notes
- Document architectural decisions
- Maintain troubleshooting guides

## Security

### Credential Management

- Never commit credentials to the repository
- Use Kubernetes secrets for sensitive data
- Support multiple credential sources
- Implement proper secret rotation

### Code Security

- Run security scans with pre-commit hooks
- Validate all user inputs
- Use secure defaults
- Follow OWASP guidelines

## Release Process

### Versioning

We follow semantic versioning (semver):
- **MAJOR**: Breaking API changes
- **MINOR**: New features, backward compatible
- **PATCH**: Bug fixes, backward compatible

### Release Steps

1. Update version in relevant files
2. Update CHANGELOG.md
3. Create release branch
4. Tag release: `git tag v0.5.1`
5. Push tag: `git push origin v0.5.1`
6. GitHub Actions will build and publish

## Getting Help

### Communication

- GitHub Discussions for questions and ideas
- GitHub Issues for bugs and feature requests
- Join the Crossplane Slack community

### Common Issues

1. **Code generation fails**: Run `make clean && make generate`
2. **Tests fail**: Ensure dependencies are up to date
3. **Docker build fails**: Check Go version and dependencies
4. **CRDs not installing**: Verify kubectl context and permissions

## Project Structure

```
provider-backblaze/
├── apis/                    # API type definitions
│   ├── bucket/v1/          # Bucket resource API
│   ├── user/v1/            # User resource API (planned)
│   └── policy/v1/          # Policy resource API (planned)
├── cmd/provider/           # Provider main entry point
├── config/                 # Provider configuration
├── examples/               # Example manifests
├── internal/               # Internal implementation
│   ├── clients/           # External API clients
│   └── controller/        # Resource controllers
├── package/               # Crossplane package definition
└── scripts/               # Development scripts
```

## Backblaze B2 Specific Guidelines

### API Client

- Use S3-compatible endpoints when possible
- Handle B2-specific authentication
- Implement proper error mapping
- Support all B2 regions

### Resource Design

- Expose B2-specific features where beneficial
- Maintain S3 compatibility for common operations
- Handle B2 naming conventions and restrictions
- Support B2-specific lifecycle features

### Testing with B2

- Use test buckets with predictable names
- Clean up resources after tests
- Handle rate limiting gracefully
- Test with different bucket types

## Code of Conduct

We follow the [Crossplane Code of Conduct](https://github.com/crossplane/crossplane/blob/master/CODE_OF_CONDUCT.md). Please treat all community members with respect.

## License

By contributing to this project, you agree that your contributions will be licensed under the Apache License 2.0.