BINARY     := sig-stuff-native
BUILD_DIR  := ./build
CMD_PKG    := sig-stuff.dev/native/cmd/sig-stuff-native
VERSION    ?= dev
LDFLAGS    := -ldflags "-X sig-stuff.dev/native/internal/version.Version=$(VERSION)"

.PHONY: build install test cross vet fmt clean

build:
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) $(CMD_PKG)

install: build
	bash scripts/install-linux.sh $(BUILD_DIR)/$(BINARY)

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
