package vfs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yifanmeng/codefuse/internal/index"
	"github.com/yifanmeng/codefuse/pkg/types"
)

func TestGenerator_GenerateAll(t *testing.T) {
	tmpDir := t.TempDir()

	graph := &index.Graph{Graph: types.Graph{
		ProjectPath: tmpDir,
		Nodes: []types.Node{
			{ID: "main.main", Name: "main", Kind: types.KindPackage, File: "main.go", Line: 1},
			{ID: "main.Greeter", Name: "Greeter", Kind: types.KindStruct, File: "main.go", Line: 3},
			{ID: "main.Greeter.Hello", Name: "Hello", Kind: types.KindMethod, File: "main.go", Line: 5, Parent: "Greeter"},
			{ID: "util.World", Name: "World", Kind: types.KindFunction, File: "util.go", Line: 1},
		},
		Edges: []types.Edge{
			{From: "main.Greeter.Hello", To: "util.World", Kind: types.EdgeKindCalls, File: "main.go", Line: 6},
		},
	}}
	graph.BuildIndexes()

	gen := NewGenerator(graph, tmpDir)
	err := gen.GenerateAll()
	require.NoError(t, err)

	// Check symbol views
	symbolDir := filepath.Join(tmpDir, ".codefuse", "vfs", "symbols")
	_, err = os.Stat(symbolDir)
	require.NoError(t, err)

	// Greeter should exist
	greeterPath := filepath.Join(symbolDir, "Greeter")
	content, err := os.ReadFile(greeterPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "Greeter")
	assert.Contains(t, string(content), "struct")
	assert.Contains(t, string(content), "main.go:3")

	// Hello should exist
	helloPath := filepath.Join(symbolDir, "Hello")
	content, err = os.ReadFile(helloPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "Hello")
	assert.Contains(t, string(content), "method")

	// Check outline views
	outlineDir := filepath.Join(tmpDir, ".codefuse", "vfs", "outline")
	_, err = os.Stat(outlineDir)
	require.NoError(t, err)

	mainOutline := filepath.Join(outlineDir, "main.go")
	content, err = os.ReadFile(mainOutline)
	require.NoError(t, err)
	assert.Contains(t, string(content), "package")
	assert.Contains(t, string(content), "Greeter")
	assert.Contains(t, string(content), "Hello")

	// Check references view exists
	refDir := filepath.Join(tmpDir, ".codefuse", "vfs", "references")
	_, err = os.Stat(refDir)
	require.NoError(t, err)
}

// TestGenerator_ReferenceViews verifies that the references/ directory
// contains caller/callee information for each symbol.
func TestGenerator_ReferenceViews(t *testing.T) {
	tmpDir := t.TempDir()

	graph := &index.Graph{Graph: types.Graph{
		ProjectPath: tmpDir,
		Nodes: []types.Node{
			{ID: "auth.Authenticate", Name: "Authenticate", Kind: types.KindFunction, File: "auth.go", Line: 5},
			{ID: "auth.Validate", Name: "Validate", Kind: types.KindFunction, File: "auth.go", Line: 10},
			{ID: "main.Login", Name: "Login", Kind: types.KindFunction, File: "main.go", Line: 3},
			{ID: "main.Health", Name: "Health", Kind: types.KindFunction, File: "main.go", Line: 8},
		},
		Edges: []types.Edge{
			{From: "auth.Authenticate", To: "auth.Validate", Kind: types.EdgeKindCalls, File: "auth.go", Line: 6},
			{From: "main.Login", To: "auth.Authenticate", Kind: types.EdgeKindCalls, File: "main.go", Line: 4},
		},
	}}
	graph.BuildIndexes()

	gen := NewGenerator(graph, tmpDir)
	err := gen.GenerateAll()
	require.NoError(t, err)

	refDir := filepath.Join(tmpDir, ".codefuse", "vfs", "references")

	// Authenticate should have caller (Login from main.go) and callee (Validate from auth.go)
	authPath := filepath.Join(refDir, "Authenticate")
	content, err := os.ReadFile(authPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "Callers")
	assert.Contains(t, string(content), "`Login`")          // caller short name
	assert.Contains(t, string(content), "main.go:4")        // caller location
	assert.Contains(t, string(content), "Callees")
	assert.Contains(t, string(content), "`Validate`")       // callee short name
	assert.Contains(t, string(content), "auth.go:6")        // callee location

	// Validate should have caller (Authenticate) but no callees
	validatePath := filepath.Join(refDir, "Validate")
	content, err = os.ReadFile(validatePath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "`Authenticate`")   // caller short name
	assert.Contains(t, string(content), "No callees")

	// Health should have no callers and no callees
	healthPath := filepath.Join(refDir, "Health")
	content, err = os.ReadFile(healthPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "No callers")
	assert.Contains(t, string(content), "No callees")
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"foo/bar", "foo_bar"},
		{"foo\\bar", "foo_bar"},
		{"foo:bar", "foo_bar"},
		{"foo*bar", "foo_bar"},
		{"foo?bar", "foo_bar"},
		{"foo<bar>", "foo_bar_"},
		{"foo|bar", "foo_bar"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, sanitizeFilename(tt.input))
		})
	}
}
