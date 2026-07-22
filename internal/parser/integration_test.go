package parser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yifanmeng/codefuse/pkg/types"
)

// Integration tests require tree-sitter CLI. They skip gracefully when it's absent.

func requireTreeSitter(t *testing.T) {
	t.Helper()
	if !TreeSitterAvailable() {
		t.Skip("tree-sitter CLI not installed")
	}
}

func writeTempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}

func TestIntegration_ExtractFile_Python(t *testing.T) {
	requireTreeSitter(t)
	tmpDir := t.TempDir()

	content := "def hello():\n    pass\n\nclass Foo:\n    def bar(self):\n        pass\n"
	path := writeTempFile(t, tmpDir, "test.py", content)

	nodes, edges, sinks, err := ExtractFile(path, "test.py", "python")
	require.NoError(t, err)
	assert.NotEmpty(t, nodes, "should extract Python symbols")

	names := make(map[string]bool)
	for _, n := range nodes {
		names[n.Name] = true
	}
	assert.True(t, names["hello"], "should find hello function")
	assert.True(t, names["bar"], "should find bar method")
	assert.True(t, names["Foo"], "should find Foo class")
	_ = edges
	_ = sinks
}

func TestIntegration_ExtractFile_Java(t *testing.T) {
	requireTreeSitter(t)
	tmpDir := t.TempDir()

	content := "class Hello {\n    void world() {}\n}\n"
	path := writeTempFile(t, tmpDir, "Hello.java", content)

	nodes, _, _, err := ExtractFile(path, "Hello.java", "java")
	require.NoError(t, err)
	assert.NotEmpty(t, nodes, "should extract Java symbols")
}

func TestIntegration_ExtractFile_Go(t *testing.T) {
	requireTreeSitter(t)
	tmpDir := t.TempDir()

	content := "package main\n\nfunc hello() {}\n"
	path := writeTempFile(t, tmpDir, "main.go", content)

	nodes, _, _, err := ExtractFile(path, "main.go", "go")
	require.NoError(t, err)
	assert.NotEmpty(t, nodes, "should extract Go symbols")
}

func TestIntegration_ExtractFile_SyntaxError(t *testing.T) {
	requireTreeSitter(t)
	tmpDir := t.TempDir()

	// Intentionally broken Python.
	path := writeTempFile(t, tmpDir, "broken.py", "def broken(:\n    pass\n")

	_, _, _, err := ExtractFile(path, "broken.py", "python")
	// tree-sitter may parse broken syntax gracefully (error recovery).
	// We just verify it doesn't crash.
	_ = err
}

func TestIntegration_ExtractFile_Empty(t *testing.T) {
	requireTreeSitter(t)
	tmpDir := t.TempDir()

	path := writeTempFile(t, tmpDir, "empty.py", "\n")

	nodes, _, _, err := ExtractFile(path, "empty.py", "python")
	require.NoError(t, err)
	assert.Empty(t, nodes, "empty file should produce no symbols")
}


func TestIntegration_ExtractBatch_Python(t *testing.T) {
	requireTreeSitter(t)
	tmpDir := t.TempDir()

	writeTempFile(t, tmpDir, "a.py", "def foo():\n    pass\n")
	writeTempFile(t, tmpDir, "b.py", "class Bar:\n    pass\n")

	files := scanTempDir(t, tmpDir)
	require.Len(t, files, 2)

	nodesByFile, edgesByFile, sinksByFile, failed := ExtractBatch(files)
	assert.Empty(t, failed, "batch extraction should not fail")
	assert.Len(t, nodesByFile, 2)
	_ = edgesByFile
	_ = sinksByFile
}


// Helpers

func scanTempDir(t *testing.T, dir string) []types.FileEntry {
	t.Helper()
	var files []types.FileEntry
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(e.Name())
		lang := detectExtLang(ext)
		if lang == "" {
			continue
		}
		info, _ := e.Info()
		files = append(files, types.FileEntry{
			Path:     e.Name(),
			AbsPath:  filepath.Join(dir, e.Name()),
			Language: lang,
			Size:     info.Size(),
			Mtime:    info.ModTime().UnixNano(),
		})
	}
	return files
}

func detectExtLang(ext string) string {
	for name, c := range BuiltinConfig() {
		for _, e := range c.Extensions {
			if e == ext {
				return name
			}
		}
	}
	return ""
}
