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
ls .codefuse/vfs/symbols/ | grep "^use"

# 3. Or use CLI for targeted queries
codefuse query "*Controller"
codefuse outline src/services/user.ts
```

### Example Session

**Scenario**: Agent needs to understand how authentication works.

```bash
# Find auth-related symbols
$ codefuse query "*auth*"
Found 15 result(s) for '*auth*':
  function useAuth
    File: src/hooks/useAuth.ts:4
  class AuthController
    File: src/controllers/auth.ts:12
  function authenticate
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
## useAuth (function)
- File: src/hooks/useAuth.ts:4

# Check who calls authenticate
$ codefuse query "authenticate"
Found 3 result(s) for 'authenticate':
  function authenticate
    File: src/middleware/auth.ts:8
  method login
    File: src/controllers/auth.ts:18
  method register
    File: src/controllers/auth.ts:35
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

# Find all React hooks
codefuse query "use*" -k function
```

### Code Review

```bash
# Generate VFS for review
codefuse vfs generate

# Reviewer can browse symbols without IDE
cat .codefuse/vfs/outline/src_models_user.go
cat .codefuse/vfs/symbols/ValidateEmail
```

### Large Refactorings

```bash
# Find every occurrence of a symbol name
codefuse query "OldServiceName"

# Check if any file still references it after rename
codefuse query "OldServiceName" && echo "Still referenced!"
```
