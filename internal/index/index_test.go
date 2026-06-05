package index

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yifanmeng/codefuse/pkg/types"
)

func TestBuild_ExtractsGoSymbols(t *testing.T) {
	content := `package main

import "fmt"

type User struct {
	Name string
}

func Hello(name string) string {
	return "Hello, " + name
}

func (u *User) Greet() string {
	return "Hi"
}
`
	tmpDir := t.TempDir()
	goFile := filepath.Join(tmpDir, "hello.go")
	require.NoError(t, os.WriteFile(goFile, []byte(content), 0644))

	files := []types.FileEntry{{
		Path:     "hello.go",
		AbsPath:  goFile,
		Language: types.LangGo,
	}}

	idx, err := Build(tmpDir, files, false)
	require.NoError(t, err)

	for _, sym := range idx.Symbols {
		t.Logf("Symbol: name=%s kind=%s line=%d", sym.Name, sym.Kind, sym.Line)
	}
	require.Len(t, idx.Symbols, 4) // package, type, func, method

	kinds := make(map[string]int)
	for _, sym := range idx.Symbols {
		kinds[sym.Kind]++
	}
	assert.Equal(t, 1, kinds[types.KindPackage])
	assert.Equal(t, 1, kinds[types.KindStruct])
	assert.Equal(t, 1, kinds[types.KindFunction])
	assert.Equal(t, 1, kinds[types.KindMethod])

	// Check method has parent
	for _, sym := range idx.Symbols {
		if sym.Kind == types.KindMethod {
			assert.Equal(t, "User", sym.Parent)
		}
	}
}

func TestBuild_ExtractsPythonSymbols(t *testing.T) {
	content := `class Calculator:
    def add(self, a, b):
        return a + b

def main():
    pass
`
	tmpDir := t.TempDir()
	pyFile := filepath.Join(tmpDir, "calc.py")
	require.NoError(t, os.WriteFile(pyFile, []byte(content), 0644))

	files := []types.FileEntry{{
		Path:     "calc.py",
		AbsPath:  pyFile,
		Language: types.LangPython,
	}}

	idx, err := Build(tmpDir, files, false)
	require.NoError(t, err)

	kinds := make(map[string]int)
	for _, sym := range idx.Symbols {
		kinds[sym.Kind]++
	}
	assert.Equal(t, 1, kinds[types.KindClass])
	assert.Equal(t, 2, kinds[types.KindMethod]+kinds[types.KindFunction])
}

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	idxDir := filepath.Join(tmpDir, ".codefuse")

	idx := &Index{
		ProjectPath: tmpDir,
		Files: []types.FileEntry{
			{Path: "main.go", Language: types.LangGo},
		},
		Symbols: []types.Symbol{
			{Name: "main", Kind: types.KindFunction, File: "main.go", Line: 1},
		},
	}

	err := idx.Save(idxDir)
	require.NoError(t, err)

	loaded, err := Load(idxDir)
	require.NoError(t, err)
	assert.Equal(t, idx.ProjectPath, loaded.ProjectPath)
	assert.Len(t, loaded.Files, 1)
	assert.Len(t, loaded.Symbols, 1)
	assert.Equal(t, "main", loaded.Symbols[0].Name)
}

func TestFindSymbol(t *testing.T) {
	idx := &Index{
		Symbols: []types.Symbol{
			{Name: "Hello", Kind: types.KindFunction, File: "a.go", Line: 1},
			{Name: "HelloWorld", Kind: types.KindFunction, File: "b.go", Line: 2},
			{Name: "Hello", Kind: types.KindClass, File: "c.py", Line: 3},
		},
	}
	idx.buildSymbolMap()

	results := idx.FindSymbol("Hello", "")
	assert.Len(t, results, 2)

	results = idx.FindSymbol("Hello", types.KindFunction)
	assert.Len(t, results, 1)
	assert.Equal(t, types.KindFunction, results[0].Kind)
}

func TestBuild_EmptyProject(t *testing.T) {
	tmpDir := t.TempDir()
	idx, err := Build(tmpDir, nil, false)
	require.NoError(t, err)
	assert.Len(t, idx.Symbols, 0)
	assert.Len(t, idx.Files, 0)
}
