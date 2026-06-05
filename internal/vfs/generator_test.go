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

	idx := &index.Index{
		ProjectPath: tmpDir,
		Symbols: []types.Symbol{
			{Name: "main", Kind: types.KindPackage, File: "main.go", Line: 1},
			{Name: "Greeter", Kind: types.KindStruct, File: "main.go", Line: 3},
			{Name: "Hello", Kind: types.KindMethod, File: "main.go", Line: 5, Parent: "Greeter"},
			{Name: "World", Kind: types.KindFunction, File: "util.go", Line: 1},
		},
	}

	gen := NewGenerator(idx, tmpDir)
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
