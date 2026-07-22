# Contributing to CodeFuse

## Setup

```bash
git clone https://github.com/zzszmyf/codefuse.git
cd codefuse
npm install -g tree-sitter-cli
go run ./cmd/codefuse setup treesitter --auto
go build ./cmd/codefuse
```

## Workflow

```bash
make check       # format, vet, test
make cover       # coverage report
make build       # build binary
```

## Architecture

```
cmd/codefuse/     CLI + MCP server
pkg/config/       Language definitions (8 languages, 5 lines each)
pkg/types/        Node, Edge, Sink, Annotation, Graph
internal/scanner/ File system walker
internal/parser/  tree-sitter XML parsing + import/varMap regex
internal/index/   Graph builder, Trie, query engine
```

## Adding a language

1. Add entry to `pkg/config/config.go` (declaration + call node types)
2. Add grammar to `internal/parser/treesitter_setup.go`
3. (Optional) Add import parse patterns to `extractor.go`

Zero parser code. All languages share one extraction engine.

## Testing

```bash
go test ./...                           # all
go test ./internal/index/ -v            # specific
go test -race ./...                     # race detector
go test -coverprofile=c.out ./...       # coverage
```

## CI

- Ubuntu: Go 1.25/1.26 + tree-sitter + race detector
- macOS: Go 1.25 + race detector
- Lint: gofmt + go vet + golangci-lint
- Release: GoReleaser (linux/darwin/windows, amd64/arm64)
