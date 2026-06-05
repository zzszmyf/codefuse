# Contributing to CodeFuse

Thank you for your interest in contributing! This document explains how to get started.

## Development Setup

```bash
# Requirements: Go 1.25+
git clone https://github.com/zzszmyf/codefuse.git
cd codefuse
go mod download
go build ./cmd/codefuse
```

## Running Tests

```bash
# All tests
go test ./...

# With race detector
go test -race ./...

# With coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Code Style

- `go fmt` — all Go code must be formatted
- `go vet ./...` — no vet warnings
- Follow existing patterns in the codebase

## Submitting Changes

1. **Open an issue first** for significant changes (new features, API changes)
2. **Fork and branch**: `git checkout -b feature/my-feature`
3. **Write tests** for new functionality
4. **Ensure CI passes**: `make test lint`
5. **Commit message format**: use present tense, be specific
   - Good: `feat: add glob pattern support to query command`
   - Bad: `fixed stuff`
6. **Open a Pull Request** with a clear description

## Pull Request Checklist

- [ ] Tests pass (`go test ./...`)
- [ ] Code is formatted (`go fmt`)
- [ ] No vet warnings (`go vet ./...`)
- [ ] Documentation updated (README, USAGE, or code comments)
- [ ] Commit messages are clear and descriptive

## Reporting Bugs

Use the [Bug Report](https://github.com/zzszmyf/codefuse/issues/new?template=bug_report.md) template. Include:

- CodeFuse version (`codefuse --version`)
- Go version (`go version`)
- Operating system
- Steps to reproduce
- Expected vs actual behavior

## Requesting Features

Use the [Feature Request](https://github.com/zzszmyf/codefuse/issues/new?template=feature_request.md) template. Describe:

- The problem you're trying to solve
- Your proposed solution
- Alternatives you've considered

## License

By contributing, you agree that your contributions will be licensed under the Apache-2.0 License.
