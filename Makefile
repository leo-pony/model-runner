# Project variables
APP_NAME := model-runner
GO_VERSION := 1.23.7
LLAMA_SERVER_VERSION := latest
LLAMA_SERVER_VARIANT := cpu
BASE_IMAGE := ubuntu:24.04
VLLM_BASE_IMAGE := nvidia/cuda:12.9.0-runtime-ubuntu24.04
DOCKER_IMAGE := docker/model-runner:latest
DOCKER_IMAGE_VLLM := docker/model-runner:latest-vllm-cuda
DOCKER_TARGET ?= final-llamacpp
PORT := 8080
MODELS_PATH := $(shell pwd)/models-store
LLAMA_ARGS ?=
DOCKER_BUILD_ARGS := \
	--load \
	--platform linux/$(shell docker version --format '{{.Server.Arch}}') \
	--build-arg LLAMA_SERVER_VERSION=$(LLAMA_SERVER_VERSION) \
	--build-arg LLAMA_SERVER_VARIANT=$(LLAMA_SERVER_VARIANT) \
	--build-arg BASE_IMAGE=$(BASE_IMAGE) \
	--target $(DOCKER_TARGET) \
	-t $(DOCKER_IMAGE)

# Model distribution tool configuration
MDL_TOOL_NAME := model-distribution-tool
STORE_PATH ?= ./model-store
SOURCE ?=
TAG ?=
LICENSE ?=

# Main targets
.PHONY: build run clean test docker-build docker-build-multiplatform docker-run docker-build-vllm docker-run-vllm docker-run-impl help validate model-distribution-tool
# Default target
.DEFAULT_GOAL := build

# Build the Go application
build:
	CGO_ENABLED=1 go build -ldflags="-s -w" -o $(APP_NAME) ./main.go

# Build model-distribution-tool
model-distribution-tool:
	CGO_ENABLED=1 go build -ldflags="-s -w" -o $(MDL_TOOL_NAME) ./cmd/mdltool

# Run the application locally
run: build
	LLAMA_ARGS="$(LLAMA_ARGS)" \
	./$(APP_NAME)

# Clean build artifacts
clean:
	rm -f $(APP_NAME)
	rm -f $(MDL_TOOL_NAME)
	rm -f model-runner.sock
	rm -rf $(MODELS_PATH)

# Run tests
test:
	go test -v ./...

validate:
	find . -type f -name "*.sh" | xargs shellcheck

# Build Docker image
docker-build:
	HTTP_PROXY="http://172.17.0.1:7890" HTTPS_PROXY="http://172.17.0.1:7890" docker buildx build \
    --build-arg HTTP_PROXY="http://172.17.0.1:7890" \
    --build-arg HTTPS_PROXY="http://172.17.0.1:7890" \
	$(DOCKER_BUILD_ARGS) .

# Build multi-platform Docker image
docker-build-multiplatform:
	HTTP_PROXY="http://172.17.0.1:7890" HTTPS_PROXY="http://172.17.0.1:7890" docker buildx build \
	--progress=plain \
    --build-arg HTTP_PROXY="http://172.17.0.1:7890" \
    --build-arg HTTPS_PROXY="http://172.17.0.1:7890" \
	--platform linux/amd64,linux/arm64 $(DOCKER_BUILD_ARGS) .

# Run in Docker container with TCP port access and mounted model storage
docker-run: docker-build
	@$(MAKE) -s docker-run-impl

# Build vLLM Docker image
docker-build-vllm:
	@$(MAKE) docker-build \
		DOCKER_TARGET=final-vllm \
		DOCKER_IMAGE=$(DOCKER_IMAGE_VLLM) \
		LLAMA_SERVER_VARIANT=cuda \
		BASE_IMAGE=$(VLLM_BASE_IMAGE)

# Run vLLM Docker container with TCP port access and mounted model storage
docker-run-vllm: docker-build-vllm
	@$(MAKE) -s docker-run-impl DOCKER_IMAGE=$(DOCKER_IMAGE_VLLM)

# Common implementation for running Docker container
docker-run-impl:
	@echo ""
	@echo "Starting service on port $(PORT) with model storage at $(MODELS_PATH)..."
	@echo "Service will be available at: http://localhost:$(PORT)"
	@echo "Example usage: curl http://localhost:$(PORT)/models"
	@echo ""
	PORT="$(PORT)" \
	MODELS_PATH="$(MODELS_PATH)" \
	DOCKER_IMAGE="$(DOCKER_IMAGE)" \
	LLAMA_ARGS="$(LLAMA_ARGS)" \
	DMR_ORIGINS="$(DMR_ORIGINS)" \
	DO_NOT_TRACK="${DO_NOT_TRACK}" \
	DEBUG="${DEBUG}" \
	scripts/docker-run.sh

# Model distribution tool operations
mdl-pull: model-distribution-tool
	@echo "Pulling model from $(TAG)..."
	./$(MDL_TOOL_NAME) --store-path $(STORE_PATH) pull $(TAG)

mdl-package: model-distribution-tool
	@echo "Packaging model $(SOURCE) to $(TAG)..."
	./$(MDL_TOOL_NAME) package --tag $(TAG) $(if $(LICENSE),--licenses $(LICENSE)) $(SOURCE)

mdl-list: model-distribution-tool
	@echo "Listing models..."
	./$(MDL_TOOL_NAME) --store-path $(STORE_PATH) list

mdl-get: model-distribution-tool
	@echo "Getting model $(TAG)..."
	./$(MDL_TOOL_NAME) --store-path $(STORE_PATH) get $(TAG)

mdl-get-path: model-distribution-tool
	@echo "Getting path for model $(TAG)..."
	./$(MDL_TOOL_NAME) --store-path $(STORE_PATH) get-path $(TAG)

mdl-rm: model-distribution-tool
	@echo "Removing model $(TAG)..."
	./$(MDL_TOOL_NAME) --store-path $(STORE_PATH) rm $(TAG)

mdl-tag: model-distribution-tool
	@echo "Tagging model $(SOURCE) as $(TAG)..."
	./$(MDL_TOOL_NAME) --store-path $(STORE_PATH) tag $(SOURCE) $(TAG)

# Show help
help:
	@echo "Available targets:"
	@echo "  build				- Build the Go application"
	@echo "  model-distribution-tool	- Build the model distribution tool"
	@echo "  run				- Run the application locally"
	@echo "  clean				- Clean build artifacts"
	@echo "  test				- Run tests"
	@echo "  docker-build			- Build Docker image for current platform"
	@echo "  docker-build-multiplatform	- Build Docker image for multiple platforms"
	@echo "  docker-run			- Run in Docker container with TCP port access and mounted model storage"
	@echo "  docker-build-vllm		- Build vLLM Docker image"
	@echo "  docker-run-vllm		- Run vLLM Docker container"
	@echo "  help				- Show this help message"
	@echo ""
	@echo "Model distribution tool targets:"
	@echo "  mdl-pull			- Pull a model (TAG=registry/model:tag)"
	@echo "  mdl-package			- Package and push a model (SOURCE=path/to/model.gguf TAG=registry/model:tag LICENSE=path/to/license.txt)"
	@echo "  mdl-list			- List all models"
	@echo "  mdl-get			- Get model info (TAG=registry/model:tag)"
	@echo "  mdl-get-path			- Get model path (TAG=registry/model:tag)"
	@echo "  mdl-rm			- Remove a model (TAG=registry/model:tag)"
	@echo "  mdl-tag			- Tag a model (SOURCE=registry/model:tag TAG=registry/model:newtag)"
	@echo ""
	@echo "Backend configuration options:"
	@echo "  LLAMA_ARGS    - Arguments for llama.cpp (e.g., \"--verbose --jinja -ngl 999 --ctx-size 2048\")"
	@echo ""
	@echo "Example usage:"
	@echo "  make run LLAMA_ARGS=\"--verbose --jinja -ngl 999 --ctx-size 2048\""
	@echo "  make docker-run LLAMA_ARGS=\"--verbose --jinja -ngl 999 --threads 4 --ctx-size 2048\""
	@echo ""
	@echo "Model distribution tool examples:"
	@echo "  make mdl-pull TAG=registry.example.com/models/llama:v1.0"
	@echo "  make mdl-package SOURCE=./model.gguf TAG=registry.example.com/models/llama:v1.0 LICENSE=./license.txt"
	@echo "  make mdl-package SOURCE=./qwen2.5-3b-instruct TAG=registry.example.com/models/qwen:v1.0"
	@echo "  make mdl-list"
	@echo "  make mdl-rm TAG=registry.example.com/models/llama:v1.0"
