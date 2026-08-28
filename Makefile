BINARY  := gh-cli-extension
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/0xHackerSpace/gh-cli-extension/cmd.version=$(VERSION)

.DEFAULT_GOAL := check

## build: compile the extension binary at the repo root
.PHONY: build
build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) .

## test: run the test suite
.PHONY: test
test:
	go test ./...

## cover: run tests and open a coverage report
.PHONY: cover
cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

## fmt: format all Go sources
.PHONY: fmt
fmt:
	gofmt -w .

## lint: verify formatting and run go vet
.PHONY: lint
lint:
	@test -z "$$(gofmt -l .)" || { echo "unformatted files:"; gofmt -l .; exit 1; }
	go vet ./...

## check: lint + test (what CI runs)
.PHONY: check
check: lint test

## install: build and install this working copy as a local gh extension
.PHONY: install
install: build
	gh extension install . || gh extension upgrade cli-extension

## clean: remove build artifacts
.PHONY: clean
clean:
	rm -f $(BINARY) $(BINARY).exe coverage.out coverage.html
	rm -rf dist/

## help: list available targets
.PHONY: help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /'
