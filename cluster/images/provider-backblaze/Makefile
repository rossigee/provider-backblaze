# ====================================================================================
# Setup Project

PLATFORMS := linux_amd64 linux_arm64
include ../../../build/makelib/common.mk

# ====================================================================================
# Options
IMAGE ?= $(BUILD_REGISTRY)/provider-backblaze:$(VERSION)
TARGETOS := $(word 1, $(subst _, ,$(PLATFORM)))
include ../../../build/makelib/imagelight.mk

# ====================================================================================
# Image Build
img.build:
	@$(INFO) docker build $(IMAGE)
	@$(MAKE) img.build.shared
	@$(OK) docker build $(IMAGE)

img.build.shared:
	@cp Dockerfile $(IMAGE_TEMP_DIR)
	@cp -r $(OUTPUT_DIR)/bin/ $(IMAGE_TEMP_DIR)/
	@if echo "$(BUILD_ARGS)" | grep -q "platform.*linux/.*,.*linux/"; then \
		echo "Building multi-platform image with buildx"; \
		docker buildx build $(BUILD_ARGS) --build-arg TARGETOS=$(TARGETOS) --build-arg TARGETARCH=$(TARGETARCH) -t $(IMAGE) $(IMAGE_TEMP_DIR); \
	else \
		echo "Building single-platform image"; \
		docker build $(BUILD_ARGS) --build-arg TARGETOS=$(TARGETOS) --build-arg TARGETARCH=$(TARGETARCH) -t $(IMAGE) $(IMAGE_TEMP_DIR); \
	fi

# ====================================================================================
# Image Publish
img.publish: img.build
	@$(INFO) docker push $(IMAGE)
	@if echo "$(BUILD_ARGS)" | grep -q "platform.*linux/.*,.*linux/"; then \
		echo "Multi-platform image already pushed during build"; \
	else \
		docker push $(IMAGE); \
	fi
	@$(OK) docker push $(IMAGE)