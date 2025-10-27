# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Planned
- Advanced bucket features (lifecycle rules, CORS, encryption)
- Integration tests with real Backblaze B2 environment
- Performance optimizations and caching
- Terraform import compatibility
- Cross-region replication support

## [0.12.0] - 2025-10-27

### Changed
- Updated Go version to 1.25.3

## [0.9.1] - 2025-09-17

### 🚀 Major Release: Full Crossplane v2 Support

**This release adds complete Crossplane v2 support with dual-scope resource architecture.**

### Added
- **Full Crossplane v2 Support**: Both cluster-scoped (v1) and namespaced (v1beta1) APIs
- **User Controller**: Complete application key management with v1beta1 support
- **Policy Controller**: S3-compatible bucket policy management with v1beta1 support
- **Dual-Scope Architecture**: All resources support both v1 and v1beta1 versions
- **Namespace Isolation**: v1beta1 resources provide team-based multi-tenancy
- **Comprehensive Integration Tests**: Full test suite for v2 functionality
- **Migration Documentation**: Complete guide for v2 adoption in `docs/CROSSPLANE_V2_MIGRATION.md`
- **Validation Scripts**: `test/validate_v2_deployment.sh` for deployment verification

### API Resources (NEW)
- **User v1**: `user.backblaze.crossplane.io/v1` (cluster-scoped)
- **User v1beta1**: `user.backblaze.m.crossplane.io/v1beta1` (namespaced) ✨ **Recommended**
- **Policy v1**: `policy.backblaze.crossplane.io/v1` (cluster-scoped)
- **Policy v1beta1**: `policy.backblaze.m.crossplane.io/v1beta1` (namespaced) ✨ **Recommended**
- **Bucket v1beta1**: `bucket.backblaze.m.crossplane.io/v1beta1` (namespaced) ✨ **Recommended**

### Enhanced Features
- **Multi-Tenancy**: Namespace-level resource isolation and RBAC control
- **Secret Management**: Automatic application key secrets with namespace boundaries
- **Policy Management**: Both simple bucket policies and complex JSON policy documents
- **Application Keys**: Fine-grained capability-based permissions with bucket restrictions
- **Backward Compatibility**: Existing v1 resources continue working unchanged

### Infrastructure Updates
- **Go 1.25.1**: Updated to latest Go version
- **golangci-lint 2.4.0**: Latest linter version with modern Go support
- **CI/CD Workflows**: Updated with correct versions and comprehensive testing
- **Build System**: Enhanced with dual-scope CRD generation
- **Quality Gates**: All tests pass including new v2 functionality tests

### Documentation
- **README**: Complete v2 examples and migration information
- **Migration Guide**: Detailed Crossplane v2 migration with RBAC examples
- **Integration Tests**: Comprehensive test suite validating dual-scope functionality
- **API Documentation**: Updated with all v1beta1 resource specifications

## [0.5.1] - 2025-08-14

### Added
- Standardized CI/CD workflows following "CI Builds, Release Publishes" pattern
- Comprehensive security scanning with govulncheck, gosec, and CodeQL
- Build validation in CI without publishing (eliminates tag conflicts)
- Release workflow as single source of truth for all publishing
- Version and latest tags now point to identical images

### Changed
- CI workflow now only validates builds (no publishing)
- Release workflow handles all registry publishing
- Improved Docker build process with proper versioning
- Updated to Go 1.24.5 for consistency across workflows

### Fixed
- Eliminated registry tag conflicts between CI and Release workflows
- Standardized registry publishing to `ghcr.io/rossigee` only
- Resolved build system compatibility issues
- Fixed Makefile comments to reflect current registry strategy

### Infrastructure
- **Container Registry**: `ghcr.io/rossigee/provider-backblaze:v0.5.1`
- **CI/CD**: Standardized workflows matching other providers
- **Security**: Enhanced security scanning and SARIF uploads
- **Quality**: Parallel validation jobs for faster feedback

## [0.1.0] - 2025-07-27

### Added
- Initial release of provider-backblaze
- Bucket resource management with full CRUD operations
- S3-compatible API integration using AWS SDK
- Backblaze B2 regional endpoint support
- Comprehensive unit tests and build system
- GitHub Actions CI/CD pipeline
- Complete documentation, examples, and contributing guidelines
- Pre-commit hooks and code quality tools

### Features
- **Bucket Management**: Create, read, update, and delete Backblaze B2 buckets
- **S3 Compatibility**: Full compatibility with S3-compatible tools and workflows  
- **Regional Support**: Auto-configuration for all Backblaze B2 regions
- **Flexible Authentication**: Support for application key-based authentication
- **Deletion Policies**: Configurable bucket deletion behavior (empty vs force delete)
- **Status Reporting**: Rich status information with conditions and observations
- **Connection Secrets**: Automatic generation of connection details for applications

### API Resources
- **Bucket** (`bucket.backblaze.crossplane.io/v1`): Backblaze B2 bucket management
- **ProviderConfig** (`backblaze.crossplane.io/v1beta1`): Provider configuration and credentials

### Infrastructure  
- **Container Registry**: `ghcr.io/rossigee/provider-backblaze:v0.1.0`
- **Crossplane Package**: Standard `.xpkg` format for easy installation
- **Build System**: Make-based build with Docker and Crossplane package support
- **CI/CD**: GitHub Actions with linting, testing, and automated releases
- **Quality Gates**: Pre-commit hooks, security scanning, and code generation validation

### Architecture
- Built on Crossplane Runtime v1.18.0
- Uses AWS SDK v1.44.0 for S3-compatible operations
- Follows standard Crossplane provider patterns
- Path-style URL support for Backblaze B2 compatibility
- Regional endpoint auto-discovery and configuration

### Examples
- Basic bucket creation and management
- S3-compatible configuration examples
- Provider setup with credentials management
- Integration with existing S3 workflows

### Documentation
- Complete README with quickstart guide
- Development guide with local testing setup
- Contributing guidelines and code standards
- API reference documentation
- Troubleshooting and debugging guides

## Compatibility Matrix

| Feature | Status | Notes |
|---------|--------|-------|
| Bucket CRUD Operations | ✅ Complete | Full lifecycle management (v1 + v1beta1) |
| User/Key Management | ✅ Complete | Application key management (v1 + v1beta1) |
| Bucket Policies | ✅ Complete | S3-compatible policies (v1 + v1beta1) |
| S3 API Compatibility | ✅ Complete | Works with existing S3 tools |
| Regional Endpoints | ✅ Complete | All B2 regions supported |
| Application Key Auth | ✅ Complete | B2-native authentication |
| Connection Secrets | ✅ Complete | Auto-generated for apps |
| Crossplane v2 Support | ✅ Complete | Dual-scope with namespace isolation |
| Multi-Tenancy | ✅ Complete | Namespace-based resource isolation |
| RBAC Integration | ✅ Complete | Namespace-level permissions |
| Lifecycle Rules | 🔄 Planned | B2-specific lifecycle |
| CORS Configuration | 🔄 Planned | Web application support |
| Bucket Encryption | 🔄 Planned | B2 encryption features |

## Breaking Changes

None in this initial release.

## Migration Guide

This is the initial release, so no migration is needed. For users migrating from other S3-compatible providers:

1. Update provider configuration to use Backblaze B2 credentials
2. Change bucket references to use `bucket.backblaze.crossplane.io/v1`
3. Update regional endpoint configuration as needed
4. Test S3-compatible applications with new endpoints

## Known Issues

- Integration tests require manual Backblaze B2 account setup (environmental variables)
- Advanced B2-specific features (lifecycle, CORS, encryption) not yet implemented
- Cross-region replication capabilities pending B2 API support

## Acknowledgments

This provider builds on the excellent foundation provided by:
- The Crossplane community and runtime
- AWS SDK for Go S3 compatibility layer
- Standard Crossplane provider patterns from sibling providers
- Backblaze B2's S3-compatible API design