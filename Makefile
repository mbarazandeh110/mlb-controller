# Defining variables
GO = go
BINARY_NAME = mlb-controller
BUILD_DIR = bin
SRC_DIR = ./cmd/mlb-controller
DOCKER_IMAGE = mlb-controller:latest
LDFLAGS = -ldflags "-s -w"
CGO_ENABLED ?= 0
GOOS ?= linux
GOARCH ?= amd64
LDFLAGS = -ldflags "-s -w -extldflags '-static'"
TAGS = -tags netgo
ENVTEST_K8S_VERSION = 1.30.0

# Default target
.PHONY: all
all: build

# Build the Go binary
.PHONY: build
build:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) \
	$(GO) build $(TAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(SRC_DIR)

# Run tests
.PHONY: test
test:
	$(GO) test -v ./...

# Run tests with coverage
.PHONY: test-cover
test-cover:
	$(GO) test ./... -coverprofile=coverage.out
	$(GO) tool cover -html=coverage.out

# Clean build artifacts
.PHONY: clean
clean:
	rm -rf $(BUILD_DIR)
	rm -f coverage.out

# Format code
.PHONY: fmt
fmt:
	$(GO) fmt ./...

# Run vet
.PHONY: vet
vet:
	$(GO) vet ./...

# Lint code
.PHONY: lint
lint:
	golangci-lint run

# Build Docker image
.PHONY: docker-build
docker-build:
	docker build -t $(DOCKER_IMAGE) .

# Run Docker container
.PHONY: docker-run
docker-run:
	docker run -it --rm -v $(PWD)/configs:/app/configs $(DOCKER_IMAGE)

# Remove Docker image
.PHONY: docker-clean
docker-clean:
	docker rmi $(DOCKER_IMAGE)

# Install dependencies
.PHONY: deps
deps:
	$(GO) mod tidy
	$(GO) mod download

# Run the application
.PHONY: run
run: build
	./$(BUILD_DIR)/$(BINARY_NAME)

# Build and run in development mode
.PHONY: dev
dev: fmt
	$(GO) run $(SRC_DIR)

.PHONY: test-integration
test-integration:
	KUBEBUILDER_ASSETS="$(shell setup-envtest use $(ENVTEST_K8S_VERSION) -p path)" $(GO) test -v -tags=integration ./internal/controller/...


# Help command to display available targets
.PHONY: help
help:
	@echo "Available targets:"
	@echo "  all          - Build the project (default)"
	@echo "  build        - Build the Go binary"
	@echo "  test         - Run tests"
	@echo "  test-cover   - Run tests with coverage"
	@echo "  clean        - Remove build artifacts"
	@echo "  fmt          - Format Go code"
	@echo "  vet          - Run Go vet"
	@echo "  lint         - Run golangci-lint"
	@echo "  docker-build - Build Docker image"
	@echo "  docker-run   - Run Docker container"
	@echo "  docker-clean - Remove Docker image"
	@echo "  deps         - Install dependencies"
	@echo "  run          - Run the built binary"
	@echo "  dev          - Run in development mode"
	@echo "  test-integration - Run integration tests"
