# CodeFuse Usage Guide

## For AI Agents

CodeFuse is designed so AI agents can explore codebases with standard shell operations instead of repeated `grep` + `read_file` calls.

### Workflow

```bash
# 1. Index the project (one time, or after large changes)
codefuse index . --treesitter

# 2. Agent explores via VFS
cat .codefuse/vfs/symbols/AuthController
cat .codefuse/vfs/outline/src_controllers_auth.ts
cat .codefuse/vfs/references/authenticate  # who calls authenticate?
ls .codefuse/vfs/symbols/ | grep "^use"

# 3. Or use CLI for targeted queries
codefuse query "*Controller"
codefuse query authenticate --callers
codefuse outline src/services/user.ts
```

### Example Session

**Scenario**: Agent needs to understand how authentication works.

```bash
# Find auth-related symbols (prefix query uses trie index)
$ codefuse query "auth*"
Found 15 result(s) for 'auth*':
  function useAuth
    ID:   hooks.useAuth
    File: src/hooks/useAuth.ts:4
  class AuthController
    ID:   controllers.AuthController
    File: src/controllers/auth.ts:12
  function authenticate
    ID:   middleware.authenticate
    File: src/middleware/auth.ts:8

# Look at the controller outline
$ codefuse outline src/controllers/auth.ts
Outline: src/controllers/auth.ts
L012  class       AuthController
L018  method      login
L035  method      register
L052  method      refreshToken

# Read the hook implementation
$ cat .codefuse/vfs/symbols/useAuth
# Symbol: useAuth
## useAuth (function) @ src/hooks/useAuth.ts:4
- File: src/hooks/useAuth.ts:4

# Check who calls authenticate (cross-file call graph)
$ codefuse query authenticate --callers
function authenticate
  ID:   middleware.authenticate
  File: src/middleware/auth.ts:8

  Callers (3):
    → login (method) @ src/controllers/auth.ts:20
    → register (method) @ src/controllers/auth.ts:38
    → TestAuth (function) @ tests/auth_test.go:15

# Check what authenticate calls
$ codefuse query authenticate --callees
function authenticate
  ID:   middleware.authenticate
  File: src/middleware/auth.ts:8

  Callees (2):
    → ValidateToken (function) @ src/middleware/auth.ts:10
    → CreateSession (function) @ src/middleware/auth.ts:12
```

### VFS Directory Structure

After `codefuse vfs generate`, agents can explore:

```
.codefuse/vfs/
├── symbols/          # One file per unique symbol name
│   ├── authenticate      # All definitions of "authenticate"
│   └── useAuth
├── outline/          # One file per source file
│   └── src_controllers_auth.ts
└── references/       # Call graph: callers & callees per symbol
    ├── authenticate      # Who calls authenticate + who it calls
    └── useAuth
```

## For Developers

### Daily Development

```bash
# Quick index before a coding session
codefuse index .

# Find where a function is defined
codefuse query handleRequest

# See the structure of an unfamiliar file
codefuse outline src/api/router.go

# Find all React hooks (prefix query — fast trie lookup)
codefuse query "use*" -k function

# Trace a call chain
codefuse query handleRequest --callees
codefuse query ValidateToken --callers
```

### Code Review

```bash
# Generate VFS for review
codefuse vfs generate

# Reviewer can browse symbols and their relationships
cat .codefuse/vfs/outline/src_models_user.go
cat .codefuse/vfs/symbols/ValidateEmail
cat .codefuse/vfs/references/ValidateEmail  # see callers
```

### Large Refactorings

```bash
# Find every occurrence of a symbol name
codefuse query "OldServiceName"

# Check if any function still calls it after rename
codefuse query "OldServiceName" --callers

# Verify no stale references remain
codefuse query "OldServiceName" && echo "Still referenced!"
```

### Setting up Tree-sitter

```bash
# Check what grammars are missing
codefuse setup treesitter

# Auto-install all missing grammars
codefuse setup treesitter --auto

# Then index with higher accuracy
codefuse index . --treesitter
```
