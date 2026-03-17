BINARY_NAME := aws-env
BIN_DIR := bin
TEST_ENDPOINT := http://127.0.0.1:5000
TEST_ENV := AWS_ACCESS_KEY_ID=test AWS_SECRET_ACCESS_KEY=test AWS_REGION=us-east-1 AWS_ENDPOINT_URL=$(TEST_ENDPOINT)
GOFLAGS := CGO_ENABLED=0
GOCACHE_DIR := /tmp/go-build

.PHONY: test build build-linux-amd64 build-linux-arm64 build-darwin-arm64 clean

test:
	@set -e; \
	docker compose up -d moto; \
	trap 'docker compose down' EXIT; \
	until curl -sf $(TEST_ENDPOINT)/moto-api/ >/dev/null; do sleep 1; done; \
	GOCACHE=$(GOCACHE_DIR) $(TEST_ENV) go test ./... -coverprofile=coverage.out -covermode=atomic; \
	GOCACHE=$(GOCACHE_DIR) go tool cover -func=coverage.out | tail -n 1

build: build-linux-amd64 build-linux-arm64 build-darwin-arm64

build-linux-amd64:
	@mkdir -p $(BIN_DIR)
	@GOCACHE=$(GOCACHE_DIR) GOOS=linux GOARCH=amd64 $(GOFLAGS) go build -o $(BIN_DIR)/$(BINARY_NAME)-linux-amd64 .

build-linux-arm64:
	@mkdir -p $(BIN_DIR)
	@GOCACHE=$(GOCACHE_DIR) GOOS=linux GOARCH=arm64 $(GOFLAGS) go build -o $(BIN_DIR)/$(BINARY_NAME)-linux-arm64 .

build-darwin-arm64:
	@mkdir -p $(BIN_DIR)
	@GOCACHE=$(GOCACHE_DIR) GOOS=darwin GOARCH=arm64 $(GOFLAGS) go build -o $(BIN_DIR)/$(BINARY_NAME)-darwin-arm64 .

clean:
	@rm -rf $(BIN_DIR) coverage.out
