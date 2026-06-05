# Architecture

## Overview

CodeFuse is a CLI tool that indexes a codebase and exposes it through multiple interfaces:
- **CLI commands** (`query`, `outline`, `list`)
- **Physical VFS views** (`.codefuse/vfs/`)
- **FUSE mount** (real-time virtual filesystem)

## Data Flow

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Scanner   │────▶│   Parser    │────▶│    Index    │
│  (fs walk)  │     │(AST/regex)  │     │ (JSON + mem)│
└─────────────┘     └─────────────┘     └──────┬──────┘
                                                │
                       ┌────────────────────────┼────────────────────────┐
                       │                        │                        │
                       ▼                        ▼                        ▼
               ┌─────────────┐         ┌─────────────┐         ┌─────────────┐
               │  CLI Query  │         │ VFS Generator│        │  FUSE Mount  │
               │  (find)     │         │ (.codefuse/) │        │  (go-fuse)   │
               └─────────────┘         └─────────────┘         └─────────────┘
```

## Components

### Scanner (`internal/scanner`)

Recursively walks the project directory, respecting `.gitignore` patterns. Produces `[]types.FileEntry` with:
- Relative and absolute paths
- Detected language (by extension)
- File modification time

### Parser (`internal/parser`)

Three-tier parser strategy:

| Language | Strategy | Accuracy | Speed |
|----------|----------|----------|-------|
| Go | `go/ast` (stdlib) | 100% | Fast |
| TS/JS/Python/Rust | tree-sitter CLI | High | Medium |
| Fallback | Regex | Medium | Fast |

**Tree-sitter batching**: Files are grouped by language and parsed in batches (500 files per `tree-sitter parse` invocation) to minimize process startup overhead.

### Index (`internal/index`)

- **Full build**: Parses all files, writes `index.json` + `manifest.json`
- **Incremental build**: Compares mtimes in `manifest.json`, only re-parses changed/deleted files
- **Symbol map**: Runtime `map[string][]types.Symbol` for O(1) name lookups
- **Glob queries**: `path.Match` for `*?[]` pattern support

### VFS Generator (`internal/vfs`)

Creates `.codefuse/vfs/` with two views:
- **`symbols/`**: One file per unique symbol name. Content shows all occurrences (kind, file, line, parent, signature).
- **`outline/`**: One file per source file. Content is a line-sorted symbol list.

Agents can `cat`/`ls` these files directly without invoking the CLI.

### FUSE Filesystem (`internal/fusefs`)

Uses `go-fuse/v2` to mount a live view of the index:
- `/mountpoint/symbols/` — dynamic symbol files
- `/mountpoint/outline/` — dynamic outline files

Content is generated on-demand via `Read()`. Unmounts cleanly on `SIGINT`/`SIGTERM`.

## Design Decisions

### Why go/ast for Go?

The standard library `go/parser` + `go/ast` is 100% accurate, has zero external dependencies, and handles edge cases (method receivers, embedded types, generics) that regex cannot. It is the reference implementation for Go symbol extraction.

### Why tree-sitter CLI instead of CGO bindings?

`go-tree-sitter` (CGO) requires a C compiler and tree-sitter library on every target platform, complicating cross-compilation. Calling the tree-sitter CLI as an external process avoids CGO entirely, at the cost of batching complexity.

### Why "Path as Query"?

AI agents already understand `ls`, `cat`, and `find`. Instead of designing a new query API, CodeFuse creates synthetic files that agents read naturally. This eliminates API learning friction.

## Future Directions

- **Cross-reference analysis**: Build a call graph for "find all callers of X"
- **Semantic search**: Vector embeddings for natural language → symbol matching
- **LSP integration**: Speak the Language Server Protocol for IDE compatibility
