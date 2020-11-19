SOURCES = $(wildcard pkg/*/*.go)

.PHONY: test
test:
	@go test ./... -coverprofile cover.out

.PHONY: test-coverage
test-coverage: cover.out
	@go tool cover -html=cover.out

cover.out: $(SOURCES)
	@go test ./... -coverprofile cover.out
