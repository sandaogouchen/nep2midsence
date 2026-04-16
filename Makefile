.PHONY: build clean test vet

BINARY_NAME=casemover
VERSION=0.1.0
BUILD_DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

LDFLAGS=-ldflags "-X github.com/sandaogouchen/nep2midsence/internal/cli.Version=$(VERSION) \
	-X github.com/sandaogouchen/nep2midsence/internal/cli.BuildDate=$(BUILD_DATE) \
	-X github.com/sandaogouchen/nep2midsence/internal/cli.GitCommit=$(GIT_COMMIT)"

build:
	go build $(LDFLAGS) -o $(BINARY_NAME) ./cmd/casemover/

clean:
	rm -f $(BINARY_NAME)
	rm -rf .casemover/

test:
	go test ./...

vet:
	go vet ./...

lint:
	golangci-lint run

install:
	go install $(LDFLAGS) ./cmd/casemover/
