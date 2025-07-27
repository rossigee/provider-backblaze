# Set Shell to bash, otherwise some targets fail with dash/zsh etc.
SHELL := /bin/bash

# Disable built-in rules
MAKEFLAGS += --no-builtin-rules
MAKEFLAGS += --no-builtin-variables
.SUFFIXES:
.SECONDARY:
.DEFAULT_GOAL := help

# Project variables
PROJECT_NAME := provider-backblaze
PROJECT_REPO := github.com/rossigee/provider-backblaze

# Version and Image settings
VERSION ?= v0.1.0
IMG ?= ghcr.io/rossigee/provider-backblaze:$(VERSION)
IMG_LATEST ?= ghcr.io/rossigee/provider-backblaze:latest

# Binary settings
BIN_FILENAME := provider
BUILD_DIR := ./_output

# Go settings
GO_VERSION := 1.23
GO_LDFLAGS := -s -w
CGO_ENABLED := 0

# Tools
CONTROLLER_GEN_VERSION := v0.16.0
CONTROLLER_GEN := $(BUILD_DIR)/controller-gen

.PHONY: help
help: ## Show this help
	@grep -E -h '\s##\s' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.PHONY: all
all: build ## Build all artifacts

.PHONY: build
build: build-bin docker-build ## Build binary and container image

.PHONY: build-bin
build-bin: export CGO_ENABLED = 0
build-bin: fmt vet ## Build binary
	@mkdir -p $(BUILD_DIR)
	go build -ldflags "$(GO_LDFLAGS)" -o $(BUILD_DIR)/$(BIN_FILENAME) ./cmd/provider

.PHONY: test
test: test-unit ## Run all tests

.PHONY: test-unit
test-unit: ## Run unit tests
	go test -race -covermode atomic -coverprofile=coverage.out ./...

.PHONY: fmt
fmt: ## Run 'go fmt' against code
	go fmt ./...

.PHONY: vet
vet: ## Run 'go vet' against code
	go vet ./...

.PHONY: lint
lint: golangci-lint ## Run linters

.PHONY: golangci-lint
golangci-lint: ## Run golangci-lint
	@which golangci-lint > /dev/null || (echo "golangci-lint not found. Install it from https://golangci-lint.run/usage/install/" && exit 1)
	golangci-lint run --timeout 5m --out-format colored-line-number ./...

.PHONY: generate
generate: ## Generate code and manifests
	@go generate ./...

.PHONY: manifests
manifests: controller-gen ## Generate CRD manifests
	$(CONTROLLER_GEN) crd:generateEmbeddedObjectMeta=true paths="./apis/..." output:crd:artifacts:config=package/crds

.PHONY: docker-build
docker-build: ## Build container image
	docker build -t $(IMG) -f Dockerfile .
	docker tag $(IMG) $(IMG_LATEST)

.PHONY: docker-push
docker-push: ## Push container image
	docker push $(IMG)
	docker push $(IMG_LATEST)

.PHONY: docker-run
docker-run: docker-build ## Run container image locally
	docker run --rm -it $(IMG)

.PHONY: install
install: manifests ## Install CRDs into the K8s cluster specified in ~/.kube/config
	kubectl apply -f package/crds/

.PHONY: uninstall
uninstall: manifests ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config
	kubectl delete -f package/crds/

.PHONY: deploy
deploy: manifests ## Deploy controller to the K8s cluster specified in ~/.kube/config
	kubectl apply -f package/

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config
	kubectl delete -f package/

.PHONY: clean
clean: ## Clean build artifacts
	rm -rf $(BUILD_DIR)
	docker rmi $(IMG) $(IMG_LATEST) 2>/dev/null || true

.PHONY: mod-tidy
mod-tidy: ## Run go mod tidy
	go mod tidy

.PHONY: mod-verify
mod-verify: ## Run go mod verify
	go mod verify

.PHONY: xpkg-build
xpkg-build: ## Build Crossplane package
	@mkdir -p $(BUILD_DIR)
	kubectl crossplane build provider -f package/ -o $(BUILD_DIR)/$(PROJECT_NAME).xpkg

.PHONY: xpkg-push
xpkg-push: xpkg-build ## Push Crossplane package
	kubectl crossplane push provider $(BUILD_DIR)/$(PROJECT_NAME).xpkg $(IMG)

# Tool targets
.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary
$(CONTROLLER_GEN):
	@mkdir -p $(BUILD_DIR)
	GOBIN=$(abspath $(BUILD_DIR)) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION)

# Development targets
.PHONY: run
run: generate fmt vet ## Run against the configured Kubernetes cluster in ~/.kube/config
	go run ./cmd/provider

.PHONY: debug
debug: generate fmt vet ## Run with debug logging
	go run ./cmd/provider --debug

# CI/CD targets
.PHONY: ci-test
ci-test: test lint ## Run CI tests

.PHONY: ci-build
ci-build: build ## Run CI build

.PHONY: release
release: clean generate manifests test lint build docker-build docker-push xpkg-build ## Full release build

# Example targets
.PHONY: examples-install
examples-install: ## Install example resources
	kubectl apply -f examples/

.PHONY: examples-uninstall
examples-uninstall: ## Uninstall example resources
	kubectl delete -f examples/ --ignore-not-found=true