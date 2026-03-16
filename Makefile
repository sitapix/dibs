.PHONY: build test clean lint setup install fmt coverage

VERSION ?= dev

build:
	go build -ldflags="-s -w -X main.version=$(VERSION)" -trimpath -o dibs .

test:
	go test -race ./... -v

clean:
	rm -f dibs coverage.out

lint:
	go vet ./...
	golangci-lint run ./...

setup:
	git config core.hooksPath .githooks
	@echo "Git hooks activated."

install:
	go install -ldflags="-s -w -X main.version=$(VERSION)" .

fmt:
	gofmt -w .

coverage:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
