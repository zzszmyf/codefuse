# CodeFuse

> **grep for code, not text.**

[![CI](https://github.com/zzszmyf/codefuse/actions/workflows/ci.yml/badge.svg)](https://github.com/zzszmyf/codefuse/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/zzszmyf/codefuse)](https://goreportcard.com/report/github.com/zzszmyf/codefuse)
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
| Noise | Thousands of hits | 10–6,000× fewer |
| Cross-file | No | Import-aware + type inference |
| External deps | No | Auto-tagged sinks (sql, http, torch, …) |
| Agent tool calls | 5–15 per search | 1–2 |
| Truth source | Direct file read | Index → file read (always current) |

## Quick Start

```bash
go install github.com/yifanmeng/codefuse/cmd/codefuse@latest
codefuse setup treesitter --auto     # one-time per machine
codefuse index .                      # ~30s for 2,400 files

codefuse grep "AuthService" .         # drop-in grep replacement
codefuse query AuthService --callers  # who calls it
codefuse sinks                        # external dependencies by package
codefuse reachable AuthService --pkg sql  # does it reach the DB?
codefuse serve                        # MCP server for Claude Code / Cursor
codefuse watch &                      # live index updates
```

## Benchmark

### vllm — Python 2,426 files · 28K symbols · 21K edges · 74K sinks

| Query | CodeFuse | grep | cf hits | grep hits | Reduction |
|---|---|---|---|---|---|
| `LLM` | 67ms | 161ms | **1** | 6,596 | **6,596×** |
| `Scheduler` | 65ms | 63ms | **1** | 512 | **512×** |
| `flash_attn` | 76ms | 61ms | **16** | 216 | **13×** |
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
┌─ Layer 1: Thin Index (~5MB) ──────────────────────────┐
│  name → [(file, line, col), ...]  ·  Trie + gob       │
│  Positions only — never content                        │
├─ Layer 2: Import Resolution ───────────────────────────┤
│  Python/Java/Go/Rust import parsing ·  ModuleMap       │
├─ Layer 3: Type Inference (VarMap) ────────────────────┤
│  dao = UserDao() → {"dao": "UserDao"}                 │
│  List<UserDao> x → {"x": "UserDao"}                   │
├─ Layer 4: Cross-file Call Graph ──────────────────────┤
│  dao.findById() → varMap→import→file→findById ✅       │
├─ Layer 5: External Sinks ─────────────────────────────┤
│  torch.tensor()→pkg=torch  sql.Query()→pkg=sql        │
│  Auto-tagged. Zero hardcoded categories.               │
├─ Layer 6: grep Interface ─────────────────────────────┤
│  file:line:content — identical output                 │
│  No index? Falls back to real grep.                    │
└────────────────────────────────────────────────────────┘
```

## grep Compatibility

Full grep flag coverage:

| Flag | Behavior |
|---|---|
| `pattern` | Symbol name (exact → CamelCase → substring → glob) |
| `-r`, `-R` | Recursive (default) |
| `-n` | Line numbers |
| `-l` | Files only |
| `-i` | Case-insensitive |
| `-c` | Count matches |
| `-w` | Whole-word |
| `-v` | Invert match |
| `-o` | Only matching |
| `-m N` | Max count |
| `-q` | Quiet (exit code) |
| `-A/-B/-C N` | Context lines |
| `-t`, `--text` | Bypass index (real grep) |
| `--include/--exclude` | File patterns |

## Commands

| Command | Description |
|---|---|
| `codefuse index [path]` | Build thin symbol index |
| `codefuse grep <flags> <pattern> [path]` | Drop-in grep replacement |
| `codefuse query <symbol>` | Symbol definitions |
| `codefuse query <symbol> --callers` | Who calls this |
| `codefuse query <symbol> --callees` | Who this calls |
| `codefuse sinks` | External calls by package |
| `codefuse sinks <symbol>` | Sinks for a symbol |
| `codefuse sinks --pkg sql` | Filter by package |
| `codefuse reachable <sym> --pkg sql` | BFS to matching sink |
| `codefuse outline <file>` | File symbols by line |
| `codefuse list` | Index summary |
| `codefuse watch [path]` | Live index updates |
| `codefuse serve [path]` | MCP server (stdio JSON-RPC) |
| `codefuse setup treesitter --auto` | Install grammars |

## Supported Languages

8 languages. Each ~5 lines of tree-sitter node type names. No per-language Go code.

| Language | Extensions | Symbols | Imports | Type Inf. |
|---|---|---|---|---|
| **Go** | `.go` | func, method, type | ✅ | — |
| **Python** | `.py` | def, class | ✅ | ✅ |
| **Java** | `.java` | method, class, interface | ✅ | ✅ |
| **TypeScript** | `.ts`, `.tsx` | function, class, interface | — | — |
| **JavaScript** | `.js`, `.jsx` | function, class, method | — | — |
| **Rust** | `.rs` | fn, struct, enum, trait | ✅ | — |
| **C** | `.c`, `.h` | function | — | — |
| **C++** | `.cpp`, `.cc`, `.hpp` | function, class, struct | — | — |

## DevEx

```bash
make check       # gofmt + go vet + test
make pre-commit  # check + race detector
make fixtures    # regenerate XML test data
make bench-profile  # query performance profile
```

| Metric | Value |
|---|---|
| Test functions | 135 |
| Benchmark functions | 9 |
| Test/source ratio | 0.48 |
| Mean coverage | 74% |
| gofmt issues | 0 |
| go vet warnings | 0 |
| Build time | < 2s |
| Direct dependencies | 3 |

## Design Principles

1. **Code is the only truth.** Index stores positions, never content. Every query reads actual source files.
2. **Language is configuration, not code.** Declarations, calls, imports — just tree-sitter node type names.
3. **Don't change the user.** Same flags as grep. Same output. Drop the binary, rename it `grep`, done.
4. **Safe by default.** No index → falls back to real grep. `--text` → always real grep. Zero risk.
5. **Zero-config analysis.** Sinks auto-tagged by package name. No hardcoded "database" or "http" lists.

## License

[Apache-2.0](LICENSE)
