.DEFAULT_GOAL := build

BINARY := yls
PKG    := ./cmd/yls

.PHONY: build test lint fmt run clean all

all: build

build:
	go build -o $(BINARY) $(PKG)

test:
	go test -race ./...

lint:
	golangci-lint run

fmt:
	gofmt -w .
	goimports -w .

# Usage: make run FILE=secrets/production.enc.yaml
run:
	go run $(PKG) $(FILE)

clean:
	rm -f $(BINARY)
	rm -rf dist/
