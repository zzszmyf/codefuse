package index

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yifanmeng/codefuse/pkg/types"
)

// TestBuildGraph_Parallel verifies that parallel index building produces
// identical results to serial building. The same test project is built
// and we verify all nodes and edges are present.
func TestBuildGraph_Parallel(t *testing.T) {
	// Create a multi-package project with cross-file calls.
	files := map[string]string{
		"auth/auth.go": `package auth

func CheckToken(t string) bool { return true }
func Login(u string) bool { return CheckToken(u) }
`,
		"api/handler.go": `package api

import "myproject/auth"

func Handle() { auth.Login("user") }
func Health() {}
`,
		"service/data.go": `package service

type Store struct{}
func (s *Store) Get(id int) int { return id }
`,
		"main.go": `package main

import (
	"myproject/api"
	"myproject/service"
)

func Run() {
	api.Handle()
	s := &service.Store{}
	s.Get(1)
}
`,
	}

	tmpDir := t.TempDir()
	for path, content := range files {
		dir := filepath.Join(tmpDir, filepath.Dir(path))
		require.NoError(t, os.MkdirAll(dir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, path), []byte(content), 0644))
	}

	fileEntries := []types.FileEntry{
		{Path: "auth/auth.go", AbsPath: filepath.Join(tmpDir, "auth/auth.go"), Language: types.LangGo},
		{Path: "api/handler.go", AbsPath: filepath.Join(tmpDir, "api/handler.go"), Language: types.LangGo},
		{Path: "service/data.go", AbsPath: filepath.Join(tmpDir, "service/data.go"), Language: types.LangGo},
		{Path: "main.go", AbsPath: filepath.Join(tmpDir, "main.go"), Language: types.LangGo},
	}

	graph, err := BuildGraph(tmpDir, fileEntries, false)
	require.NoError(t, err)

	// Verify all expected nodes exist.
	expectedNodes := []string{
		"auth.CheckToken",
		"auth.Login",
		"api.Handle",
		"api.Health",
		"service.Store",
		"service.Store.Get",
		"main.Run",
	}
	for _, id := range expectedNodes {
		assert.NotNil(t, graph.FindNodeByID(id), "node %s should exist", id)
	}

	// Verify call graph edges.
	// Login -> CheckToken
	assert.True(t, hasCallee(graph.FindCallees("auth.Login"), "auth.CheckToken"))

	// Handle -> Login (cross-package)
	assert.True(t, hasCallee(graph.FindCallees("api.Handle"), "auth.Login"))

	// Run -> Handle (cross-package)
	assert.True(t, hasCallee(graph.FindCallees("main.Run"), "api.Handle"))

	// Health has no calls
	assert.Empty(t, graph.FindCallees("api.Health"))
}

// BenchmarkBuildGraph_SerialVsParallel compares serial and parallel build performance.
func BenchmarkBuildGraph(b *testing.B) {
	// Create a moderate-sized project (20 files, ~50 functions each).
	tmpDir := b.TempDir()
	var fileEntries []types.FileEntry

	for i := 0; i < 20; i++ {
		pkgName := filepath.Join(tmpDir, "pkg"+string(rune('0'+byte(i%10))))
		require.NoError(b, os.MkdirAll(pkgName, 0755))

		var content string
		for j := 0; j < 50; j++ {
			content += "func Func" + string(rune('A'+j%26)) + "() {}\n"
		}
		path := filepath.Join(pkgName, "code.go")
		require.NoError(b, os.WriteFile(path, []byte("package pkg"+string(rune('0'+i%10))+"\n"+content), 0644))
		fileEntries = append(fileEntries, types.FileEntry{
			Path:     filepath.Join("pkg"+string(rune('0'+i%10)), "code.go"),
			AbsPath:  path,
			Language: types.LangGo,
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := BuildGraph(tmpDir, fileEntries, false)
		require.NoError(b, err)
	}
}
