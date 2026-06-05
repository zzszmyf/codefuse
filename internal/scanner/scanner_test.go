package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yifanmeng/codefuse/pkg/types"
)

func TestScan_FindsSourceFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source files
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "utils.py"), []byte("def utils(): pass"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "app.rs"), []byte("fn main() {}"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "script.js"), []byte("console.log(1)"), 0644))

	files, err := Scan(tmpDir)
	require.NoError(t, err)
	assert.Len(t, files, 4)

	langs := make(map[string]bool)
	for _, f := range files {
		langs[f.Language] = true
	}
	assert.True(t, langs[types.LangGo])
	assert.True(t, langs[types.LangPython])
	assert.True(t, langs[types.LangRust])
	assert.True(t, langs[types.LangJS])
}

func TestScan_SkipsIgnoredDirs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file in node_modules
	nodeModDir := filepath.Join(tmpDir, "node_modules", "somepkg")
	require.NoError(t, os.MkdirAll(nodeModDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(nodeModDir, "index.js"), []byte(""), 0644))

	// Create a real source file
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main"), 0644))

	files, err := Scan(tmpDir)
	require.NoError(t, err)
	assert.Len(t, files, 1)
	assert.Equal(t, "main.go", files[0].Path)
}

func TestScan_SkipsNonSourceFiles(t *testing.T) {
	tmpDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("# readme"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "data.json"), []byte("{}"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "image.png"), []byte("PNG"), 0644))

	files, err := Scan(tmpDir)
	require.NoError(t, err)
	assert.Len(t, files, 0)
}

func TestScan_MarksTestFiles(t *testing.T) {
	tmpDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "foo.go"), []byte("package foo"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "foo_test.go"), []byte("package foo"), 0644))

	files, err := Scan(tmpDir)
	require.NoError(t, err)
	require.Len(t, files, 2)

	for _, f := range files {
		if f.Path == "foo_test.go" {
			assert.True(t, f.IsTest)
		} else {
			assert.False(t, f.IsTest)
		}
	}
}

func TestScan_SkipsGitDir(t *testing.T) {
	tmpDir := t.TempDir()

	gitDir := filepath.Join(tmpDir, ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "config"), []byte(""), 0644))

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main"), 0644))

	files, err := Scan(tmpDir)
	require.NoError(t, err)
	assert.Len(t, files, 1)
}
