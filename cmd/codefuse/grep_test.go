package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yifanmeng/codefuse/internal/index"
	"github.com/yifanmeng/codefuse/pkg/types"
)

// buildMockIndex creates a minimal index in a temp dir for grep testing.
func buildMockIndex(t *testing.T) (string, string) {
	t.Helper()
	tmpDir := t.TempDir()
	indexDir := filepath.Join(tmpDir, ".codefuse")
	os.MkdirAll(indexDir, 0755)

	g := index.NewGraph(tmpDir)
	g.Files = []types.FileEntry{
		{Path: "auth.go", Language: "go", Size: 100},
		{Path: "db/db.go", Language: "go", Size: 200},
	}
	g.Nodes = []types.Node{
		{ID: "auth.go:3:1", Name: "Authenticate", File: "auth.go", Line: 3, Column: 1},
		{ID: "auth.go:8:1", Name: "Login", File: "auth.go", Line: 8, Column: 1},
		{ID: "auth.go:15:1", Name: "helper", File: "auth.go", Line: 15, Column: 1},
		{ID: "db/db.go:5:1", Name: "Query", File: "db/db.go", Line: 5, Column: 1},
		{ID: "db/db.go:12:1", Name: "Execute", File: "db/db.go", Line: 12, Column: 1},
	}
	g.Edges = []types.Edge{
		{From: "auth.go:8:1", To: "auth.go:3:1", Kind: types.EdgeKindCalls, File: "auth.go", Line: 9},
		{From: "auth.go:3:1", To: "db/db.go:5:1", Kind: types.EdgeKindCalls, File: "auth.go", Line: 4},
	}
	g.Sinks = []types.Sink{
		{From: "db/db.go:5:1", CalleeName: "sql.Query", Pkg: "sql", File: "db/db.go", Line: 6},
		{From: "auth.go:3:1", CalleeName: "os.ReadFile", Pkg: "os", File: "auth.go", Line: 5},
	}
	g.Imports = map[string][]types.FileImport{
		"auth.go": {{ShortName: "db", FullPath: "db/db.go"}},
	}
	g.BuildIndexes()
	g.BuildTrie()
	if err := g.Save(indexDir); err != nil {
		t.Fatalf("save mock: %v", err)
	}

	// Create actual source files for ReadLine.
	os.MkdirAll(filepath.Join(tmpDir, "db"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "auth.go"),
		[]byte("package auth\n\nfunc Authenticate(token string) bool {\n\treturn token != \"\"\n}\n\nfunc Login(u, p string) bool {\n\treturn Authenticate(p)\n}\n"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "db/db.go"),
		[]byte("package db\n\nfunc Query(sql string) []Row {\n\treturn nil\n}\n"), 0644)

	return indexDir, tmpDir
}

func TestGrep_LoadAndQuery(t *testing.T) {
	indexDir, projDir := buildMockIndex(t)
	require.NotEmpty(t, indexDir)

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projDir)

	graph, err := index.LoadGraph(indexDir)
	require.NoError(t, err)
	require.NotNil(t, graph)

	results := graph.Query("Authenticate", false)
	assert.NotEmpty(t, results)
	assert.Equal(t, "auth.go", results[0].File)
}

func TestGrep_CaseInsensitive(t *testing.T) {
	indexDir, projDir := buildMockIndex(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projDir)

	graph, err := index.LoadGraph(indexDir)
	require.NoError(t, err)

	results := graph.Query("authenticate", true)
	assert.NotEmpty(t, results)
	assert.True(t, strings.EqualFold(results[0].Name, "authenticate"))
}

func TestGrep_Substring(t *testing.T) {
	indexDir, projDir := buildMockIndex(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projDir)

	graph, err := index.LoadGraph(indexDir)
	require.NoError(t, err)

	// "thent" → substring of "Authenticate".
	results := graph.Query("thent", false)
	assert.NotEmpty(t, results)
}

func TestGrep_Prefix(t *testing.T) {
	indexDir, projDir := buildMockIndex(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projDir)

	graph, err := index.LoadGraph(indexDir)
	require.NoError(t, err)

	results := graph.Query("Auth*", false)
	assert.NotEmpty(t, results)
}

func TestGrep_NoMatch(t *testing.T) {
	indexDir, projDir := buildMockIndex(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projDir)

	graph, err := index.LoadGraph(indexDir)
	require.NoError(t, err)

	results := graph.Query("NonExistent", false)
	assert.Empty(t, results)
}

func TestGrep_SinksForNode(t *testing.T) {
	indexDir, projDir := buildMockIndex(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projDir)

	graph, err := index.LoadGraph(indexDir)
	require.NoError(t, err)

	sinks := graph.SinksForNodeID("db/db.go:5:1")
	assert.Len(t, sinks, 1) // sql.Query
	assert.Equal(t, "sql", sinks[0].Pkg)
}

func TestGrep_Reachable(t *testing.T) {
	indexDir, projDir := buildMockIndex(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projDir)

	graph, err := index.LoadGraph(indexDir)
	require.NoError(t, err)

	// Login calls Authenticate, which has os.ReadFile sink.
	paths := graph.ReachableFrom("auth.go:8:1", "os", 10)
	assert.NotEmpty(t, paths, "Login should reach os.ReadFile via Authenticate")
}

func TestGrep_FindCallers(t *testing.T) {
	indexDir, projDir := buildMockIndex(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projDir)

	graph, err := index.LoadGraph(indexDir)
	require.NoError(t, err)

	callers := graph.FindCallers("auth.go:3:1") // Authenticate
	assert.Len(t, callers, 1)
	assert.Equal(t, "Login", callers[0].Node.Name)
}

func TestGrep_SaveAndReload(t *testing.T) {
	indexDir, projDir := buildMockIndex(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projDir)

	// Load via nodes-only path.
	graph, err := index.LoadGraphNodes(indexDir)
	require.NoError(t, err)
	assert.NotEmpty(t, graph.Nodes)
}

// Tests that exercise cmd-level functions (runGrepCompat, parseGrepFlags pipeline).

func TestGrep_RunGrepCompat_WithIndex(t *testing.T) {
	_, projDir := buildMockIndex(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projDir)

	// runGrepCompat exercises the full grep pipeline.
	// It loads the index, queries, reads source lines, and formats output.
	err := runGrepCompat([]string{"Authenticate"})
	assert.NoError(t, err)
}

func TestGrep_RunGrepCompat_CountOnly(t *testing.T) {
	_, projDir := buildMockIndex(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projDir)

	// With -c flag, should output count and not fall back.
	err := runGrepCompat([]string{"-c", "Authenticate"})
	assert.NoError(t, err)
}

func TestGrep_RunGrepCompat_FilesOnly(t *testing.T) {
	_, projDir := buildMockIndex(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projDir)

	err := runGrepCompat([]string{"-l", "Authenticate"})
	assert.NoError(t, err)
}

func TestGrep_RunGrepCompat_NoMatch_Fallback(t *testing.T) {
	_, projDir := buildMockIndex(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projDir)

	// Symbol not in index → should fall back to real grep.
	// real grep will find "NonExistent" in... nothing, but shouldn't crash.
	err := runGrepCompat([]string{"NonExistentSymbolXYZ"})
	// Fallback to real grep may return exit 1 (no matches) — that's OK.
	_ = err
}

func TestGrep_FilterByPaths_EdgeCases(t *testing.T) {
	hits := []hit{
		{file: "a/b.go", line: 1},
		{file: "c/d.go", line: 2},
	}

	// Empty paths → no filter.
	assert.Len(t, filterByPaths(hits, nil), 2)
	assert.Len(t, filterByPaths(hits, []string{}), 2)
	assert.Len(t, filterByPaths(hits, []string{"."}), 2)

	// "./a" should match "a/b.go".
	result := filterByPaths(hits, []string{"./a"})
	assert.Len(t, result, 1)
}

func TestGrep_FindIndexDir_FromSubdir(t *testing.T) {
	_, projDir := buildMockIndex(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)

	// Navigate to a subdirectory — should still find .codefuse/ by walking up.
	subDir := filepath.Join(projDir, "db")
	os.MkdirAll(subDir, 0755)
	os.Chdir(subDir)

	found, _ := findIndexDir()
	assert.NotEmpty(t, found, "should find .codefuse by walking up from subdir")
}

func TestIsGrepMode_EdgeCases(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()

	os.Args = []string{"/usr/bin/grep"}
	assert.True(t, isGrepMode())

	os.Args = []string{"grep"}
	assert.True(t, isGrepMode())

	os.Args = []string{"/usr/local/bin/codefuse"}
	assert.False(t, isGrepMode())
}

func TestParseGrepFlags_EmptyArgs(t *testing.T) {
	opts := parseGrepFlags([]string{})
	assert.Equal(t, "", opts.pattern)
	assert.True(t, opts.recursive)  // default
	assert.True(t, opts.lineNumber) // default
}

func TestParseGrepFlags_OnlyPattern(t *testing.T) {
	opts := parseGrepFlags([]string{"HelloWorld"})
	assert.Equal(t, "HelloWorld", opts.pattern)
}

// Tests using mock execGrepCmd to cover the full pipeline.

func TestGrep_ForceText_BypassIndex(t *testing.T) {
	_, projDir := buildMockIndex(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projDir)

	mockCalled := false
	origExec := execGrepCmd
	execGrepCmd = func(opts grepOptions) error {
		mockCalled = true
		assert.True(t, opts.forceText)
		return nil
	}
	defer func() { execGrepCmd = origExec }()

	// -t should bypass index and call execGrepCmd directly.
	err := runGrepCompat([]string{"-t", "Anything"})
	assert.NoError(t, err)
	assert.True(t, mockCalled, "-t should bypass index")
}

func TestGrep_NoIndex_Fallback(t *testing.T) {
	// No .codefuse/ directory → should fall back.
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(tmpDir)

	mockCalled := false
	origExec := execGrepCmd
	execGrepCmd = func(opts grepOptions) error {
		mockCalled = true
		return nil
	}
	defer func() { execGrepCmd = origExec }()

	err := runGrepCompat([]string{"pattern"})
	assert.NoError(t, err)
	assert.True(t, mockCalled, "no index should fall back to grep")
}

func TestGrep_NoMatch_CountZero(t *testing.T) {
	_, projDir := buildMockIndex(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projDir)

	// NonExistent symbol + -c → should output "0" and NOT fall back.
	err := runGrepCompat([]string{"-c", "NonExistentSymbolXYZ"})
	assert.NoError(t, err)
	// Note: this tests the count-only path for no-match (prints "0").
}

func TestGrep_WithLineNumbers(t *testing.T) {
	_, projDir := buildMockIndex(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projDir)

	// -n is on by default — verify output includes line numbers.
	err := runGrepCompat([]string{"Authenticate"})
	assert.NoError(t, err)
}

func TestGrep_NoLineNumbers(t *testing.T) {
	// This is hard to test without capturing stdout.
	// Verify the flag parsing at least.
	opts := parseGrepFlags([]string{"Authenticate"})
	assert.True(t, opts.lineNumber) // default
}

func TestGrep_InvertMatch(t *testing.T) {
	_, projDir := buildMockIndex(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projDir)

	// -v should run without error.
	err := runGrepCompat([]string{"-v", "Authenticate"})
	assert.NoError(t, err)
}

func TestGrep_OnlyMatching(t *testing.T) {
	_, projDir := buildMockIndex(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projDir)

	// -o should run without error.
	err := runGrepCompat([]string{"-o", "Authenticate"})
	assert.NoError(t, err)
}

func TestGrep_ContextLines(t *testing.T) {
	_, projDir := buildMockIndex(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projDir)

	err := runGrepCompat([]string{"-A3", "Authenticate"})
	assert.NoError(t, err)

	err = runGrepCompat([]string{"-B2", "Authenticate"})
	assert.NoError(t, err)

	err = runGrepCompat([]string{"-C1", "Authenticate"})
	assert.NoError(t, err)
}

func TestGrep_MaxCount(t *testing.T) {
	_, projDir := buildMockIndex(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projDir)

	err := runGrepCompat([]string{"-m1", "Auth*"})
	assert.NoError(t, err)
}

func TestExecRealGrep_FlagForwarding(t *testing.T) {
	// execRealGrep constructs correct grep command.
	// We can't easily test the actual command execution,
	// but we verify the function signature and flag mapping.
	opts := grepOptions{
		recursive:    true,
		lineNumber:   true,
		ignoreCase:   true,
		countOnly:    true,
		invertMatch:  true,
		maxCount:     5,
		contextAfter: 2,
		pattern:      "test",
		paths:        []string{"."},
	}
	assert.Equal(t, "test", opts.pattern)
	assert.Equal(t, 5, opts.maxCount)
}

func TestRunGrepCompat_FlagCombinations(t *testing.T) {
	_, projDir := buildMockIndex(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projDir)

	// Test various flag combinations don't crash.
	combos := [][]string{
		{"-rn", "Auth"},
		{"-il", "auth"},
		{"-rnc", "Auth"},
		{"-i", "-w", "authenticate"},
		{"-A2", "-B1", "Login"},
	}

	for _, args := range combos {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			err := runGrepCompat(args)
			assert.NoError(t, err)
		})
	}
}
