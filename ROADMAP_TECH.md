# Technical Roadmap

> Versioned technical milestones for CodeFuse. Complements the CEO roadmap with concrete engineering deliverables.

## Current State (v0.1.0)

```
Dependencies: 3 direct (cobra, go-fuse/v2, testify)
Lines of Go:   ~2,400
Core modules:
  scanner  - fs walk + .gitignore + language detection
  parser   - go/ast | tree-sitter CLI batch | regex fallback
  index    - JSON persistence + mtime manifest + symbol map
  vfs      - physical .codefuse/vfs/ (symbols/ + outline/)
  fusefs   - go-fuse/v2 live mount
  cli      - cobra commands
```

**Known gaps:**
- No cross-file call graph (vfs/generator.go has `TODO: implement actual reference analysis`)
- Tree-sitter grammar management is manual (user must install + configure parser directories)
- No programmatic API (only CLI)
- No watch/incremental daemon
- Index format is not versioned (breaking changes will corrupt old indexes)
- No embedding/semantic search

---

## v0.2.0 — "Understand" (1-2 months)

**Theme:** Close the core capability gaps that prevent CodeFuse from being a "code understanding" tool vs just a "symbol listing" tool.

### 1. Call Graph Analysis (P0)

**Problem:** Agent asks "who calls `authenticate`?" — codefuse cannot answer.

**Approach:**
- For Go: use `go/ast` + `types` package to resolve identifiers to their definitions, then build a caller→callee map
- For other languages: use tree-sitter query patterns (`(call_expression function: (identifier) @func)`) to extract call sites, then match symbol names heuristically
- Store as `references.json` alongside `index.json`

**New dependencies:**
- `golang.org/x/tools/go/packages` (for Go cross-package type resolution, optional enhancement)

**Files touched:**
- `internal/parser/goparser.go` — add `ExtractGoReferences()`
- `internal/parser/treesitter.go` — add tree-sitter query for call expressions
- `internal/index/references.go` — new package for reference graph
- `internal/vfs/generator.go` — remove TODO, implement `references/` view

**Verification:**
```bash
codefuse index .
codefuse query "authenticate" --callers
# or
cat .codefuse/vfs/references/authenticate
```

### 2. Automatic Tree-sitter Grammar Management (P1)

**Problem:** `--treesitter` fails silently if grammar is not installed. Users give up.

**Approach:**
- `codefuse setup treesitter` — interactive wizard that:
  1. Detects project languages
  2. Downloads pre-built WASM grammars from GitHub releases (tree-sitter-langs)
  3. Stores in `~/.config/codefuse/grammars/`
  4. Updates tree-sitter config.json parser-directories automatically
- Fallback: if WASM not available, clone grammar repo and `tree-sitter build --wasm`

**No new Go dependencies** — uses `os/exec` + `net/http` + `archive/zip`

**Files touched:**
- `cmd/codefuse/main.go` — add `setup` subcommand
- `internal/parser/treesitter_setup.go` — new file

### 3. Index Format Versioning (P1)

**Problem:** Adding call graph to index.json is a breaking change. Old indexes will crash or misread.

**Approach:**
- Add `"version": "2"` field to `index.json`
- `index.Load()` checks version, rejects incompatible versions with "re-index required" message
- Manifest tracks index format version alongside mtimes

**Files touched:**
- `pkg/types/types.go` — add `IndexVersion` constant
- `internal/index/index.go` — `Build()` writes version, `Load()` validates

### 4. Query Performance (P2)

**Problem:** `FindSymbolGlob("*")` on 76K symbols does a linear scan.

**Approach:**
- Build a `trie` or `radix tree` from symbol names for prefix/glob queries
- Benchmark target: `FindSymbol("use*")` < 1ms on 100K symbols
- Add `BenchmarkFindSymbol` / `BenchmarkFindSymbolGlob` to CI

**No new dependencies** — implement simple prefix trie in `internal/index/`

---

## v0.3.0 — "Agent-Native" (2-3 months)

**Theme:** Make CodeFuse the default code exploration backend for AI agents.

### 1. MCP (Model Context Protocol) Server (P0)

**Problem:** Every agent framework (Claude Code, Cursor, Codex CLI, OpenClaw) re-implements code exploration. CodeFuse should be the reusable backend.

**Approach:**
- Implement MCP server exposing resources + tools:
  - Resource: `codefuse://{project}/symbols/{name}` — symbol definition
  - Resource: `codefuse://{project}/outline/{file}` — file outline
  - Tool: `query_symbols` — glob search
  - Tool: `find_callers` — call graph
  - Tool: `index_project` — (re-)index
- Transport: stdio (default for MCP) + optional SSE

**New dependencies:**
- `github.com/mark3labs/mcp-go` (official Go MCP SDK)

**Files touched:**
- `cmd/codefuse/mcp.go` — new subcommand `codefuse mcp`
- `internal/mcp/` — new package

**Verification:**
```bash
codefuse mcp
# Claude Desktop config:
# {
#   "mcpServers": {
#     "codefuse": {
#       "command": "codefuse",
#       "args": ["mcp"]
#     }
#   }
# }
```

### 2. Semantic Search (P1)

**Problem:** Agent asks "where's the auth logic?" — exact/glob search fails if symbol names don't contain "auth".

**Approach:**
- Embed symbol names + docstrings using lightweight local model (e.g. `sentence-transformers/all-MiniLM-L6-v2` via ONNX Runtime or ollama)
- Store vectors in SQLite with `sqlite-vec` extension (pure Go, no CGO) or local HNSW index
- `codefuse query "auth logic" --semantic` — returns top-k similar symbols

**New dependencies:**
- `github.com/asg017/sqlite-vec` (vector search in SQLite, pure Go)
- OR implement simple HNSW in-memory index ourselves (no deps)

**Files touched:**
- `internal/index/semantic.go` — new package
- `cmd/codefuse/cmds.go` — add `--semantic` flag to query

### 3. Watch Mode / Daemon (P1)

**Problem:** User edits a file, but FUSE mount or VFS still shows stale data. Must manually re-run `codefuse index`.

**Approach:**
- `codefuse watch` — fsnotify-based daemon that:
  1. Watches source files for changes
  2. Incrementally re-parses only changed files
  3. Updates index + VFS + FUSE in real-time
- Use `github.com/fsnotify/fsnotify` (cross-platform, battle-tested)

**New dependencies:**
- `github.com/fsnotify/fsnotify`

**Files touched:**
- `cmd/codefuse/main.go` — add `watch` subcommand
- `internal/watch/` — new package
- `internal/index/incremental.go` — extract single-file re-parse logic

### 4. TypeScript Ecosystem Polish (P2)

**Problem:** TSX/JSX parsing via tree-sitter is incomplete (missing JSX-specific nodes, React hooks misclassified).

**Approach:**
- Add dedicated `ExtractTSSymbols()` using tree-sitter queries specific to `tsx` grammar
- Recognize React patterns: `useXxx` hooks, `forwardRef`, `memo`, `createContext`
- Support `export { foo } from './bar'` re-exports

**Files touched:**
- `internal/parser/treesitter.go` — add TSX-specific node handlers

---

## v0.4.0 — "Scale" (3-4 months)

**Theme:** Support the large codebases where codefuse adds the most value.

### 1. Multi-Project / Monorepo Index (P0)

**Problem:** A monorepo has 50 sub-projects. Indexing the entire thing takes 30 minutes. But Agent only works in 1-2 projects at a time.

**Approach:**
- `codefuse index --scope packages/auth` — index only a subdirectory
- `.codefuse/workspace.json` — define which directories belong to the workspace
- Cross-project symbol resolution: if `packages/auth` exports `AuthContext`, `packages/app` can resolve it

**Files touched:**
- `internal/index/workspace.go` — new package
- `cmd/codefuse/cmds.go` — add `--scope` flag

### 2. Remote Index Cache (P1)

**Problem:** CI/CD pipeline indexes the same large repo on every build. 30 minutes × every PR = massive waste.

**Approach:**
- `codefuse remote` protocol:
  - Server: `codefuse serve --addr :8080` — gRPC or HTTP/JSON API
  - Client: `codefuse index --remote https://codefuse.company.com` — fetch pre-built index, apply local delta
- Store indexes in S3-compatible object storage with content-addressable keys (sha256 of manifest)
- Incremental sync: client sends file mtimes, server returns what changed

**New dependencies:**
- `google.golang.org/grpc` OR keep it simple with `net/http` + JSON

**Files touched:**
- `cmd/codefuse/main.go` — add `serve` subcommand
- `internal/remote/` — new package (client + server)

### 3. Streaming for Large Files (P2)

**Problem:** Indexing a 10MB generated JSON or minified JS file hangs or OOMs.

**Approach:**
- Skip files > configurable size limit (default 1MB) with warning
- For large files: streaming parser (read in chunks, don't hold full AST in memory)
- Add `max_file_size` to `.codefuse/config.json`

**Files touched:**
- `internal/scanner/scanner.go` — add size check
- `internal/parser/` — add streaming mode

---

## v1.0.0 — "Production" (6 months)

**Theme:** Stable API, complete language coverage, enterprise-ready.

### 1. Full LSP Bridge (P0)

**Problem:** Tree-sitter supports 100+ languages but grammar management is painful. LSP servers already exist for every language.

**Approach:**
- `codefuse index --lsp` — use language server instead of tree-sitter for symbol extraction
- Implement LSP client: initialize server, send `textDocument/documentSymbol`, parse response
- Cache LSP server process (one per language, long-lived)

**New dependencies:**
- `github.com/sourcegraph/jsonrpc2` or hand-roll LSP JSON-RPC client

**Files touched:**
- `internal/parser/lsp.go` — new package
- `cmd/codefuse/main.go` — add `--lsp` flag

### 2. SDK / Library API (P1)

**Problem:** CodeFuse is CLI-only. Other Go tools cannot import it as a library.

**Approach:**
- Export stable API from `pkg/codefuse/`:
  ```go
  package codefuse
  func Open(projectPath string) (*Project, error)
  func (p *Project) Query(pattern string, opts QueryOpts) ([]Symbol, error)
  func (p *Project) Outline(filePath string) ([]Symbol, error)
  func (p *Project) Mount(mountpoint string) error
  ```
- Internal packages stay in `internal/`. Public API is thin wrapper + types.

**Files touched:**
- `pkg/codefuse/` — new package

### 3. Configuration System (P1)

**Problem:** Flags proliferate (`--treesitter`, `--lsp`, `--scope`, `--semantic`). Users need a config file.

**Approach:**
- `.codefuse/config.json` or `.codefuse/config.yaml`:
  ```yaml
  parser:
    go: ast
    typescript: treesitter
    default: lsp
  index:
    exclude:
      - "*.gen.go"
      - "node_modules/"
    max_file_size: 1MB
  query:
    default_mode: glob
    semantic:
      model: all-MiniLM-L6-v2
  ```

**Files touched:**
- `pkg/config/` — new package

---

## Dependency Budget Philosophy

| Tier | Policy | Examples |
|------|--------|----------|
| **Core** | Zero external deps | scanner, index, basic parser |
| **CLI** | Standard Go ecosystem | cobra, fsnotify |
| **Optional** | User opt-in only | mcp-go, sqlite-vec, grpc |
| **CGO** | Avoid at all costs | (no CGO dependencies) |

**Rule:** Every new direct dependency must have >1K stars, active maintenance, and a clear removal path.

---

## Testing Strategy by Version

| Version | Test Focus |
|---------|-----------|
| v0.2.0 | Benchmark suite (index speed, query latency, memory usage) |
| v0.3.0 | MCP conformance tests, fsnotify integration tests |
| v0.4.0 | Large repo stress tests (Linux kernel, Chromium), remote sync tests |
| v1.0.0 | API stability tests (ensure no breaking changes), fuzz testing |
