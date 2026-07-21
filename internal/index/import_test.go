package index

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yifanmeng/codefuse/internal/parser"
	"github.com/yifanmeng/codefuse/pkg/types"
)

func TestParseImports_Python_ImportFrom(t *testing.T) {
	content := `from db.user_dao import UserDao
from utils.helper import encrypt, decrypt as dec
import os
import torch.nn as nn`

	imports, modMap := parser.ParseImports(content, "service/auth.py", "python")
	assert.NotEmpty(t, imports)

	// UserDao: from db.user_dao import UserDao → FullPath = db/user_dao.py
	assert.Equal(t, "db/user_dao.py", modMap["db.user_dao"])
	userDao := findImport(imports, "UserDao")
	assert.NotNil(t, userDao)
	assert.Equal(t, "db/user_dao.py", userDao.FullPath)

	// encrypt: no alias
	enc := findImport(imports, "encrypt")
	assert.NotNil(t, enc)
	assert.Equal(t, "utils/helper.py", enc.FullPath)

	// dec: alias for decrypt → ShortName="decrypt", Alias="dec"
	dc := findImport(imports, "dec")
	assert.NotNil(t, dc)
	assert.Equal(t, "decrypt", dc.ShortName)
	assert.Equal(t, "dec", dc.Alias)

	// os: import os → module-level, no specific name
	assert.Contains(t, modMap, "os")

	// torch.nn: import torch.nn as nn → alias
	assert.Contains(t, modMap, "torch.nn")
}

func TestParseImports_Java_SingleClass(t *testing.T) {
	content := `import com.foo.UserDao;
import java.sql.Connection;
import org.apache.dubbo.registry.RegistryService;
import java.util.*;`

	imports, modMap := parser.ParseImports(content, "com/foo/UserService.java", "java")
	assert.NotEmpty(t, imports)

	assert.Equal(t, "com/foo/UserDao.java", modMap["com.foo.UserDao"])
	assert.Equal(t, "java/sql/Connection.java", modMap["java.sql.Connection"])
	assert.Equal(t, "org/apache/dubbo/registry/RegistryService.java", modMap["org.apache.dubbo.registry.RegistryService"])
	assert.Equal(t, "java/util/", modMap["java.util.*"])

	userDao := findImport(imports, "UserDao")
	assert.NotNil(t, userDao)
	assert.Equal(t, "com/foo/UserDao.java", userDao.FullPath)
}

func TestParseImports_Go(t *testing.T) {
	content := `package auth
import (
	"myproject/db"
	sqlx "github.com/jmoiron/sqlx"
	"myproject/utils"
)`

	imports, modMap := parser.ParseImports(content, "service/auth.go", "go")
	assert.NotEmpty(t, modMap)

	assert.Equal(t, "myproject/db/", modMap["db"])
	assert.Equal(t, "github.com/jmoiron/sqlx", modMap["sqlx"])
	assert.Equal(t, "myproject/utils/", modMap["utils"])
	_ = imports
}

func TestResolveEdge_CrossFile_WithImport(t *testing.T) {
	// Simulate: AuthService.java imports UserDao, calls userDao.findById()
	// The callee "findById" should resolve to UserDao.java's findById method.
	g := &Graph{
		Graph: types.Graph{
			Nodes: []types.Node{
				{ID: "db/UserDao.java:15:1", Name: "findById", File: "db/UserDao.java", Line: 15, Column: 1},
				{ID: "service/AuthService.java:10:1", Name: "login", File: "service/AuthService.java", Line: 10, Column: 1},
				{ID: "service/AuthService.java:12:1", Name: "findById", File: "service/AuthService.java", Line: 12, Column: 1},
			},
		},
	}
	g.BuildIndexes()
	g.BuildTrie()

	// AuthService.java imports UserDao from db/UserDao.java
	imports := []types.FileImport{
		{ShortName: "UserDao", FullPath: "db/UserDao.java"},
	}
	modMap := types.ModuleMap{"com.foo.UserDao": "db/UserDao.java"}

	// Edge: login → userDao.findById (callee name contains method call)
	edge := types.Edge{
		From: "service/AuthService.java:10:1", // login
		To:   "userDao.findById",               // callee expression
		Kind: types.EdgeKindCalls,
		File: "service/AuthService.java",
		Line: 11,
	}

	resolved := resolveEdgeWithImports(edge, imports, modMap, &g.Graph)
	// Should find findById in db/UserDao.java
	assert.True(t, len(resolved) > 0, "should resolve cross-file edge via import")
	assert.Equal(t, "db/UserDao.java:15:1", resolved[0].To)
}

func TestResolveEdge_SameFile_WinsOverImport(t *testing.T) {
	g := &Graph{
		Graph: types.Graph{
			Nodes: []types.Node{
				{ID: "a.go:10:1", Name: "encrypt", File: "a.go", Line: 10, Column: 1},
				{ID: "b.go:5:1", Name: "encrypt", File: "b.go", Line: 5, Column: 1},
			},
		},
	}
	g.BuildIndexes()
	g.BuildTrie()

	imports := []types.FileImport{
		{ShortName: "encrypt", FullPath: "b.go"},
	}

	edge := types.Edge{
		From: "a.go:10:1",
		To:   "encrypt",
		Kind: types.EdgeKindCalls,
		File: "a.go",
		Line: 11,
	}

	resolved := resolveEdgeWithImports(edge, imports, nil, &g.Graph)
	// Same-file "encrypt" at a.go:10:1 should win over import-to-b.go.
	assert.Len(t, resolved, 1)
	assert.Equal(t, "a.go:10:1", resolved[0].To)
}

func TestBuildModuleMap(t *testing.T) {
	nodes := []types.Node{
		{File: "db/user_dao.py", Line: 1, Column: 1, Name: "UserDao"},
		{File: "db/user_dao.py", Line: 5, Column: 1, Name: "findById"},
		{File: "service/auth_service.py", Line: 1, Column: 1, Name: "AuthService"},
		{File: "com/foo/UserDao.java", Line: 1, Column: 1, Name: "UserDao"},
		{File: "com/foo/UserService.java", Line: 1, Column: 1, Name: "UserService"},
	}

	modMap := BuildModuleMap(nodes, "/project")
	assert.NotEmpty(t, modMap)

	// Python: db/user_dao.py → module "db.user_dao"
	assert.Equal(t, "db/user_dao.py", modMap["db.user_dao"])

	// Java: com/foo/UserDao.java → dotted name "com.foo.UserDao"
	assert.Equal(t, "com/foo/UserDao.java", modMap["com.foo.UserDao"])
}

// Helpers

func findImport(imports []types.FileImport, name string) *types.FileImport {
	for i, imp := range imports {
		if imp.ShortName == name || imp.Alias == name {
			return &imports[i]
		}
	}
	return nil
}

// Ensure parser package is accessible.
var _ = parser.TreeSitterAvailable
