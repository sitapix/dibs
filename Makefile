.PHONY: build test clean lint setup install fmt coverage release-check

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

# release-check: full pre-tag verification. Run before `git tag v*`.
# Stricter than the pre-push hook: full race tests (not -short), deadcode,
# govulncheck, go.mod tidy drift, and a reproducibility build that exercises
# the exact ldflags CI uses so version wiring can't silently break.
release-check:
	@echo "→ go vet"
	@go vet ./...
	@echo "→ go test -race (full)"
	@go test -race -count=1 ./...
	@echo "→ deadcode"
	@command -v deadcode >/dev/null || { echo "install: go install golang.org/x/tools/cmd/deadcode@latest"; exit 1; }
	@out=$$(deadcode ./...); if [ -n "$$out" ]; then echo "$$out"; exit 1; fi
	@echo "→ govulncheck"
	@command -v govulncheck >/dev/null || { echo "install: go install golang.org/x/vuln/cmd/govulncheck@latest"; exit 1; }
	@govulncheck ./...
	@echo "→ go mod verify"
	@go mod verify
	@echo "→ go mod tidy drift check"
	@go mod tidy
	@git diff --exit-code go.mod go.sum || { echo "go.mod or go.sum drifted during tidy — commit the result and re-run"; exit 1; }
	@echo "→ reproducible release build"
	@tmp=$$(mktemp -d); trap 'rm -rf $$tmp' EXIT; \
	  go build -ldflags="-s -w -X main.version=release-check" -trimpath -o $$tmp/dibs .; \
	  actual=$$($$tmp/dibs --version); \
	  expected="dibs release-check"; \
	  if [ "$$actual" != "$$expected" ]; then \
	    echo "version wiring broken: got '$$actual', want '$$expected'"; \
	    exit 1; \
	  fi
	@echo ""
	@echo "all release checks passed"
