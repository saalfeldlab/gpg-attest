BINARY     := gpg-attest
BUILD_DIR  := ./build
CMD_PKG    := gpg-attest.org/client/cmd/gpg-attest
VERSION    ?= dev
LDFLAGS    := -ldflags "-X gpg-attest.org/client/internal/version.Version=$(VERSION)"

# Detect host OS (uname on Unix; fallback to "Windows" on Windows)
UNAME := $(shell uname -s 2>/dev/null || echo Windows)

.PHONY: build install install-linux install-macos uninstall-linux uninstall-macos pkg deb test cross vet fmt clean

build:
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) $(CMD_PKG)

install: build
ifeq ($(UNAME),Darwin)
	@./scripts/install-macos.sh $(BUILD_DIR)/$(BINARY)
else ifeq ($(UNAME),Linux)
	@./scripts/install-linux.sh $(BUILD_DIR)/$(BINARY)
else
	@echo "On Windows run: powershell -ExecutionPolicy Bypass -File scripts\install-windows.ps1"
endif

install-linux: build
	@./scripts/install-linux.sh $(BUILD_DIR)/$(BINARY)

install-macos: build
	@./scripts/install-macos.sh $(BUILD_DIR)/$(BINARY)

uninstall-linux:
	@./scripts/uninstall-linux.sh

uninstall-macos:
	@./scripts/uninstall-macos.sh

pkg: build
	@./scripts/build-pkg.sh $(BUILD_DIR)/$(BINARY) $(VERSION)

deb: build
	@./scripts/build-deb.sh $(BUILD_DIR)/$(BINARY) $(VERSION)

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

# Cross-compile for all supported platforms
cross:
	GOOS=linux   GOARCH=amd64  go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-linux-amd64   $(CMD_PKG)
	GOOS=linux   GOARCH=arm64  go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-linux-arm64   $(CMD_PKG)
	GOOS=darwin  GOARCH=amd64  go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-darwin-amd64  $(CMD_PKG)
	GOOS=darwin  GOARCH=arm64  go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-darwin-arm64  $(CMD_PKG)
	GOOS=windows GOARCH=amd64  go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-windows-amd64.exe $(CMD_PKG)
	GOOS=windows GOARCH=arm64  go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-windows-arm64.exe $(CMD_PKG)

clean:
	rm -rf $(BUILD_DIR)
