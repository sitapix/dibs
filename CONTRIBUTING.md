# Contributing to dibs

Thanks for your interest in contributing to dibs! This document provides guidelines and information for contributors.

## Code of Conduct

By participating in this project, you agree to abide by our [Code of Conduct](CODE_OF_CONDUCT.md).

## How to Contribute

### Reporting Bugs

Before submitting a bug report:

1. Check the [existing issues](https://github.com/sitapix/dibs/issues) to avoid duplicates
2. Make sure you're using the latest version

When submitting a bug report, include:

- Your operating system and version
- Go version (`go version`)
- Steps to reproduce the issue
- Expected vs actual behavior
- Any relevant error messages

### Suggesting Features

Feature requests are welcome! Please:

1. Check existing issues to see if it's already been suggested
2. Clearly describe the use case and expected behavior
3. Explain why this feature would be useful to other users

### Pull Requests

1. **Fork the repository** and create your branch from `main`
2. **Make your changes** following the coding standards below
3. **Add or update tests** for your changes
4. **Run tests** to make sure nothing is broken
5. **Submit a pull request** with a clear description

## Development Setup

```bash
# Clone your fork
git clone https://github.com/YOUR_USERNAME/dibs.git
cd dibs

# Set up pre-commit hooks
make setup

# Build
make build

# Run locally
./dibs --help
```

Requires Go 1.26+.

## Coding Standards

- Follow standard Go conventions (`gofmt`, `go vet`)
- Keep functions focused and small
- Handle errors explicitly; don't ignore them
- Add tests for new functionality
- Minimize external dependencies. dibs uses only the Go standard library plus `golang.org/x/net` (for the Public Suffix List); justify any new dependency in the PR description

### Commit Messages

- Use present tense ("Add feature" not "Added feature")
- Use imperative mood ("Fix bug" not "Fixes bug")
- Keep the first line under 72 characters
- Reference issues when applicable: "Fix DNS timeout (#123)"

Example:
```
Add CSV output format option

- Implement --csv flag for spreadsheet-compatible output
- Add header row with column names
- Update help text and README

Closes #42
```

## Testing

Before submitting a PR:

```bash
# Run all tests
make test

# Run tests with race detector
go test -race ./...

# Lint (requires golangci-lint)
make lint

# Build to verify compilation
make build
```

Before tagging a release, run the full pre-flight gate:

```bash
# Requires: go install golang.org/x/tools/cmd/deadcode@latest
#           go install golang.org/x/vuln/cmd/govulncheck@latest
make release-check
```

`release-check` adds deadcode detection, govulncheck, `go mod` drift check, and a reproducibility build that verifies the release ldflags still wire `main.version` correctly. Don't push a tag until it passes.

## Project Structure

```
dibs/
├── main.go            # CLI entry point and orchestration
├── fetch.go           # TLD list and RDAP bootstrap fetching/caching
├── pool.go            # Generic concurrent worker pool (fanOut)
├── config/            # Configuration loading and validation
├── dns/               # DoH and system DNS resolvers
├── output/            # Terminal, JSON, CSV renderers
├── rdap/              # RDAP verification client
├── tlds/              # TLD list parsing, caching, filtering
├── Formula/           # Homebrew formula
├── .github/workflows/ # CI and release automation
├── install.sh         # Binary installer script
├── Makefile           # Build, test, lint targets
├── README.md
├── CONTRIBUTING.md
└── LICENSE
```

## Release Process

Releases are managed by maintainers. The process:

1. Create a git tag: `git tag -a v1.x.x -m "Release v1.x.x"`
2. Push tag: `git push origin v1.x.x`
4. CI automatically builds binaries, creates a GitHub release, and updates the Homebrew formula

## Questions?

Feel free to open an issue for any questions about contributing.

Thank you for helping make dibs better!
