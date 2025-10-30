# Project Setup
PROJECT_NAME := provider-backblaze
PROJECT_REPO := github.com/rossigee/$(PROJECT_NAME)

PLATFORMS ?= linux_amd64 linux_arm64
-include build/makelib/common.mk

# Setup Output
-include build/makelib/output.mk

# Setup Go
# Use a modern golangci-lint version compatible with Go 1.25
GOLANGCILINT_VERSION ?= 2.5.0
NPROCS ?= 1
GO_TEST_PARALLEL := $(shell echo $$(( $(NPROCS) / 2 )))
GO_STATIC_PACKAGES = $(GO_PROJECT)/cmd/provider
GO_LDFLAGS += -X $(GO_PROJECT)/internal/version.Version=$(VERSION)
GO_SUBDIRS += cmd internal apis
GO111MODULE = on
-include build/makelib/golang.mk

# Setup Kubernetes tools
UP_VERSION = v0.28.0
UP_CHANNEL = stable
UPTEST_VERSION = v0.11.1
-include build/makelib/k8s_tools.mk

# Setup Images
IMAGES = provider-backblaze
# Force registry override (can be overridden by make command arguments)
REGISTRY_ORGS = ghcr.io/rossigee
-include build/makelib/imagelight.mk

# Setup XPKG - Standardized registry configuration
# Force registry override (can be overridden by make command arguments)
XPKG_REG_ORGS = ghcr.io/rossigee
XPKG_REG_ORGS_NO_PROMOTE = ghcr.io/rossigee

# Optional registries (can be enabled via environment variables)
# Harbor publishing has been removed - using only ghcr.io/rossigee
# To enable Upbound: export ENABLE_UPBOUND_PUBLISH=true make publish XPKG_REG_ORGS=xpkg.upbound.io/crossplane-contrib
XPKGS = provider-backblaze
-include build/makelib/xpkg.mk

# NOTE: we force image building to happen prior to xpkg build so that we ensure
# image is present in daemon.
xpkg.build.provider-backblaze: do.build.images

# Setup Package Metadata
CROSSPLANE_VERSION = 2.0.2
-include build/makelib/local.xpkg.mk
-include build/makelib/controlplane.mk

# Targets

# run `make submodules` after cloning the repository for the first time.
submodules:
	@git submodule sync
	@git submodule update --init --recursive

# NOTE: the build submodule currently overrides XDG_CACHE_HOME in order to
# force the Helm 3 to use the .work/helm directory. This causes Go on Linux
# machines to use that directory as the build cache as well. We should adjust
# this behavior in the build submodule because it is also causing Linux users
# to duplicate their build cache, but for now we just make it easier to identify
# its location in CI so that we cache between builds.
go.cachedir:
	@go env GOCACHE

# Go module cache directory for CI caching
go.mod.cachedir:
	@go env GOMODCACHE

# NOTE: we must ensure up is installed in tool cache prior to build as including the k8s_tools
# machinery prior to the xpkg machinery sets UP to point to tool cache.
build.init: $(UP)

# This is for running out-of-cluster locally, and is for convenience. Running
# this make target will print out the command which was used. For more control,
# try running the binary directly with different arguments.
run: go.build
	@$(INFO) Running Crossplane locally out-of-cluster . . .
	@# To see other arguments that can be provided, run the command with --help instead
	$(GO_OUT_DIR)/provider --debug

# NOTE: we ensure up is installed prior to running platform-specific packaging steps in xpkg.build.
xpkg.build: $(UP)

# Ensure CLI is available for package builds and publishing
$(foreach x,$(XPKGS),$(eval xpkg.build.$(x): $(CROSSPLANE_CLI)))

# Rules to build packages for each platform
$(foreach p,$(filter linux_%,$(PLATFORMS)),$(foreach x,$(XPKGS),$(eval $(XPKG_OUTPUT_DIR)/$(p)/$(x)-$(VERSION).xpkg: $(CROSSPLANE_CLI); @$(MAKE) xpkg.build.$(x) PLATFORM=$(p))))

# Ensure packages are built for all platforms before publishing
$(foreach r,$(XPKG_REG_ORGS),$(foreach x,$(XPKGS),$(eval xpkg.release.publish.$(r).$(x): $(CROSSPLANE_CLI) $(foreach p,$(filter linux_%,$(PLATFORMS)),$(XPKG_OUTPUT_DIR)/$(p)/$(x)-$(VERSION).xpkg))))

# Install CRDs into a cluster
install-crds: generate
	kubectl apply -f package/crds

# Uninstall CRDs from a cluster
uninstall-crds:
	kubectl delete -f package/crds

# Install examples into cluster
install-examples:
	kubectl apply -f examples/

# Delete examples from cluster
delete-examples:
	kubectl delete --ignore-not-found -f examples/

# Run integration tests (requires B2 credentials)
test-integration:
	@echo "Running integration tests against real Backblaze B2..."
	@if [ -z "$(B2_APPLICATION_KEY_ID)" ] || [ -z "$(B2_APPLICATION_KEY)" ]; then \
		echo "Error: B2_APPLICATION_KEY_ID and B2_APPLICATION_KEY environment variables must be set"; \
		exit 1; \
	fi
	go test -v ./test/integration/... -timeout 10m

# Run integration tests with cleanup disabled (for debugging)
test-integration-debug:
	@echo "Running integration tests with cleanup disabled..."
	@if [ -z "$(B2_APPLICATION_KEY_ID)" ] || [ -z "$(B2_APPLICATION_KEY)" ]; then \
		echo "Error: B2_APPLICATION_KEY_ID and B2_APPLICATION_KEY environment variables must be set"; \
		exit 1; \
	fi
	SKIP_CLEANUP=true go test -v ./test/integration/... -timeout 10m

# Run integration test benchmarks
test-integration-bench:
	@echo "Running integration test benchmarks..."
	@if [ -z "$(B2_APPLICATION_KEY_ID)" ] || [ -z "$(B2_APPLICATION_KEY)" ]; then \
		echo "Error: B2_APPLICATION_KEY_ID and B2_APPLICATION_KEY environment variables must be set"; \
		exit 1; \
	fi
	go test -v ./test/integration/... -bench=. -benchtime=10s -timeout 10m

.PHONY: submodules run install-crds uninstall-crds install-examples delete-examples test-integration test-integration-debug test-integration-bench