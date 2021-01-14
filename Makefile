SOURCES = $(wildcard pkg/*/*.go)
BUILD_DIR := build
GOLANGCI_LINT_VERSION = v1.35.2

.PHONY: test
test:
	@go test ./... -coverprofile cover.out

.PHONY: test-coverage
test-coverage: cover.out
	@go tool cover -html=cover.out

cover.out: $(SOURCES)
	@go test ./... -coverprofile cover.out

.PHONY: verify
verify: verify-go-lint

.PHONY: verify-go-lint
verify-go-lint: $(BUILD_DIR)/golangci-lint
	$(BUILD_DIR)/golangci-lint run --timeout=2m

$(BUILD_DIR)/golangci-lint:
	export \
		VERSION=$(GOLANGCI_LINT_VERSION) \
		URL=https://raw.githubusercontent.com/golangci/golangci-lint \
		BINDIR=$(BUILD_DIR) && \
	curl -sfL $$URL/$$VERSION/install.sh | sh -s $$VERSION
	$(BUILD_DIR)/golangci-lint version
	$(BUILD_DIR)/golangci-lint linters
