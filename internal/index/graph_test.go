package index

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yifanmeng/codefuse/pkg/types"
)

func TestNewGraph(t *testing.T) {
	g := NewGraph("/project")
	assert.Equal(t, "/project", g.ProjectPath)
	assert.Equal(t, types.IndexVersion, g.Version)
	assert.NotNil(t, g.Files)
	assert.NotNil(t, g.Nodes)
	assert.NotNil(t, g.Edges)
}

func TestSaveAndLoadGraph(t *testing.T) {
	tmpDir := t.TempDir()

	g := NewGraph("/test")
	g.Nodes = []types.Node{
		{ID: "a.go:5:1", Name: "Hello", File: "a.go", Line: 5, Column: 1},
		{ID: "b.go:10:1", Name: "World", File: "b.go", Line: 10, Column: 1},
	}
	g.Edges = []types.Edge{
		{From: "a.go:5:1", To: "b.go:10:1", Kind: types.EdgeKindCalls, File: "a.go", Line: 7},
	}
	g.BuildIndexes()
	g.BuildTrie()

	require.NoError(t, g.Save(tmpDir))

	// Load and verify.
	loaded, err := LoadGraph(tmpDir)
	require.NoError(t, err)
	assert.Len(t, loaded.Nodes, 2)
	assert.Len(t, loaded.Edges, 1)

	// Query should work.
	results := loaded.Query("Hello", false)
	assert.Len(t, results, 1)
	assert.Equal(t, "a.go", results[0].File)
}

func TestQuery_ExactMatch(t *testing.T) {
	g := NewGraph("/test")
	g.Nodes = []types.Node{
		{ID: "a.go:1:1", Name: "AuthService", File: "a.go", Line: 1, Column: 1},
		{ID: "a.go:10:1", Name: "AuthClient", File: "a.go", Line: 10, Column: 1},
		{ID: "b.go:1:1", Name: "Serve", File: "b.go", Line: 1, Column: 1},
	}
	g.BuildIndexes()
	g.BuildTrie()

	results := g.Query("AuthService", false)
	assert.Len(t, results, 1)
	assert.Equal(t, "AuthService", results[0].Name)
}

func TestQuery_PrefixMatch(t *testing.T) {
	g := NewGraph("/test")
	g.Nodes = []types.Node{
		{ID: "a.go:1:1", Name: "AuthService", File: "a.go", Line: 1, Column: 1},
		{ID: "a.go:10:1", Name: "AuthClient", File: "a.go", Line: 10, Column: 1},
		{ID: "b.go:1:1", Name: "Serve", File: "b.go", Line: 1, Column: 1},
	}
	g.BuildIndexes()
	g.BuildTrie()

	results := g.Query("Auth*", false)
	assert.Len(t, results, 2)
}

func TestQuery_GlobMatch(t *testing.T) {
	g := NewGraph("/test")
	g.Nodes = []types.Node{
		{ID: "a.go:1:1", Name: "AuthService", File: "a.go", Line: 1, Column: 1},
		{ID: "b.go:1:1", Name: "Serve", File: "b.go", Line: 1, Column: 1},
	}
	g.BuildIndexes()
	g.BuildTrie()

	results := g.Query("*Service", false)
	assert.Len(t, results, 1)
	assert.Equal(t, "AuthService", results[0].Name)
}

func TestQuery_NoMatch(t *testing.T) {
	g := NewGraph("/test")
	g.Nodes = []types.Node{
		{ID: "a.go:1:1", Name: "Foo", File: "a.go", Line: 1, Column: 1},
	}
	g.BuildIndexes()
	g.BuildTrie()

	results := g.Query("NotFound", false)
	assert.Empty(t, results)
}

func TestFindCallers_Callees(t *testing.T) {
	g := NewGraph("/test")
	g.Nodes = []types.Node{
		{ID: "a.go:1:1", Name: "Caller", File: "a.go", Line: 1, Column: 1},
		{ID: "b.go:5:1", Name: "Middle", File: "b.go", Line: 5, Column: 1},
		{ID: "c.go:10:1", Name: "Target", File: "c.go", Line: 10, Column: 1},
	}
	g.Edges = []types.Edge{
		{From: "a.go:1:1", To: "b.go:5:1", Kind: types.EdgeKindCalls, File: "a.go", Line: 2},
		{From: "b.go:5:1", To: "c.go:10:1", Kind: types.EdgeKindCalls, File: "b.go", Line: 6},
	}
	g.BuildIndexes()

	callers := g.FindCallers("c.go:10:1")
	assert.Len(t, callers, 1)
	assert.Equal(t, "Middle", callers[0].Node.Name)

	callees := g.FindCallees("a.go:1:1")
	assert.Len(t, callees, 1)
	assert.Equal(t, "Middle", callees[0].Node.Name)
}

func TestReadLine(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.go")
	content := "package main\n\nfunc Hello() {\n\tprintln(\"hi\")\n}\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	line, err := ReadLine(path, 3)
	require.NoError(t, err)
	assert.Equal(t, "func Hello() {", line)

	_, err = ReadLine(path, 100)
	assert.Error(t, err)
}

func TestReadLines(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.py")
	content := "def foo():\n    pass\n\ndef bar():\n    return 1\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	lines, err := ReadLines(path, 1, 2)
	require.NoError(t, err)
	assert.Len(t, lines, 2)
	assert.Equal(t, "def foo():", lines[0])
}
