# CodeFuse

[![CI](https://github.com/zzszmyf/codefuse/actions/workflows/ci.yml/badge.svg)](https://github.com/zzszmyf/codefuse/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/zzszmyf/codefuse)](https://goreportcard.com/report/github.com/zzszmyf/codefuse)
[![codecov](https://codecov.io/gh/zzszmyf/codefuse/branch/main/graph/badge.svg)](https://codecov.io/gh/zzszmyf/codefuse)
[![Release](https://img.shields.io/github/release/zzszmyf/codefuse.svg)](https://github.com/zzszmyf/codefuse/releases)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

CodeFuse is a **Code Virtual File System** for AI Agents. It indexes your codebase into a **graph of symbols and their relationships** (who calls whom), then exposes it as queryable views — symbols, file outlines, call graphs, and a FUSE-mounted filesystem — so AI Agents can explore code using familiar operations (`ls`, `cat`, `find`, `glob`) instead of brittle text grepping.

## Why

Traditional AI coding agents rely on `grep` + `read_file` to explore codebases. This is:
- **Slow** — `grep` rescans the entire project every time
- **Imprecise** — regex matches names in comments, strings, and unrelated symbols
- **Stateless** — no memory of "this function calls that function across files"
- **Verbose** — agents need 5–10 tool calls just to locate one function

CodeFuse solves this by **pre-indexing** the codebase into a **call graph** and generating **virtual filesystem views** that agents can navigate directly.

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
# Index your project (auto-detects Go, TS, JS, Python, Rust)
codefuse index ./my-project

# Query a symbol (exact match)
codefuse query HelloWorld

# Query with prefix wildcard — uses trie index, O(m+k) performance
codefuse query "use*"
codefuse query "*Handler"
codefuse query "get?User"

# Find who calls a function (cross-file call graph)
codefuse query Authenticate --callers

# Find who a function calls
codefuse query BuildGraph --callees

# Show file outline
codefuse outline src/main.go

# Generate VFS views for agent exploration
codefuse vfs generate

# List all indexed symbols
codefuse list

# Mount as FUSE filesystem (macOS needs macFUSE, Linux needs /dev/fuse)
codefuse mount /tmp/mymount

# Set up tree-sitter grammars for higher accuracy
codefuse setup treesitter --auto
```

## Commands

| Command | Description |
|---------|-------------|
| `codefuse index <path>` | Index a codebase into Graph{Nodes,Edges} |
| `codefuse index <path> --treesitter` | Index with tree-sitter (higher accuracy) |
| `codefuse list` | List indexed files, nodes, edges, and counts by kind |
| `codefuse query <symbol>` | Find symbol definitions (exact / prefix `*` / glob `*?[]`) |
| `codefuse query <symbol> --callers` | Show who calls this symbol (cross-file) |
| `codefuse query <symbol> --callees` | Show who this symbol calls |
| `codefuse query <symbol> -k function` | Filter by kind (func, class, method, etc.) |
| `codefuse outline <file>` | Show structured file outline |
| `codefuse vfs generate` | Generate `.codefuse/vfs/` views |
| `codefuse mount <mountpoint>` | Mount as FUSE filesystem |
| `codefuse setup treesitter` | Detect missing grammars and install them |

## Indexing

### Default mode (fast)

Uses `go/ast` for Go (100% accurate) and regex fallback for other languages. Good for daily use.

```bash
codefuse index ./my-project
# Indexed 4258 files, 56406 nodes, 48291 edges in ./my-project
```

### Tree-sitter mode (accurate)

Uses [tree-sitter](https://tree-sitter.github.io/tree-sitter/) CLI for precise AST-based symbol extraction and call graph analysis. Slower first run but catches types, interfaces, enums, arrow functions, and cross-file calls that regex misses.

**Prerequisites:**
1. Install tree-sitter CLI: `npm install -g tree-sitter-cli`
2. Set up grammars: `codefuse setup treesitter --auto`

```bash
codefuse index ./my-project --treesitter
# Indexed 4258 files, 76411 nodes, 91234 edges in ./my-project
```

### Incremental indexing

On subsequent runs, only changed files are re-parsed. The manifest at `.codefuse/manifest.json` tracks file mtimes and index format version.

## Querying Symbols

### Exact match

```bash
$ codefuse query HelloWorld
Found 2 result(s) for 'HelloWorld':

function HelloWorld
  ID:   main.HelloWorld
  File: src/greeter.go:14

class HelloWorld
  ID:   models.HelloWorld
  File: src/models.py:22
```

### Prefix & glob patterns

Prefix queries (`use*`) use a **trie index** for O(m+k) performance:

```bash
$ codefuse query "use*"
Found 42 result(s) for 'use*':
  function useAuth
    ID:   hooks.useAuth
    File: src/hooks/useAuth.ts:4
  function useDebounce
    ID:   hooks.useDebounce
    File: src/hooks/useDebounce.ts:7

$ codefuse query "*Handler"
Found 8 result(s) for '*Handler':
  class RequestHandler
    ID:   handlers.RequestHandler
    File: src/handlers.py:10

$ codefuse query "get?User"
Found 3 result(s) for 'get?User':
  function getUser
    ID:   api.getUser
    File: src/api.ts:12
  function getUsers
    ID:   api.getUsers
    File: src/api.ts:28
```

### Call graph analysis

```bash
$ codefuse query Authenticate --callers
function Authenticate
  ID:   auth.Authenticate
  File: auth/auth.go:15

  Callers (3):
    → Login (function) @ main/main.go:42
    → Session.Check (method) @ handlers/session.go:28
    → TestAuthenticate (function) @ auth/auth_test.go:10

$ codefuse query Login --callees
function Login
  ID:   main.Login
  File: main/main.go:40

  Callees (2):
    → Authenticate (function) @ main/main.go:42
    → CreateSession (function) @ main/main.go:45
```

### Filter by kind

```bash
$ codefuse query "Api*" -k class
Found 1 result(s) for 'Api*':
  class ApiClient
    ID:   client.ApiClient
    File: src/client.ts:5
```

## File Outlines

```bash
$ codefuse outline src/handlers/request.go
Outline: src/handlers/request.go

L007	variable	MaxRequestSize
L014	function	NewRequestHandler
L028	method	HandleGet
L045	method	HandlePost
L062	method	Validate
```

## VFS Views (for AI Agents)

After running `codefuse vfs generate`, your project gets a `.codefuse/vfs/` directory:

```
my-project/
├── src/
│   └── main.go
└── .codefuse/
    ├── graph.json          # v0.2 Graph{Nodes,Edges} index
    ├── manifest.json       # Version + file mtimes
    └── vfs/
        ├── symbols/          # One file per symbol
        │   ├── HelloWorld
        │   └── ApiClient
        ├── outline/          # One file per source file
        │   └── src_main.go
        └── references/       # Caller/callee per symbol
            ├── Authenticate
            └── Login
```

Agents can now explore code without calling `codefuse` CLI repeatedly:

```bash
# List all symbols
ls .codefuse/vfs/symbols/

# Read a symbol's definition and locations
cat .codefuse/vfs/symbols/HelloWorld

# See file structure
cat .codefuse/vfs/outline/src_main.go

# See who calls Authenticate (cross-file)
cat .codefuse/vfs/references/Authenticate
```

## FUSE Mount

Mount the index as a real filesystem for shell-native exploration:

```bash
# macOS: install macFUSE first (https://macfuse.io/)
# Linux: ensure /dev/fuse exists

mkdir -p /tmp/mymount
codefuse mount /tmp/mymount

# In another terminal:
ls /tmp/mymount/symbols/           # all symbol names
cat /tmp/mymount/symbols/main      # symbol details
ls /tmp/mymount/outline/           # all source files
cat /tmp/mymount/outline/src_main.go   # file outline
ls /tmp/mymount/references/        # call graph views
cat /tmp/mymount/references/Authenticate  # callers & callees

# Unmount
umount /tmp/mymount
```

## Supported Languages

| Language | Parser | Symbols | Call Graph |
|----------|--------|---------|------------|
| Go | `go/ast` (stdlib) | package, func, method, struct, interface, const, var | ✅ Cross-package via AST |
| TypeScript / TSX | tree-sitter / regex | function, class, interface, type, enum, method, variable | ✅ Same-file heuristic |
| JavaScript / JSX | tree-sitter / regex | function, class, method, variable | ✅ Same-file heuristic |
| Python | tree-sitter / regex | function, class, method | ✅ Same-file heuristic |
| Rust | tree-sitter / regex | fn, struct, trait, impl, method | ✅ Same-file heuristic |

Go uses the official `go/ast` parser (zero deps, 100% accurate) with full cross-package call graph resolution. Other languages use tree-sitter CLI when `--treesitter` is passed, falling back to regex otherwise. Call graphs for non-Go languages use heuristic same-file matching.

## Technical Roadmap

See [`ROADMAP_TECH.md`](ROADMAP_TECH.md) for versioned engineering milestones (v0.3 → v1.0).

## Architecture

```
codefuse/
├── cmd/codefuse/          # CLI entrypoint (cobra)
├── internal/
│   ├── scanner/           # File system scanning (.gitignore, language detection)
│   ├── index/             # Graph indexing (Nodes+Edges) + trie query + parallel build
│   ├── parser/            # Go AST parser, tree-sitter CLI wrapper, regex fallback, setup
│   ├── vfs/               # Virtual filesystem view generation (symbols/outline/references)
│   └── fusefs/            # FUSE filesystem (go-fuse/v2) with references/
└── pkg/types/             # Shared types: Node, Edge, Graph
```

## FAQ

### How is CodeFuse different from CodeGraph?

| | **CodeGraph** | **CodeFuse** |
|--|---------------|--------------|
| **Core output** | Code dependency graph (visualization) | Virtual filesystem + call graph |
| **Problem solved** | "Who does this class call? Any circular deps?" | "How does an Agent quickly find `useAuth`? Who calls it?" |
| **Interaction** | Generate static graph, human reads it | `ls` / `cat` / `codefuse query --callers` |
| **Primary user** | Human developers (refactoring, onboarding) | AI Agents + CLI users |
| **Tech core** | Graph data model (class→method→call edges) | Graph{Nodes,Edges} + VFS/FUSE mount + trie index |
| **Update model** | One-shot analysis | Incremental index, live mount |

**CodeGraph** draws a "code map" for you to understand architecture.  
**CodeFuse** turns code into a "searchable, traversable filesystem" for Agents — with call graph edges.

They complement each other — CodeFuse for fast symbol lookup + caller analysis, CodeGraph for architectural visualization.

## Roadmap

- [x] Graph data model (Node + Edge replaces Symbol[])
- [x] Cross-file call graph analysis (Go: precise, others: heuristic)
- [x] Trie-based prefix query optimization
- [x] Parallel index building (worker pool)
- [x] VFS `references/` call graph views
- [x] CLI `--callers` / `--callees`
- [x] Tree-sitter grammar auto-setup (`codefuse setup treesitter`)
- [x] Index format versioning + migration
- [ ] MCP (Model Context Protocol) server
- [ ] Semantic search (vector embeddings)
- [ ] Watch mode / daemon for live index updates
- [ ] Language server protocol (LSP) integration

## License

[Apache-2.0](LICENSE)
