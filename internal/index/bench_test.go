package index

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yifanmeng/codefuse/pkg/types"
)

func BenchmarkBuild(b *testing.B) {
	// Create a synthetic project with 100 files
	tmpDir := b.TempDir()
	files := make([]types.FileEntry, 100)
	for i := range files {
		name := filepath.Join(tmpDir, "file", filepath.Join("a", "b", "c"), "file", "_"+string(rune(i)))
		os.MkdirAll(filepath.Dir(name), 0755)
		os.WriteFile(name, []byte(`package main

func Hello() {}
func World() {}
type Foo struct {}
`), 0644)
		files[i] = types.FileEntry{
			Path:     filepath.Join("file", "a", "b", "c", "file_"+string(rune(i))),
			AbsPath:  name,
			Language: types.LangGo,
			Mtime:    1,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := Build(tmpDir, files, false)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFindSymbol(b *testing.B) {
	idx := &Index{
		Symbols: make([]types.Symbol, 10000),
	}
	for i := 0; i < 10000; i++ {
		idx.Symbols[i] = types.Symbol{
			Name: "Symbol_" + string(rune(i)),
			Kind: types.KindFunction,
			File: "a.go",
			Line: i,
		}
	}
	idx.buildSymbolMap()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = idx.FindSymbol("Symbol_5000", "")
	}
}

func BenchmarkFindSymbolGlob(b *testing.B) {
	idx := &Index{
		Symbols: make([]types.Symbol, 10000),
	}
	for i := 0; i < 10000; i++ {
		idx.Symbols[i] = types.Symbol{
			Name: "Symbol_" + string(rune(i)),
			Kind: types.KindFunction,
			File: "a.go",
			Line: i,
		}
	}
	idx.buildSymbolMap()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = idx.FindSymbol("Symbol_*", "")
	}
}
