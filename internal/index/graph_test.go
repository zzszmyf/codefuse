package index

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yifanmeng/codefuse/pkg/types"
)

// TestBuildGraph_GoCallGraph verifies cross-file call graph extraction.
// This is the core v0.2 capability: "who calls whom" across files.
func TestBuildGraph_GoCallGraph(t *testing.T) {
	// File A: defines Authenticate and ValidateToken
	fileA := `package auth

func Authenticate(user string) bool {
	return ValidateToken(user)
}

func ValidateToken(token string) bool {
	return token != ""
}
`
	// File B: calls Authenticate from another package
	fileB := `package main

import "myproject/auth"

func Login() {
	if auth.Authenticate("alice") {
		println("ok")
	}
}

func Setup() {
	println("setup")
}
`
	// File C: defines a type with a method that calls Authenticate
	fileC := `package handlers

import "myproject/auth"

type Session struct{}

func (s *Session) Check() bool {
	return auth.Authenticate("bob")
}
`

	tmpDir := t.TempDir()

	authDir := filepath.Join(tmpDir, "auth")
	require.NoError(t, os.MkdirAll(authDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(authDir, "auth.go"), []byte(fileA), 0644))

	mainDir := filepath.Join(tmpDir, "main")
	require.NoError(t, os.MkdirAll(mainDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, "main.go"), []byte(fileB), 0644))

	handlerDir := filepath.Join(tmpDir, "handlers")
	require.NoError(t, os.MkdirAll(handlerDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(handlerDir, "session.go"), []byte(fileC), 0644))

	files := []types.FileEntry{
		{Path: "auth/auth.go", AbsPath: filepath.Join(authDir, "auth.go"), Language: types.LangGo},
		{Path: "main/main.go", AbsPath: filepath.Join(mainDir, "main.go"), Language: types.LangGo},
		{Path: "handlers/session.go", AbsPath: filepath.Join(handlerDir, "session.go"), Language: types.LangGo},
	}

	graph, err := BuildGraph(tmpDir, files, false)
	require.NoError(t, err)
	require.NotNil(t, graph)

	// Verify nodes exist
	authNode := graph.FindNodeByID("auth.Authenticate")
	require.NotNil(t, authNode, "auth.Authenticate node should exist")
	assert.Equal(t, "Authenticate", authNode.Name)
	assert.Equal(t, types.KindFunction, authNode.Kind)

	validateNode := graph.FindNodeByID("auth.ValidateToken")
	require.NotNil(t, validateNode, "auth.ValidateToken node should exist")

	loginNode := graph.FindNodeByID("main.Login")
	require.NotNil(t, loginNode, "main.Login node should exist")

	sessionCheckNode := graph.FindNodeByID("handlers.Session.Check")
	require.NotNil(t, sessionCheckNode, "handlers.Session.Check node should exist")
	assert.Equal(t, "Check", sessionCheckNode.Name)
	assert.Equal(t, types.KindMethod, sessionCheckNode.Kind)
	assert.Equal(t, "Session", sessionCheckNode.Parent)

	// --- Core call graph assertions ---

	// 1. Authenticate calls ValidateToken (same file)
	callees := graph.FindCallees("auth.Authenticate")
	assert.True(t, hasCallee(callees, "auth.ValidateToken"),
		"Authenticate should call ValidateToken, got: %v", formatEdges(callees))

	// 2. Login calls Authenticate (cross-file, cross-package)
	callers := graph.FindCallers("auth.Authenticate")
	assert.True(t, hasCaller(callers, "main.Login"),
		"Authenticate should be called by Login, got: %v", formatEdges(callers))

	// 3. Session.Check calls Authenticate (cross-file, method → package function)
	assert.True(t, hasCaller(callers, "handlers.Session.Check"),
		"Authenticate should be called by Session.Check, got: %v", formatEdges(callers))

	// 4. Setup has no calls
	setupCallees := graph.FindCallees("main.Setup")
	assert.Empty(t, setupCallees, "Setup should have no callees")
}

// TestBuildGraph_GoMethodCall verifies method calls like obj.Method().
func TestBuildGraph_GoMethodCall(t *testing.T) {
	fileA := `package service

type UserService struct{}

func (u *UserService) GetName(id int) string {
	return "user"
}
`
	fileB := `package api

import "myproject/service"

func Handler() {
	s := &service.UserService{}
	s.GetName(1)
}
`
	tmpDir := t.TempDir()

	svcDir := filepath.Join(tmpDir, "service")
	require.NoError(t, os.MkdirAll(svcDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(svcDir, "svc.go"), []byte(fileA), 0644))

	apiDir := filepath.Join(tmpDir, "api")
	require.NoError(t, os.MkdirAll(apiDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(apiDir, "api.go"), []byte(fileB), 0644))

	files := []types.FileEntry{
		{Path: "service/svc.go", AbsPath: filepath.Join(svcDir, "svc.go"), Language: types.LangGo},
		{Path: "api/api.go", AbsPath: filepath.Join(apiDir, "api.go"), Language: types.LangGo},
	}

	graph, err := BuildGraph(tmpDir, files, false)
	require.NoError(t, err)

	// Handler should call UserService.GetName
	callees := graph.FindCallees("api.Handler")
	assert.True(t, hasCallee(callees, "service.UserService.GetName"),
		"Handler should call UserService.GetName, got: %v", formatEdges(callees))
}

// TestGraph_SaveAndLoad verifies persistence round-trip.
func TestGraph_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	idxDir := filepath.Join(tmpDir, ".codefuse")

	graph := &Graph{Graph: types.Graph{
		Version:     types.IndexVersion,
		ProjectPath: tmpDir,
		Files: []types.FileEntry{
			{Path: "main.go", Language: types.LangGo},
		},
		Nodes: []types.Node{
			{ID: "main.Hello", Name: "Hello", Kind: types.KindFunction, File: "main.go", Line: 10},
			{ID: "main.World", Name: "World", Kind: types.KindFunction, File: "main.go", Line: 20},
		},
		Edges: []types.Edge{
			{From: "main.Hello", To: "main.World", Kind: types.EdgeKindCalls, File: "main.go", Line: 11},
		},
	}}

	err := graph.Save(idxDir)
	require.NoError(t, err)

	loaded, err := LoadGraph(idxDir)
	require.NoError(t, err)
	assert.Equal(t, graph.Version, loaded.Version)
	assert.Equal(t, graph.ProjectPath, loaded.ProjectPath)
	assert.Len(t, loaded.Nodes, 2)
	assert.Len(t, loaded.Edges, 1)

	// Verify runtime indexes are rebuilt
	node := loaded.FindNodeByID("main.Hello")
	require.NotNil(t, node)
	assert.Equal(t, "Hello", node.Name)

	callees := loaded.FindCallees("main.Hello")
	assert.Len(t, callees, 1)
	assert.Equal(t, "main.World", callees[0].To)
}

// TestGraph_FindNodeByName verifies name-based search.
func TestGraph_FindNodeByName(t *testing.T) {
	graph := &Graph{Graph: types.Graph{
		Nodes: []types.Node{
			{ID: "pkg.A", Name: "A", Kind: types.KindFunction, File: "a.go"},
			{ID: "pkg.B", Name: "A", Kind: types.KindStruct, File: "b.go"},
			{ID: "pkg.C", Name: "B", Kind: types.KindFunction, File: "c.go"},
		},
	}}
	graph.BuildIndexes()

	results := graph.FindNodeByName("A", "")
	assert.Len(t, results, 2)

	results = graph.FindNodeByName("A", types.KindFunction)
	assert.Len(t, results, 1)
	assert.Equal(t, types.KindFunction, results[0].Kind)
}

// TestGraph_VersionField verifies version is written to index.
func TestGraph_VersionField(t *testing.T) {
	tmpDir := t.TempDir()
	idxDir := filepath.Join(tmpDir, ".codefuse")

	graph := NewGraph(tmpDir)
	graph.Nodes = []types.Node{
		{ID: "main.foo", Name: "foo", Kind: types.KindFunction, File: "main.go"},
	}
	err := graph.Save(idxDir)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(idxDir, "graph.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), `"version"`)
	assert.Contains(t, string(data), types.IndexVersion)
}

// --- helpers ---

func hasCaller(edges []types.Edge, callerID string) bool {
	for _, e := range edges {
		if e.From == callerID {
			return true
		}
	}
	return false
}

func hasCallee(edges []types.Edge, calleeID string) bool {
	for _, e := range edges {
		if e.To == calleeID {
			return true
		}
	}
	return false
}

func formatEdges(edges []types.Edge) []string {
	out := make([]string, len(edges))
	for i, e := range edges {
		out[i] = e.From + " --" + e.Kind + "-> " + e.To
	}
	return out
}

// TestBuildGraph_PythonKindClassification verifies that module-level defs after
// a class are classified as KindFunction, not KindMethod.
func TestBuildGraph_PythonKindClassification(t *testing.T) {
	fileA := `class MyClass:
    def method_a(self):
        pass

# After class ends, this should be a function
def top_level():
    pass
`

	tmpDir := t.TempDir()
	aPath := filepath.Join(tmpDir, "a.py")
	require.NoError(t, os.WriteFile(aPath, []byte(fileA), 0644))

	files := []types.FileEntry{
		{Path: "a.py", AbsPath: aPath, Language: types.LangPython},
	}

	graph, err := BuildGraph(tmpDir, files, false)
	require.NoError(t, err)

	// method_a should be KindMethod inside MyClass
	methodA := graph.FindNodeByName("method_a", "")
	require.Len(t, methodA, 1)
	assert.Equal(t, types.KindMethod, methodA[0].Kind)
	assert.Equal(t, "MyClass", methodA[0].Parent)

	// top_level should be KindFunction, NOT KindMethod
	topLevel := graph.FindNodeByName("top_level", "")
	require.Len(t, topLevel, 1)
	assert.Equal(t, types.KindFunction, topLevel[0].Kind)
	assert.Empty(t, topLevel[0].Parent)
}

// TestBuildGraph_PythonCallGraph verifies that Python regex-based call graph
// extraction produces edges for same-file calls.
func TestBuildGraph_PythonCallGraph(t *testing.T) {
	fileA := `def foo():
    bar()
    baz()

def bar():
    pass

def baz():
    foo()
`

	tmpDir := t.TempDir()
	aPath := filepath.Join(tmpDir, "a.py")
	require.NoError(t, os.WriteFile(aPath, []byte(fileA), 0644))

	files := []types.FileEntry{
		{Path: "a.py", AbsPath: aPath, Language: types.LangPython},
	}

	graph, err := BuildGraph(tmpDir, files, false)
	require.NoError(t, err)

	// Should have 3 nodes and at least 3 edges (foo->bar, foo->baz, baz->foo)
	assert.GreaterOrEqual(t, len(graph.Nodes), 3, "expected at least 3 nodes")
	assert.GreaterOrEqual(t, len(graph.Edges), 3, "expected at least 3 call edges")

	// Verify specific edges exist
	fooID := graph.FindNodeByName("foo", "")[0].ID
	barID := graph.FindNodeByName("bar", "")[0].ID
	bazID := graph.FindNodeByName("baz", "")[0].ID

	assert.True(t, hasEdge(graph.Edges, fooID, barID), "expected foo -> bar")
	assert.True(t, hasEdge(graph.Edges, fooID, bazID), "expected foo -> baz")
	assert.True(t, hasEdge(graph.Edges, bazID, fooID), "expected baz -> foo")
}

func hasEdge(edges []types.Edge, fromID, toID string) bool {
	for _, e := range edges {
		if e.From == fromID && e.To == toID {
			return true
		}
	}
	return false
}
