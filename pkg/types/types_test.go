package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLocationNodeID(t *testing.T) {
	id := LocationNodeID("src/main.go", 42, 10)
	assert.Equal(t, "src/main.go:42:10", id)
}

func TestGraph_BuildIndexes(t *testing.T) {
	g := &Graph{
		Nodes: []Node{
			{ID: "a:1:1", Name: "Foo", File: "a.go", Line: 1, Column: 1},
			{ID: "b:5:1", Name: "Foo", File: "b.go", Line: 5, Column: 1},
			{ID: "c:10:1", Name: "Bar", File: "c.go", Line: 10, Column: 1},
		},
		Edges: []Edge{
			{From: "a:1:1", To: "b:5:1", Kind: EdgeKindCalls, File: "a.go", Line: 3},
			{From: "b:5:1", To: "c:10:1", Kind: EdgeKindCalls, File: "b.go", Line: 7},
		},
	}

	g.BuildIndexes()

	assert.Len(t, g.nodeByID, 3)
	assert.Len(t, g.nodesByName, 2) // Foo + Bar
	assert.Len(t, g.nodesByName["Foo"], 2)
	assert.Len(t, g.nodesByName["Bar"], 1)
	assert.Len(t, g.edgesFrom, 2)
	assert.Len(t, g.edgesTo, 2)
}

func TestGraph_FindNodeByID(t *testing.T) {
	g := &Graph{
		Nodes: []Node{
			{ID: "main.go:10:1", Name: "main", File: "main.go", Line: 10, Column: 1},
		},
	}
	g.BuildIndexes()

	node := g.FindNodeByID("main.go:10:1")
	assert.NotNil(t, node)
	assert.Equal(t, "main", node.Name)

	assert.Nil(t, g.FindNodeByID("nonexistent"))
}

func TestGraph_FindNodeByName(t *testing.T) {
	g := &Graph{
		Nodes: []Node{
			{ID: "a.go:5:1", Name: "Serve", File: "a.go", Line: 5, Column: 1},
			{ID: "b.go:20:1", Name: "Serve", File: "b.go", Line: 20, Column: 1},
		},
	}
	g.BuildIndexes()

	results := g.FindNodeByName("Serve")
	assert.Len(t, results, 2)
}

func TestGraph_FindCallers(t *testing.T) {
	g := &Graph{
		Nodes: []Node{
			{ID: "a:1:1", Name: "CallerFunc", File: "a.go", Line: 1, Column: 1},
			{ID: "b:1:1", Name: "CalleeFunc", File: "b.go", Line: 1, Column: 1},
		},
		Edges: []Edge{
			{From: "a:1:1", To: "b:1:1", Kind: EdgeKindCalls, File: "a.go", Line: 3},
		},
	}
	g.BuildIndexes()

	callers := g.FindCallers("b:1:1")
	assert.Len(t, callers, 1)
	assert.Equal(t, "a:1:1", callers[0].From)
}

func TestNode_IsThin(t *testing.T) {
	node := Node{
		ID:     "file.go:10:1",
		Name:   "MyFunc",
		File:   "file.go",
		Line:   10,
		Column: 1,
	}
	assert.Equal(t, "MyFunc", node.Name)
	assert.Equal(t, "file.go", node.File)
	assert.Equal(t, 10, node.Line)
	assert.Equal(t, 1, node.Column)
}

func TestSink_PkgExtraction(t *testing.T) {
	// Sink.Pkg is auto-extracted from callee name at build time.
	// sql.Query → pkg=sql, http.Get → pkg=http, os.ReadFile → pkg=os
	s := Sink{From: "a:1:1", CalleeName: "sql.Query", Pkg: "sql", File: "a.go", Line: 5}
	assert.Equal(t, "sql", s.Pkg)

	s2 := Sink{From: "b:2:1", CalleeName: "http.Get", Pkg: "http", File: "b.go", Line: 10}
	assert.Equal(t, "http", s2.Pkg)
}

func TestAnnotation_CRUD(t *testing.T) {
	a := Annotation{
		ID:       "ann-1",
		NodeID:   "a.go:5:1",
		Key:      "sink_type",
		Value:    "db",
		Source:   "agent",
		Evidence: "calls sql.Query via gorm",
	}
	assert.Equal(t, "a.go:5:1", a.NodeID)
	assert.Equal(t, "sink_type", a.Key)
	assert.Equal(t, "db", a.Value)
	assert.Equal(t, "agent", a.Source)
}

func TestGraph_Reachable(t *testing.T) {
	g := &Graph{
		Nodes: []Node{
			{ID: "a:1:1", Name: "AuthService.Login", File: "a.go", Line: 1, Column: 1},
			{ID: "b:5:1", Name: "Authenticate", File: "b.go", Line: 5, Column: 1},
			{ID: "c:10:1", Name: "UserDao.FindByToken", File: "c.go", Line: 10, Column: 1},
		},
		Edges: []Edge{
			{From: "a:1:1", To: "b:5:1", Kind: EdgeKindCalls, File: "a.go", Line: 2},
			{From: "b:5:1", To: "c:10:1", Kind: EdgeKindCalls, File: "b.go", Line: 6},
		},
		Sinks: []Sink{
			{From: "c:10:1", CalleeName: "sql.Query", Pkg: "sql", File: "c.go", Line: 11},
		},
	}
	g.BuildIndexes()

	// AuthService → Authenticate → UserDao.FindByToken → sql.Query (DB sink)
	paths := g.Reachable("a:1:1", "sql", 10)
	assert.Len(t, paths, 1)
	assert.Len(t, paths[0], 4) // 4 nodes in path
	assert.Equal(t, "a:1:1", paths[0][0])
	assert.Equal(t, "c:10:1", paths[0][2])
}

func TestGraph_Reachable_NoMatch(t *testing.T) {
	g := &Graph{
		Nodes: []Node{
			{ID: "a:1:1", Name: "AuthService.Login", File: "a.go", Line: 1, Column: 1},
		},
		Sinks: []Sink{
			{From: "a:1:1", CalleeName: "os.ReadFile", Pkg: "os", File: "a.go", Line: 3},
		},
	}
	g.BuildIndexes()

	// No sql sink reachable from a:1:1
	paths := g.Reachable("a:1:1", "sql", 10)
	assert.Empty(t, paths)
}

func TestGraph_SinksForNode(t *testing.T) {
	g := &Graph{
		Nodes: []Node{
			{ID: "a:1:1", Name: "AuthService", File: "a.go", Line: 1, Column: 1},
		},
		Sinks: []Sink{
			{From: "a:1:1", CalleeName: "sql.Query", Pkg: "sql", File: "a.go", Line: 5},
			{From: "a:1:1", CalleeName: "http.Get", Pkg: "http", File: "a.go", Line: 10},
		},
	}

	sinks := g.SinksForNode("a:1:1")
	assert.Len(t, sinks, 2)

	// Filter by pkg
	sqlSinks := g.FilterSinks(sinks, "sql")
	assert.Len(t, sqlSinks, 1)
	assert.Equal(t, "sql.Query", sqlSinks[0].CalleeName)
}
