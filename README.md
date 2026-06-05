# CodeFuse

CodeFuse is a **Code Virtual File System** for AI Agents. It indexes your codebase and exposes it as queryable views — symbols, file outlines, and a FUSE-mounted filesystem — so AI Agents can explore code using familiar operations (`ls`, `cat`, `find`, `glob`) instead of brittle text grepping.

## Why

Traditional AI coding agents rely on `grep` + `read_file` to explore codebases. This is:
- **Slow** — `grep` rescans the entire project every time
- **Imprecise** — regex matches names in comments, strings, and unrelated symbols
- **Verbose** — agents need 5–10 tool calls just to locate one function

CodeFuse solves this by **pre-indexing** the codebase and generating **virtual filesystem views** that agents can navigate directly.

## Installation

```bash
go install github.com/yifanmeng/codefuse/cmd/codefuse@latest
```

Or build from source:

```bash
git clone https://github.com/yifanmeng/codefuse.git
cd codefuse
go build -o codefuse ./cmd/codefuse
```

## Quick Start

```bash
# Index your project
codefuse index ./my-project

# Query a symbol (exact match)
codefuse query HelloWorld

# Query with glob pattern
codefuse query "use*"
codefuse query "*Handler"
codefuse query "get?User"

# Show file outline
codefuse outline src/main.go

# Generate VFS views for agent exploration
codefuse vfs generate

# List all indexed symbols
codefuse list

# Mount as FUSE filesystem (macOS needs macFUSE, Linux needs /dev/fuse)
codefuse mount /tmp/mymount
```

## Commands

| Command | Description |
|---------|-------------|
| `codefuse index <path>` | Index a codebase |
| `codefuse index <path> --treesitter` | Index with tree-sitter (higher accuracy) |
| `codefuse list` | List indexed files and symbol counts by kind |
| `codefuse query <symbol>` | Find symbol definitions (supports glob `*?[]`) |
| `codefuse query <symbol> -k function` | Filter by kind (function, class, method, etc.) |
| `codefuse outline <file>` | Show structured file outline |
| `codefuse vfs generate` | Generate `.codefuse/vfs/` views |
| `codefuse mount <mountpoint>` | Mount as FUSE filesystem |

## Indexing

### Default mode (fast)

Uses `go/ast` for Go (100% accurate) and regex fallback for other languages. Good for daily use.

```bash
codefuse index ./my-project
# Indexed 4258 files, 56406 symbols in ./my-project
```

### Tree-sitter mode (accurate)

Uses [tree-sitter](https://tree-sitter.github.io/tree-sitter/) CLI for precise AST-based symbol extraction. Slower first run but catches types, interfaces, enums, and arrow functions that regex misses.

**Prerequisites:**
1. Install tree-sitter CLI: `npm install -g tree-sitter-cli`
2. Install language grammars in a discoverable path (e.g. `~/github/tree-sitter-typescript`)

```bash
codefuse index ./my-project --treesitter
# Indexed 4258 files, 76411 symbols in ./my-project
```

### Incremental indexing

On subsequent runs, only changed files are re-parsed. The manifest at `.codefuse/manifest.json` tracks file mtimes.

## Querying Symbols

### Exact match

```bash
$ codefuse query HelloWorld
Found 2 result(s) for 'HelloWorld':

function HelloWorld
  File: src/greeter.go:14

class HelloWorld
  File: src/models.py:22
```

### Glob patterns

```bash
$ codefuse query "use*"
Found 42 result(s) for 'use*':
  function useAuth
    File: src/hooks/useAuth.ts:4
  function useDebounce
    File: src/hooks/useDebounce.ts:7

$ codefuse query "*Handler"
Found 8 result(s) for '*Handler':
  class RequestHandler
    File: src/handlers.py:10

$ codefuse query "get?User"
Found 3 result(s) for 'get?User':
  function getUser
    File: src/api.ts:12
  function getUsers
    File: src/api.ts:28
```

### Filter by kind

```bash
$ codefuse query "Api*" -k class
Found 1 result(s) for 'Api*':
  class ApiClient
    File: src/client.ts:5
```

## File Outlines

```bash
$ codefuse outline src/handlers/request.go
Outline: src/handlers/request.go

L007  variable    MaxRequestSize
L014  function    NewRequestHandler
L028  method      HandleGet
L045  method      HandlePost
L062  method      Validate
```

## VFS Views (for AI Agents)

After running `codefuse vfs generate`, your project gets a `.codefuse/vfs/` directory:

```
my-project/
├── src/
│   └── main.go
└── .codefuse/
    ├── index.json
    ├── manifest.json
    └── vfs/
        ├── symbols/          # One file per symbol
        │   ├── HelloWorld
        │   └── ApiClient
        ├── outline/          # One file per source file
        │   └── src_main.go
        └── references/       # Call graph (WIP)
```

Agents can now explore code without calling `codefuse` CLI repeatedly:

```bash
# List all symbols
ls .codefuse/vfs/symbols/

# Read a symbol's definition and locations
cat .codefuse/vfs/symbols/HelloWorld

# See file structure
cat .codefuse/vfs/outline/src_main.go
```

## FUSE Mount

Mount the index as a real filesystem for shell-native exploration:

```bash
# macOS: install macFUSE first (https://macfuse.io/)
# Linux: ensure /dev/fuse exists

mkdir -p /tmp/mymount
codefuse mount /tmp/mymount

# In another terminal:
ls /tmp/mymount/symbols/        # all symbol names
cat /tmp/mymount/symbols/main   # symbol details
ls /tmp/mymount/outline/        # all source files
cat /tmp/mymount/outline/src_main.go  # file outline

# Unmount
umount /tmp/mymount
```

## Supported Languages

| Language | Parser | Symbols |
|----------|--------|---------|
| Go | `go/ast` (stdlib) | package, func, method, struct, interface, const, var |
| TypeScript / TSX | tree-sitter / regex | function, class, interface, type, enum, method, variable |
| JavaScript / JSX | tree-sitter / regex | function, class, method, variable |
| Python | tree-sitter / regex | function, class, method |
| Rust | tree-sitter / regex | fn, struct, trait, impl, method |

Go uses the official `go/ast` parser (zero deps, 100% accurate). Other languages use tree-sitter CLI when `--treesitter` is passed, falling back to regex otherwise.

## Architecture

```
codefuse/
├── cmd/codefuse/          # CLI entrypoint (cobra)
├── internal/
│   ├── scanner/           # File system scanning (.gitignore, language detection)
│   ├── index/             # Symbol indexing + incremental manifest
│   ├── parser/            # Go AST parser, tree-sitter CLI wrapper, regex fallback
│   ├── vfs/               # Virtual filesystem view generation
│   └── fusefs/            # FUSE filesystem (go-fuse/v2)
└── pkg/types/             # Shared types
```

## Roadmap

- [x] Tree-sitter CLI batch parsing (`--treesitter`)
- [x] Glob pattern queries (`*?[]`)
- [x] Incremental indexing (mtime-based)
- [x] FUSE mount
- [ ] Cross-reference analysis (who calls whom)
- [ ] Semantic search (vector embeddings)
- [ ] Language server protocol (LSP) integration

## License

MIT
