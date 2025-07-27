# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Planned
- User resource for Backblaze B2 application key management
- Policy resource for S3-compatible bucket policies  
- Advanced bucket features (lifecycle rules, CORS, encryption)
- Integration tests with real Backblaze B2 environment
- Performance optimizations and caching

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
| Bucket CRUD Operations | ✅ Complete | Full lifecycle management |
| S3 API Compatibility | ✅ Complete | Works with existing S3 tools |
| Regional Endpoints | ✅ Complete | All B2 regions supported |
| Application Key Auth | ✅ Complete | B2-native authentication |
| Connection Secrets | ✅ Complete | Auto-generated for apps |
| User/Key Management | 🔄 Planned | Next major feature |
| Bucket Policies | 🔄 Planned | S3-compatible policies |
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

- User and Policy resources not yet implemented
- DeepCopy generation requires manual intervention for complex API tests
- Integration tests require manual Backblaze B2 account setup

## Acknowledgments

This provider builds on the excellent foundation provided by:
- The Crossplane community and runtime
- AWS SDK for Go S3 compatibility layer
- Standard Crossplane provider patterns from sibling providers
- Backblaze B2's S3-compatible API design