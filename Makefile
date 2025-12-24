.PHONY: build clean test test-race test-coverage test-coverage-check test-fuzz bench install check ci fmt imports fumpt staticcheck revive vet errcheck nilaway gosec ineffassign deadcode govulncheck

# Build variables
BINARY_NAME=maestro-runner
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT?=$(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_DATE?=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS=-ldflags "-X github.com/devicelab-dev/maestro-runner/cmd.Version=${VERSION} -X github.com/devicelab-dev/maestro-runner/cmd.Commit=${COMMIT} -X github.com/devicelab-dev/maestro-runner/cmd.BuildDate=${BUILD_DATE}"

# Install directory
INSTALL_DIR=$(HOME)/.maestro

# Go commands
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOVET=$(GOCMD) vet
GOMOD=$(GOCMD) mod

# Build targets
all: build

build:
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) .
	@mkdir -p $(INSTALL_DIR)
	@cp $(BINARY_NAME) $(INSTALL_DIR)/
	@echo "Installed to $(INSTALL_DIR)/$(BINARY_NAME)"

build-linux:
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME)-linux-amd64 .

build-darwin:
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME)-darwin-arm64 .

build-windows:
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME)-windows-amd64.exe .

build-all: build-linux build-darwin build-windows

install:
	$(GOBUILD) $(LDFLAGS) -o $(GOPATH)/bin/$(BINARY_NAME) .

clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_NAME)-*

test:
	$(GOTEST) -v ./...

test-race:
	$(GOTEST) -v -race ./...

test-coverage:
	$(GOTEST) -v -race -coverprofile=coverage.out -covermode=atomic ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

test-coverage-check:
	$(GOTEST) -v -race -coverprofile=coverage.out -covermode=atomic ./...
	@go tool cover -func=coverage.out | grep total | awk '{print $$3}' | sed 's/%//' | \
		awk '{if ($$1 < 80) {print "Coverage " $$1 "% is below 80% threshold"; exit 1} else {print "Coverage " $$1 "% meets 80% threshold"}}'

test-fuzz:
	$(GOTEST) -v -fuzz=. -fuzztime=30s ./...

bench:
	$(GOTEST) -v -bench=. -benchmem ./...

# Code quality tools
fmt:
	gofmt -s -w .

imports:
	goimports -w .

fumpt:
	gofumpt -w .

vet:
	$(GOVET) ./...

staticcheck:
	staticcheck ./...

revive:
	revive ./...

errcheck:
	errcheck ./...

nilaway:
	nilaway ./...

gosec:
	gosec ./...

ineffassign:
	ineffassign ./...

deadcode:
	deadcode ./...

govulncheck:
	govulncheck ./...

# Quality check - run all checks (use test-race for race detection)
check: fmt imports fumpt vet staticcheck revive errcheck nilaway gosec ineffassign deadcode govulncheck test-race
	@echo "All checks passed!"

# Full CI check (includes coverage threshold)
ci: fmt imports fumpt vet staticcheck revive errcheck nilaway gosec ineffassign deadcode govulncheck test-coverage-check
	@echo "CI checks passed!"

deps:
	$(GOMOD) download
	$(GOMOD) tidy

# Install dev tools
dev-setup:
	go install golang.org/x/tools/cmd/goimports@latest
	go install mvdan.cc/gofumpt@latest
	go install honnef.co/go/tools/cmd/staticcheck@latest
	go install github.com/mgechev/revive@latest
	go install github.com/kisielk/errcheck@latest
	go install go.uber.org/nilaway/cmd/nilaway@latest
	go install github.com/securego/gosec/v2/cmd/gosec@latest
	go install github.com/gordonklaus/ineffassign@latest
	go install golang.org/x/tools/cmd/deadcode@latest
	go install golang.org/x/vuln/cmd/govulncheck@latest

# Run targets
run:
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) .
	./$(BINARY_NAME) $(ARGS)

validate:
	./$(BINARY_NAME) validate $(FLOW)

# Release
release: clean build-all
	@echo "Release builds created:"
	@ls -la $(BINARY_NAME)-*
