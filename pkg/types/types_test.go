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
	// Thin nodes store only name + position. No kind, parent, signature, docstring.
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
	// No Kind, Parent, Signature, Docstring fields — they don't exist on the type.
}
