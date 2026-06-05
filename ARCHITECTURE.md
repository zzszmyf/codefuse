# Architecture

## Overview

CodeFuse is a CLI tool that indexes a codebase into a **symbol graph** (nodes = symbols, edges = call relationships) and exposes it through multiple interfaces:
- **CLI commands** (`query`, `outline`, `list`, `--callers`, `--callees`)
- **Physical VFS views** (`.codefuse/vfs/`)
- **FUSE mount** (real-time virtual filesystem)

## Data Flow

```
┌─────────────┐     ┌─────────────────────────────┐     ┌──────────────────┐
│   Scanner   │────▶│   Parser (parallel workers) │────▶│  Graph Index     │
│  (fs walk)  │     │  Go AST │ tree-sitter │ regex │     │  {Nodes,Edges}   │
└─────────────┘     └─────────────────────────────┘     └────────┬─────────┘
                                                                  │
                    ┌───────────────────────┬─────────────────────┼─────────────────────┐
                    │                       │                     │                     │
                    ▼                       ▼                     ▼                     ▼
            ┌─────────────┐        ┌─────────────┐       ┌─────────────┐       ┌─────────────┐
            │  CLI Query  │        │  Trie Index │       │ VFS Generator│      │  FUSE Mount  │
            │  (find)     │        │  (prefix)   │       │ (.codefuse/) │      │  (go-fuse)   │
            └─────────────┘        └─────────────┘       └─────────────┘       └─────────────┘
```

## Components

### Scanner (`internal/scanner`)

Recursively walks the project directory, respecting `.gitignore` patterns. Produces `[]types.FileEntry` with:
- Relative and absolute paths
- Detected language (by extension)
- File modification time

### Parser (`internal/parser`) — Parallel Worker Pool

Three-tier parser strategy, executed in parallel across `runtime.NumCPU()` workers:

| Language | Strategy | Symbols | Call Graph | Accuracy | Speed |
|----------|----------|---------|------------|----------|-------|
| Go | `go/ast` (stdlib) | ✅ | ✅ Cross-package | 100% | Fast |
| TS/JS/Python/Rust | tree-sitter CLI | ✅ | ✅ Same-file | High | Medium |
| Fallback | Regex | ✅ | ❌ | Medium | Fast |

**Two-phase build**:
1. **Phase 1 (parallel)**: Extract all `Node`s (symbols) from every file
2. **Phase 2 (parallel)**: Extract all `Edge`s (call relationships) using the node index for cross-reference resolution

**Tree-sitter batching**: Files are grouped by language and parsed in batches (500 files per `tree-sitter parse` invocation) to minimize process startup overhead.

**Tree-sitter setup**: `codefuse setup treesitter --auto` detects project languages, clones grammar repos from GitHub, and builds WASM parsers.

### Index (`internal/index`) — Graph Model

The index is a `Graph{Nodes, Edges}` rather than a flat `Symbol[]`:

```go
type Node struct {
    ID        string  // "pkg.Type.Method" or "file:line:col"
    Name      string  // "Method"
    Kind      string  // function | method | struct | interface
    File      string
    Line      int
    Parent    string
    Signature string
    Docstring string
}

type Edge struct {
    From string  // caller Node ID
    To   string  // callee Node ID
    Kind string  // calls | contains | imports | implements
    File string
    Line int
}
```

**Runtime indexes** (built on load):
- `nodeByID` — O(1) ID lookup
- `nodesByName` — O(1) exact name lookup
- `edgesFrom` / `edgesTo` — O(1) caller/callee lookup
- `nameTrie` — O(m+k) prefix lookup (m = prefix length, k = results)

**Query routing** (`Graph.Query(name, kind)`):
- Exact match → `nodesByName` (O(1))
- Prefix `foo*` → `nameTrie.FindPrefix` (O(m+k), 8.8x faster than linear on 10K symbols)
- Glob `*bar`, `b?r` → linear scan with `path.Match`

**Persistence**:
- `graph.json` — v0.2+ Graph format
- `manifest.json` — tracks file mtimes + index format version for incremental rebuilds and version compatibility checks
- `LoadAny()` — auto-detects v0.1 `index.json` vs v0.2 `graph.json`, converts on-the-fly

### VFS Generator (`internal/vfs`)

Creates `.codefuse/vfs/` with three views:
- **`symbols/`**: One file per unique symbol name. Content shows all occurrences (kind, file, line, parent, signature).
- **`outline/`**: One file per source file. Content is a line-sorted symbol list.
- **`references/`**: One file per unique symbol name. Content shows **callers** (who calls this) and **callees** (who this calls) with file/line locations.

Agents can `cat`/`ls` these files directly without invoking the CLI.

### FUSE Filesystem (`internal/fusefs`)

Uses `go-fuse/v2` to mount a live view of the index:
- `/mountpoint/symbols/` — dynamic symbol files
- `/mountpoint/outline/` — dynamic outline files
- `/mountpoint/references/` — dynamic call graph files

Content is generated on-demand via `Read()`. Unmounts cleanly on `SIGINT`/`SIGTERM`.

## Design Decisions

### Why Graph{Nodes,Edges} instead of Symbol[]?

A flat `Symbol[]` can answer "where is X defined?" but cannot answer "who calls X?" or "what does X call?". The Graph model adds relationship edges with minimal overhead:
- **Go**: `go/ast` resolves `CallExpr.Fun` to package-qualified IDs, enabling precise cross-package call graphs
- **Other languages**: tree-sitter AST extracts `call_expression` nodes; heuristic matching links them to symbol definitions

The Graph model also future-proofs the index for additional edge types (imports, implements, contains).

### Why go/ast for Go?

The standard library `go/parser` + `go/ast` is 100% accurate, has zero external dependencies, and handles edge cases (method receivers, embedded types, generics) that regex cannot. It is the reference implementation for Go symbol extraction.

### Why tree-sitter CLI instead of CGO bindings?

`go-tree-sitter` (CGO) requires a C compiler and tree-sitter library on every target platform, complicating cross-compilation. Calling the tree-sitter CLI as an external process avoids CGO entirely, at the cost of batching complexity.

### Why a trie for prefix queries?

`FindSymbolGlob("use*")` on 76K symbols does a linear scan. A prefix trie reduces this to O(m+k) where m is prefix length and k is result count. Benchmark: ~8.8x faster on 10K symbols (41μs vs 367μs).

### Why "Path as Query"?

AI agents already understand `ls`, `cat`, and `find`. Instead of designing a new query API, CodeFuse creates synthetic files that agents read naturally. This eliminates API learning friction. The addition of `references/` extends this philosophy: `cat .codefuse/vfs/references/authenticate` is the Agent-native way to ask "who calls authenticate?"

## Future Directions

- **MCP Server**: Expose CodeFuse as a Model Context Protocol server so Claude/Cursor/Codex can query it via standard tools
- **Semantic search**: Vector embeddings for natural language → symbol matching
- **Watch mode / daemon**: `fsnotify`-based incremental re-indexing
- **LSP integration**: Speak the Language Server Protocol for IDE compatibility
