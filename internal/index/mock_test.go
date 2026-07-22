package index

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yifanmeng/codefuse/pkg/types"
)

// mockIndex creates a temp directory with a pre-built index for testing.
// Returns (indexDir, projectPath, cleanup).
func mockIndex(t *testing.T) (string, string, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	indexDir := filepath.Join(tmpDir, ".codefuse")
	os.MkdirAll(indexDir, 0755)

	g := &Graph{
		Graph: types.Graph{
			Version:     types.IndexVersion,
			ProjectPath: tmpDir,
			Files: []types.FileEntry{
				{Path: "auth.go", Language: "go", Size: 100},
				{Path: "db/db.go", Language: "go", Size: 200},
			},
			Nodes: []types.Node{
				{ID: "auth.go:3:1", Name: "Authenticate", File: "auth.go", Line: 3, Column: 1},
				{ID: "auth.go:8:1", Name: "Login", File: "auth.go", Line: 8, Column: 1},
				{ID: "auth.go:15:1", Name: "helper", File: "auth.go", Line: 15, Column: 1},
				{ID: "db/db.go:5:1", Name: "Query", File: "db/db.go", Line: 5, Column: 1},
				{ID: "db/db.go:12:1", Name: "Execute", File: "db/db.go", Line: 12, Column: 1},
			},
			Edges: []types.Edge{
				{From: "auth.go:8:1", To: "auth.go:3:1", Kind: types.EdgeKindCalls, File: "auth.go", Line: 9},
				{From: "auth.go:3:1", To: "db/db.go:5:1", Kind: types.EdgeKindCalls, File: "auth.go", Line: 4},
			},
			Sinks: []types.Sink{
				{From: "db/db.go:5:1", CalleeName: "sql.Query", Pkg: "sql", File: "db/db.go", Line: 6},
				{From: "db/db.go:5:1", CalleeName: "http.Get", Pkg: "http", File: "db/db.go", Line: 7},
				{From: "auth.go:3:1", CalleeName: "os.ReadFile", Pkg: "os", File: "auth.go", Line: 5},
			},
			Imports: map[string][]types.FileImport{
				"auth.go": {{ShortName: "sql", FullPath: "db/db.go"}},
			},
			ModMap: types.ModuleMap{
				"db": "db/db.go",
			},
		},
	}
	g.BuildIndexes()
	g.BuildTrie()

	if err := g.Save(indexDir); err != nil {
		t.Fatalf("save mock index: %v", err)
	}

	// Also create source files so ReadLine works.
	os.MkdirAll(filepath.Join(tmpDir, "db"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "auth.go"),
		[]byte("package auth\n\nfunc Authenticate(token string) bool {\n\treturn token != \"\"\n}\n\nfunc Login(u, p string) bool {\n\treturn Authenticate(p)\n}\n\nfunc helper() {}\n"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "db/db.go"),
		[]byte("package db\n\nfunc Query(sql string) []Row {\n\treturn nil\n}\n\nfunc Execute(sql string) error {\n\treturn nil\n}\n"), 0644)

	return indexDir, tmpDir, func() { /* tmpDir auto-cleaned */ }
}
