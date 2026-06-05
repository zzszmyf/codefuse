# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2025-06-05

### Added

- Initial release of CodeFuse
- File system scanner with `.gitignore` support and language detection
- Symbol indexing with JSON persistence
- Go parser using `go/ast` (100% accurate, zero deps)
- Tree-sitter CLI batch parsing (`--treesitter` flag) with regex fallback
- Incremental indexing via `manifest.json` (mtime-based)
- Glob pattern queries (`*`, `?`, `[]`) in `query` command
- File outline command (`outline`)
- VFS generator (`vfs generate`) — `.codefuse/vfs/symbols/` and `.codefuse/vfs/outline/`
- FUSE mount (`mount`) using go-fuse/v2
- CLI commands: `index`, `list`, `query`, `outline`, `vfs generate`, `mount`
- Supported languages: Go, TypeScript/TSX, JavaScript/JSX, Python, Rust

[Unreleased]: https://github.com/zzszmyf/codefuse/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/zzszmyf/codefuse/releases/tag/v0.1.0
