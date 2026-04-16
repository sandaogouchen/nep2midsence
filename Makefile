.PHONY: build clean test vet lint install

BINARY_NAME=nep2midsence
VERSION=0.1.0
BUILD_DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Detect GOPATH/bin for install target
GOPATH_BIN=$(shell go env GOPATH)/bin

LDFLAGS=-ldflags "-X github.com/sandaogouchen/nep2midsence/internal/cli.Version=$(VERSION) \
	-X github.com/sandaogouchen/nep2midsence/internal/cli.BuildDate=$(BUILD_DATE) \
	-X github.com/sandaogouchen/nep2midsence/internal/cli.GitCommit=$(GIT_COMMIT)"

build:
	go build $(LDFLAGS) -o $(BINARY_NAME) ./cmd/nep2midsence/
	@echo ""
	@echo "✅ 构建成功: ./$(BINARY_NAME)"
	@echo "💡 运行方式:"
	@echo "   ./$(BINARY_NAME) --help"
	@echo "   或执行 'make install' 安装到 PATH"

clean:
	rm -f $(BINARY_NAME)
	rm -rf .nep2midsence/

test:
	go test ./...

vet:
	go vet ./...

lint:
	golangci-lint run

# Install to $GOPATH/bin (usually already in PATH)
install:
	go install $(LDFLAGS) ./cmd/nep2midsence/
	@echo ""
	@echo "✅ 已安装到: $(GOPATH_BIN)/$(BINARY_NAME)"
	@echo "💡 请确保 $(GOPATH_BIN) 在你的 PATH 中:"
	@echo "   export PATH=\"$$PATH:$(GOPATH_BIN)\""

# Install to /usr/local/bin (requires build first)
install-global: build
	cp $(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)
	@echo "✅ 已安装到: /usr/local/bin/$(BINARY_NAME)"
