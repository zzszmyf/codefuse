# CodeFuse

> **grep for code, not text.**

[![CI](https://github.com/zzszmyf/codefuse/actions/workflows/ci.yml/badge.svg)](https://github.com/zzszmyf/codefuse/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/zzszmyf/codefuse)](https://goreportcard.com/report/github.com/zzszmyf/codefuse)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

CodeFuse indexes your codebase into a **thin symbol→location map**, then exposes it through a **drop-in `grep` replacement** — same flags, same output format, but instead of thousands of text matches you get exactly the symbol definitions you're looking for.

```bash
# Same command, radically different results:
grep "LLM" -rn .                     # → 6,596 text matches (comments, strings, imports...)
codefuse grep "LLM" .                # → 10 symbol definitions (the actual class and its instances)

# Or just:
ln -s $(which codefuse) /usr/local/bin/grep
grep "LLM" -rn .                     # Now grep returns symbol definitions
```

## Why

AI coding agents spend 40% of their tool calls on `grep` + `read_file` loops — grepping a name, reading 15 files to find the one that's actually the definition, then grepping again for callers.

CodeFuse replaces this loop with a **single call**: index tells you *where* to look, actual source code tells you *what's there*. Code is the single source of truth.

| | `grep` | CodeFuse |
|---|---|---|
| What it finds | Text patterns anywhere | Symbol definitions |
| Noise level | Thousands of hits | 10–500× fewer hits |
| Speed | Fast for text, slow to verify | Fast to locate, fast to verify |
| Truth source | Direct file read | Index → file read (always current) |
| Agent tool calls | 5–15 per search | 1–2 per search |

## Quick Start

```bash
# 1. Install
go install github.com/yifanmeng/codefuse/cmd/codefuse@latest

# 2. Set up tree-sitter (one-time)
codefuse setup treesitter --auto

# 3. Index your project
codefuse index ./my-project
# → Indexed 2426 files, 28293 symbols, 13359 edges in 30s

# 4. Query — grep-compatible
codefuse grep "AuthService" .             # find definitions
codefuse grep -n "execute_model" src/     # with line numbers
codefuse grep -l "Scheduler" .            # files only

# 5. Query — structured (for deeper exploration)
codefuse query AuthService                # symbol details
codefuse query AuthService --callers      # who calls me
codefuse query AuthService --callees      # who do I call
codefuse outline src/main.go              # file structure

# 6. Real-time index updates
codefuse watch                            # watches files, auto-updates index
```

## Benchmark

### vllm — Python, 2,426 files, 28K symbols

Index: `3.5 MB` (nodes only), built in `~30s`.

| Query | CodeFuse | grep | cf hits | grep hits | Noise reduction |
|---|---|---|---|---|---|
| `flash_attn` | **73ms** | 59ms | **16** | 216 | **13×** |
| `Scheduler` | **66ms** | 60ms | **2** | 512 | **256×** |
| `ModelRunner` | **67ms** | 61ms | **12** | 244 | **20×** |
| `tensor_parallel` | **70ms** | 60ms | **9** | 467 | **52×** |
| `SamplingParams` | **69ms** | 59ms | **1** | 664 | **664×** |

### dubbo — Java, 4,048 files, 45K symbols

Index: `11 MB` (nodes only), built in `~15s`.

| Query | CodeFuse | grep | cf hits | grep hits | Noise reduction |
|---|---|---|---|---|---|
| `ServiceDiscovery` | **66ms** | 340ms | **13** | 532 | **41×** |
| `LoadBalance` | **71ms** | 163ms | **6** | 212 | **35×** |
| `ExtensionLoader` | **60ms** | 164ms | **6** | 886 | **148×** |
| `FilterChain` | **63ms** | 165ms | **2** | 99 | **50×** |
| `ClusterInvoker` | **64ms** | 164ms | **17** | 515 | **30×** |

**CodeFuse matches or beats grep on speed, while returning 10–600× fewer results — every result is a symbol definition, not a random text match.**

## grep Compatibility

CodeFuse implements the grep interface so AI agents don't need to change a single line of code:

| grep flag | Supported | Behavior |
|---|---|---|
| `pattern` | ✅ | Symbol name (exact → prefix → substring → glob) |
| `-r`, `-R` | ✅ | Recursive (default) |
| `-n` | ✅ | Line numbers |
| `-l` | ✅ | Files only |
| `-i` | ✅ | Case-insensitive |
| `-c` | ✅ | Count matches |
| `-w` | ✅ | Whole-word match |
| `-A`, `-B`, `-C N` | ✅ | Context lines |
| `-t`, `--text` | ✅ | Force real grep (skip index) |

**How it works:** when a `.codefuse/` index exists, `codefuse grep` queries the index for symbol definitions and reads actual source files for current line content. No index, or `--text` flag → falls through to the real `grep`. Zero configuration, always works.

## Supported Languages

8 languages. Each requires ~5 lines of configuration (tree-sitter node type names). No per-language Go code.

| Language | Extensions | Examples |
|---|---|---|
| **Go** | `.go` | func, method, type |
| **Python** | `.py` | def, class |
| **Java** | `.java` | method, class, interface, constructor |
| **TypeScript** | `.ts`, `.tsx` | function, class, interface, enum, type, method |
| **JavaScript** | `.js`, `.jsx` | function, class, method |
| **Rust** | `.rs` | fn, struct, enum, trait, impl |
| **C** | `.c`, `.h` | function |
| **C++** | `.cpp`, `.cc`, `.hpp` | function, class, struct |

**Adding a new language:** add an entry to `pkg/config/config.go`. No parser code needed.

## Architecture

```
CodeFuse — three layers, one job: find symbols fast.

┌─ Layer 1: Thin Index (~3 MB) ───────────────────┐
│  name → [(file, line, col), ...]                 │
│  Trie + gob binary, built once, loads in ~10ms   │
│  Stores POSITIONS only — never content            │
└──────────────────┬───────────────────────────────┘
                   │ where to look
                   ▼
┌─ Layer 2: Source Files ──────────────────────────┐
│  Read actual code at index positions              │
│  Code is the SINGLE source of truth               │
│  Index stale? No problem — you read current code  │
└──────────────────┬───────────────────────────────┘
                   │ current content
                   ▼
┌─ Layer 3: grep Interface ────────────────────────┐
│  file:line:content — identical to grep output    │
│  Same flags, same format, zero learning curve     │
│  No index? Falls back to real grep automatically  │
└──────────────────────────────────────────────────┘
```

## Commands

| Command | Description |
|---|---|
| `codefuse index [path]` | Build the thin symbol index |
| `codefuse grep <flags> <pattern> [path]` | grep-compatible symbol search |
| `codefuse query <symbol>` | Find symbol definitions |
| `codefuse query <symbol> --callers` | Show who calls this symbol |
| `codefuse query <symbol> --callees` | Show who this symbol calls |
| `codefuse outline <file>` | Show symbols in a file, sorted by line |
| `codefuse list` | Index summary and symbol distribution |
| `codefuse watch [path]` | Watch files and auto-update index |
| `codefuse setup treesitter --auto` | Install tree-sitter grammars |

## Replace grep

CodeFuse is designed to be a **transparent drop-in** — no configuration, no workflow changes, no new tool for AI agents to learn.

### Option 1: Symlink (recommended)

```bash
# Replace grep system-wide
ln -sf $(which codefuse) /usr/local/bin/grep

# Now every grep call uses CodeFuse under the hood:
grep "AuthService" -rn ./src
# → returns symbol definitions, not text matches
# → falls back to real grep when there's no index or --text is used
```

### Option 2: Alias

```bash
# In your shell config (.zshrc / .bashrc):
alias grep='codefuse grep'

# Or for AI agent sessions, set in the agent's environment:
export PATH="/path/to/codefuse-dir:$PATH"  # where a 'grep' symlink lives
```

### Option 3: AI agent configuration

Most AI coding agents call `grep` as a shell tool. No agent-side changes needed — just make sure `grep` resolves to CodeFuse:

```json
// Claude Code settings.json — no changes needed, just ensure PATH includes codefuse
// Cursor / Copilot — no changes, they use the system grep
```

If the agent allows custom tool definitions, you can also expose `codefuse grep` as a separate tool while keeping native `grep` for actual text searches:

```json
{
  "tools": {
    "grep": "codefuse grep",       // symbol search (index-powered)
    "greptext": "grep --text"      // text search (real grep fallback)
  }
}
```

### How the fallback works

```
User/Agent runs: grep "some_query" -rn ./

    Is there a .codefuse/ index?
    ├─ YES → Query index for symbol definitions
    │         ├─ Found → Output grep-format results (file:line:content)
    │         └─ Not found → Fall through to real grep
    │                        (captures text in comments, strings, etc.)
    └─ NO  → Fall through to real grep
              (behaves exactly like native grep)

    --text / -t flag → Skip index, always use real grep
```

**The result:** zero risk. If indexing isn't set up, or the symbol isn't in the index, or you pass `--text` — it's just grep.

### Per-project setup

```bash
# One-time setup per project:
cd my-project
codefuse setup treesitter --auto   # install grammars (once per machine)
codefuse index .                    # build index (15-30s)
codefuse watch &                    # optional: keep index live

# From now on, grep in this project uses the index:
grep "Scheduler" -rn .
# → 2 results (symbol definitions) instead of 512 (text matches)
```

## Design Principles

1. **Code is the only truth.** The index stores positions, never content. Every query reads actual source files.
2. **Language is configuration, not code.** New language = 5 lines of tree-sitter node types. No parser to write.
3. **Don't change the user.** Same flags as grep, same output format. Drop the binary in, rename it `grep`, done.
4. **Fast to rebuild.** Full re-index takes 15–30s. No need for complex incremental consistency guarantees.
5. **Safe by default.** No index? Falls back to real grep. Wrong result? Pass `--text` to bypass. Zero risk.

## License

[Apache-2.0](LICENSE)
