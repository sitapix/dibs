.PHONY: build test clean lint docs-lint setup install fmt coverage release-check

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

docs-lint:
	@command -v markdownlint-cli2 >/dev/null || { echo "install: brew install markdownlint-cli2"; exit 1; }
	markdownlint-cli2 "**/*.md" "!.github/**" "!release-notes.md" "!vendor/**"

setup:
	@prev=$$(git config --get core.hooksPath || true); \
	  if [ -n "$$prev" ] && [ "$$prev" != ".githooks" ]; then \
	    echo "warning: overriding existing core.hooksPath: $$prev"; \
	    echo "         to restore: git config core.hooksPath $$prev"; \
	  fi
	@git config core.hooksPath .githooks
	@echo "Git hooks activated (.githooks). Bypass per-commit with: git commit --no-verify"

install:
	go install -ldflags="-s -w -X main.version=$(VERSION)" .

fmt:
	gofmt -w .

coverage:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

# release-check: full pre-tag verification. Run before `git tag v*`.
# Stricter than the pre-push hook: full race tests (not -short), deadcode,
# govulncheck, gofmt, golangci-lint, go.mod tidy drift (non-destructive),
# and a reproducibility build that exercises the exact ldflags CI uses
# so version wiring can't silently break. Requires network access.
release-check:
	@echo "→ gofmt"
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "unformatted files:"; echo "$$out"; exit 1; fi
	@echo "→ go vet"
	@go vet ./...
	@echo "→ golangci-lint"
	@command -v golangci-lint >/dev/null || { echo "install: https://golangci-lint.run/usage/install/"; exit 1; }
	@golangci-lint run ./...
	@echo "→ markdownlint"
	@command -v markdownlint-cli2 >/dev/null || { echo "install: brew install markdownlint-cli2"; exit 1; }
	@markdownlint-cli2 "**/*.md" "!.github/**" "!release-notes.md" "!vendor/**"
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
	@go mod tidy -diff || { echo "go.mod or go.sum drifted — run 'go mod tidy' and commit the result"; exit 1; }
	@echo "→ reproducible release build"
	@tmp=$$(mktemp -d); trap 'rm -rf $$tmp' EXIT INT TERM; \
	  go build -ldflags="-s -w -X main.version=release-check" -trimpath -o $$tmp/dibs .; \
	  actual=$$($$tmp/dibs --version); \
	  expected="dibs release-check"; \
	  if [ "$$actual" != "$$expected" ]; then \
	    echo "version wiring broken: got '$$actual', want '$$expected'"; \
	    exit 1; \
	  fi
	@echo ""
	@echo "all release checks passed"
