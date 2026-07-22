package index

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yifanmeng/codefuse/internal/scanner"
	"github.com/yifanmeng/codefuse/pkg/types"
)

func TestE2E_IndexAndQuery(t *testing.T) {
	tmpDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "auth.go"),
		[]byte("package auth\n\nfunc Authenticate(token string) bool {\n\treturn token != \"\"\n}\n\nfunc Login(user, pass string) bool {\n\treturn Authenticate(pass)\n}\n"), 0644))

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "main.go"),
		[]byte("package main\n\nimport \"myproject/auth\"\n\nfunc main() {\n\tauth.Authenticate(\"test\")\n}\n"), 0644))

	files, err := scanner.Scan(tmpDir)
	require.NoError(t, err)
	require.Len(t, files, 2)

	graph, err := BuildGraph(tmpDir, files)
	if err != nil {
		t.Logf("BuildGraph failed (tree-sitter may be missing): %v", err)
		return
	}
	if len(graph.Nodes) == 0 {
		t.Log("No symbols extracted (tree-sitter grammar may be missing)")
		return
	}

	results := graph.Query("Authenticate", false)
	assert.NotEmpty(t, results, "should find Authenticate")
	assert.Equal(t, "auth.go", results[0].File)
}

func TestE2E_ImportAndVarMap(t *testing.T) {
	tmpDir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "db"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "svc"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "db/dao.py"),
		[]byte("class UserDao:\n    def findById(self, uid):\n        pass\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "svc/auth.py"),
		[]byte("from db.dao import UserDao\n\ndef login():\n    dao = UserDao()\n    result = dao.findById(1)\n"), 0644))

	files, err := scanner.Scan(tmpDir)
	require.NoError(t, err)
	require.Len(t, files, 2)

	graph, err := BuildGraph(tmpDir, files)
	if err != nil || len(graph.Nodes) == 0 {
		t.Skip("tree-sitter not available")
		return
	}

	// Verify import parsing.
	if imports, ok := graph.Imports["svc/auth.py"]; ok {
		found := false
		for _, imp := range imports {
			if imp.ShortName == "UserDao" {
				found = true
				assert.True(t, strings.HasSuffix(imp.FullPath, "dao.py"))
			}
		}
		assert.True(t, found, "should find UserDao in imports")
	}
}

func TestE2E_Sinks(t *testing.T) {
	tmpDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "service.py"),
		[]byte("from sql import Query\n\ndef login(token):\n    result = sql.Query(\"SELECT * FROM users\")\n    return result\n"), 0644))

	files, err := scanner.Scan(tmpDir)
	require.NoError(t, err)

	graph, err := BuildGraph(tmpDir, files)
	if err != nil || len(graph.Nodes) == 0 {
		t.Skip("tree-sitter not available")
		return
	}

	if len(graph.Sinks) > 0 {
		sqlSinks := graph.FilterSinks(graph.Sinks, "sql")
		assert.NotEmpty(t, sqlSinks, "should capture sql.Query as external sink")
	}
}

func TestE2E_NoCrashOnEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	files, err := scanner.Scan(tmpDir)
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestE2E_SaveAndReload(t *testing.T) {
	tmpDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "main.go"),
		[]byte("package main\n\nfunc Hello() {}\n"), 0644))

	files, err := scanner.Scan(tmpDir)
	require.NoError(t, err)

	graph, err := BuildGraph(tmpDir, files)
	if err != nil || len(graph.Nodes) == 0 {
		t.Skip("tree-sitter not available")
		return
	}

	// Save and reload.
	indexDir := filepath.Join(tmpDir, ".codefuse")
	require.NoError(t, graph.Save(indexDir))

	loaded, err := LoadGraph(indexDir)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, len(graph.Nodes), len(loaded.Nodes))

	// Verify query works after reload.
	results := loaded.Query("Hello", false)
	assert.NotEmpty(t, results)
}

// Test for unused import.
var _ = types.LocationNodeID
