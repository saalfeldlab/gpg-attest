.PHONY: build install docs

docs:
	swag init -g cmd/gpg-attest-server/main.go -o docs --parseDependency --parseInternal
	go mod tidy

build: docs
	go build -o ./build/gpg-attest-server ./cmd/gpg-attest-server

install: build
	install -m 755 ./build/gpg-attest-server $(HOME)/.local/bin/gpg-attest-server
