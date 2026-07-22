# CodeFuse

> **grep for code, not text.**

[![CI](https://github.com/zzszmyf/codefuse/actions/workflows/ci.yml/badge.svg)](https://github.com/zzszmyf/codefuse/actions)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

A **drop-in `grep` replacement** for AI coding agents. Same flags. Same output. But instead of searching text, it **understands code** — symbol definitions, call graphs, external dependencies, and transitive reachability. All from source. Zero hallucination.

```bash
# Before: grep finds text → 6,596 matches (comments, strings, noise)
grep "LLM" -rn .
# After: CodeFuse finds symbols → 1 result (the actual class definition)
ln -sf $(which codefuse) /usr/local/bin/grep
grep "LLM" -rn .
```

## Why

AI agents spend ~40% of tool calls on `grep` → `read_file` loops.  
CodeFuse compresses that to **one call**: a thin index locates symbols, actual source code provides the truth.

| | `grep` | CodeFuse |
|---|---|---|
| Finds | Text patterns anywhere | Symbol definitions + call graph |
| Noise | Thousands of hits | 10–600× fewer |
| Cross-file | No | Import-aware call edges |
| External deps | No | Auto-tagged sinks (sql, http, …) |
| Agent tool calls | 5–15 per search | 1–2 |
| Truth source | Direct file read | Index → file read (always current) |

## Quick Start

```bash
# 1. Install
go install github.com/yifanmeng/codefuse/cmd/codefuse@latest

# 2. Set up tree-sitter grammars (one-time per machine)
codefuse setup treesitter --auto

# 3. Index your project (~30s for 2,400 files)
codefuse index .

# 4. Use as grep — same flags, symbol results
codefuse grep "AuthService" .
codefuse grep -n -i "scheduler" src/

# 5. Explore callers / callees
codefuse query AuthService --callers

# 6. See external dependencies (auto-tagged by package name)
codefuse sinks
codefuse sinks AuthService
codefuse sinks --pkg sql

# 7. Ask "does method A reach the database?"
codefuse reachable AuthService --pkg sql

# 8. Watch for changes (auto-update index)
codefuse watch &

# 9. MCP server (for Claude Code, Cursor, etc.)
codefuse serve
```

## Benchmark

### vllm — Python 2,426 files · 28K symbols · 21K edges · 74K sinks

| Query | CodeFuse | grep | cf hits | grep hits | Reduction |
|---|---|---|---|---|---|
| `LLM` | 67ms | 161ms | **1** | 6,596 | **6,596×** |
| `flash_attn` | 76ms | 61ms | **16** | 216 | **13×** |
| `Scheduler` | 65ms | 63ms | **1** | 512 | **512×** |
| `ModelRunner` | 67ms | 64ms | **12** | 244 | **20×** |
| `SamplingParams` | 63ms | 63ms | **1** | 664 | **664×** |

### dubbo — Java 4,048 files · 45K symbols · 5.5K edges · 1.8K sinks

| Query | CodeFuse | grep | cf hits | grep hits | Reduction |
|---|---|---|---|---|---|
| `RegistryService` | 59ms | 222ms | **1** | 137 | **137×** |
| `ExtensionLoader` | 59ms | 172ms | **6** | 886 | **148×** |
| `ServiceConfig` | 60ms | 169ms | **35** | 624 | **18×** |

**Per-query speed competitive with grep; result quality 10–6,000× better.**

## How It Works

```
┌─────────────────────────────────────────────────────────────────┐
│ Layer 1: Thin Index (~5MB)                                      │
│   name → [(file, line, col), ...]                               │
│   Trie + gob binary. Built once. Positions only — no content.   │
├─────────────────────────────────────────────────────────────────┤
│ Layer 2: Import Resolution                                      │
│   Python: from X import Y   Java: import com.foo.Bar            │
│   Go: import "path"         Rust: use crate::foo                │
│   Builds FileImports + ModuleMap per file.                      │
├─────────────────────────────────────────────────────────────────┤
│ Layer 3: Type Inference (VarMap)                                │
│   dao = UserDao()      →  {"dao": "UserDao"}                   │
│   def f(dao: UserDao): →  {"dao": "UserDao"}                   │
│   List<UserDao> x = …  →  {"x": "UserDao"}                     │
├─────────────────────────────────────────────────────────────────┤
│ Layer 4: Cross-file Call Graph                                  │
│   dao.findById() → varMap["dao"]="UserDao" → import "UserDao"  │
│   → target file → findById ✅                                   │
├─────────────────────────────────────────────────────────────────┤
│ Layer 5: External Sinks                                         │
│   torch.tensor() → pkg=torch   sql.Query() → pkg=sql            │
│   http.Get() → pkg=http        os.ReadFile() → pkg=os           │
│   Auto-tagged by package name. Zero hardcoded categories.       │
├─────────────────────────────────────────────────────────────────┤
│ Layer 6: grep Interface                                         │
│   file:line:content — same output as grep                      │
│   No index? Falls back to real grep automatically.              │
└─────────────────────────────────────────────────────────────────┘
```

## grep Compatibility

Full grep flag coverage:

| Flag | Behavior |
|---|---|
| `pattern` | Symbol name (exact → CamelCase → substring → glob) |
| `-r`, `-R` | Recursive (default) |
| `-n` | Line numbers |
| `-l` | Files only |
| `-i` | Case-insensitive (trie + exact + substring) |
| `-c` | Count matches |
| `-w` | Whole-word match |
| `-v` | Invert match |
| `-o` | Only matching part |
| `-m N` | Max count |
| `-q` | Quiet (exit code only) |
| `-A N`, `-B N`, `-C N` | Context lines |
| `-t`, `--text` | Bypass index, use real grep |
| `--include=GLOB` | File pattern filter |
| `--exclude=GLOB` | File exclusion pattern |

## Commands

| Command | Description |
|---|---|
| `codefuse index [path]` | Build the thin symbol index |
| `codefuse grep <flags> <pattern> [path]` | Drop-in grep replacement |
| `codefuse query <symbol>` | Symbol definitions |
| `codefuse query <symbol> --callers` | Who calls this symbol |
| `codefuse query <symbol> --callees` | Who this symbol calls |
| `codefuse sinks` | All external calls, grouped by package |
| `codefuse sinks <symbol>` | External calls for a symbol |
| `codefuse sinks --pkg sql` | Filter by package name |
| `codefuse reachable <symbol> --pkg sql` | BFS path to matching sink |
| `codefuse outline <file>` | Symbols in a file, sorted by line |
| `codefuse list` | Index summary |
| `codefuse watch [path]` | Watch files, auto-update index |
| `codefuse serve [path]` | MCP server (stdio JSON-RPC) |
| `codefuse setup treesitter --auto` | Install tree-sitter grammars |

## Supported Languages

8 languages. Each is ~5 lines of **declaration node type names**. No per-language Go code.

| Language | Extensions | Declaration types |
|---|---|---|
| **Go** | `.go` | func, method, type |
| **Python** | `.py` | def, class |
| **Java** | `.java` | method, class, interface, constructor |
| **TypeScript** | `.ts`, `.tsx` | function, class, interface, enum, type, method |
| **JavaScript** | `.js`, `.jsx` | function, class, method |
| **Rust** | `.rs` | fn, struct, enum, trait, impl |
| **C** | `.c`, `.h` | function |
| **C++** | `.cpp`, `.cc`, `.hpp` | function, class, struct |

Import resolution: Python, Java, Go, Rust (regex-based).  
Type inference: Python (assignment, parameter, generic) + Java (declaration, generic).  
**New language:** add an entry to `pkg/config/config.go`. Zero parser code.

## Design Principles

1. **Code is the only truth.** Index stores positions, never content. Every query reads actual source files.
2. **Language is configuration, not code.** Declarations, calls, imports — just tree-sitter node type names.
3. **Don't change the user.** Same flags as grep. Same output. Drop the binary, rename it `grep`, done.
4. **Safe by default.** No index → falls back to real grep. `--text` → always real grep. Zero risk.
5. **Zero-config analysis.** Sinks auto-tagged by package name. No hardcoded "database" or "http" lists.

## License

[Apache-2.0](LICENSE)
