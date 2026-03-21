.PHONY: build install

build:
	go build -o ./build/gpg-attest-server ./cmd/gpg-attest-server

install: build
	install -m 755 ./build/gpg-attest-server $(HOME)/.local/bin/gpg-attest-server
