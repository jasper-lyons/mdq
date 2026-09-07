INSTALL_DIR := $(HOME)/.local/bin

.PHONY: build install test

build:
	go build -o mdq .

install: build
	mkdir -p $(INSTALL_DIR)
	install -m755 mdq $(INSTALL_DIR)/mdq

test:
	go test ./...
