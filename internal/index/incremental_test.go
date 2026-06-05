package index

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yifanmeng/codefuse/pkg/types"
)

func TestIncrementalIndex_OnlyParsesChangedFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create initial files
	mainGo := filepath.Join(tmpDir, "main.go")
	utilGo := filepath.Join(tmpDir, "util.go")

	require.NoError(t, os.WriteFile(mainGo, []byte("package main\n\nfunc main() {}\n"), 0644))
	require.NoError(t, os.WriteFile(utilGo, []byte("package main\n\nfunc Util() {}\n"), 0644))

	// First full index
	files := []types.FileEntry{
		{Path: "main.go", AbsPath: mainGo, Language: types.LangGo, Mtime: getMtime(mainGo)},
		{Path: "util.go", AbsPath: utilGo, Language: types.LangGo, Mtime: getMtime(utilGo)},
	}

	idx, err := Build(tmpDir, files, false)
	require.NoError(t, err)
	require.Len(t, idx.Symbols, 4) // package main, func main, package main, func Util

	err = idx.Save(filepath.Join(tmpDir, ".codefuse"))
	require.NoError(t, err)

	// Modify only main.go
	require.NoError(t, os.WriteFile(mainGo, []byte("package main\n\nfunc main() {}\nfunc Hello() {}\n"), 0644))

	// Second incremental index
	files2 := []types.FileEntry{
		{Path: "main.go", AbsPath: mainGo, Language: types.LangGo, Mtime: getMtime(mainGo)},
		{Path: "util.go", AbsPath: utilGo, Language: types.LangGo, Mtime: getMtime(utilGo)},
	}

	idx2, changed, err := BuildIncremental(tmpDir, files2, false)
	require.NoError(t, err)
	assert.Equal(t, 1, changed, "only main.go should be re-parsed")

	// Should have main, Hello (from main.go) and Util (retained from util.go)
	names := make(map[string]bool)
	for _, sym := range idx2.Symbols {
		names[sym.Name] = true
	}
	assert.True(t, names["main"])
	assert.True(t, names["Hello"], "new symbol from changed file should exist")
	assert.True(t, names["Util"], "unchanged file's symbols should be retained")
}

func TestIncrementalIndex_RemovesDeletedFiles(t *testing.T) {
	tmpDir := t.TempDir()

	mainGo := filepath.Join(tmpDir, "main.go")
	utilGo := filepath.Join(tmpDir, "util.go")

	require.NoError(t, os.WriteFile(mainGo, []byte("package main\n\nfunc main() {}\n"), 0644))
	require.NoError(t, os.WriteFile(utilGo, []byte("package main\n\nfunc Util() {}\n"), 0644))

	files := []types.FileEntry{
		{Path: "main.go", AbsPath: mainGo, Language: types.LangGo},
		{Path: "util.go", AbsPath: utilGo, Language: types.LangGo},
	}

	idx, err := Build(tmpDir, files, false)
	require.NoError(t, err)
	err = idx.Save(filepath.Join(tmpDir, ".codefuse"))
	require.NoError(t, err)

	// Now util.go is "deleted" (not in file list)
	files2 := []types.FileEntry{
		{Path: "main.go", AbsPath: mainGo, Language: types.LangGo, Mtime: getMtime(mainGo)},
	}

	idx2, _, err := BuildIncremental(tmpDir, files2, false)
	require.NoError(t, err)

	names := make(map[string]bool)
	for _, sym := range idx2.Symbols {
		names[sym.Name] = true
	}
	assert.True(t, names["main"])
	assert.False(t, names["Util"], "deleted file's symbols should be removed")
}

func TestIncrementalIndex_NoChanges(t *testing.T) {
	tmpDir := t.TempDir()

	mainGo := filepath.Join(tmpDir, "main.go")
	require.NoError(t, os.WriteFile(mainGo, []byte("package main\n\nfunc main() {}\n"), 0644))

	files := []types.FileEntry{
		{Path: "main.go", AbsPath: mainGo, Language: types.LangGo, Mtime: getMtime(mainGo)},
	}

	idx, err := Build(tmpDir, files, false)
	require.NoError(t, err)
	err = idx.Save(filepath.Join(tmpDir, ".codefuse"))
	require.NoError(t, err)

	// Re-index with same files (same mtime)
	files2 := []types.FileEntry{
		{Path: "main.go", AbsPath: mainGo, Language: types.LangGo, Mtime: getMtime(mainGo)},
	}

	idx2, changed, err := BuildIncremental(tmpDir, files2, false)
	require.NoError(t, err)
	assert.Equal(t, 0, changed, "no files should be re-parsed")
	assert.Len(t, idx2.Symbols, 2) // package + func main
}

func getMtime(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.ModTime().UnixNano()
}
