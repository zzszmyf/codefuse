package types

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNode_Serialization(t *testing.T) {
	node := Node{
		ID:        "main.Hello",
		Name:      "Hello",
		Kind:      KindFunction,
		File:      "main.go",
		Line:      10,
		Column:    1,
		EndLine:   12,
		Signature: "func Hello(name string) string",
		Docstring: "Hello greets someone.",
	}

	data, err := json.Marshal(node)
	require.NoError(t, err)

	// Should contain all fields
	var decoded Node
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, node.ID, decoded.ID)
	assert.Equal(t, node.Name, decoded.Name)
	assert.Equal(t, node.Kind, decoded.Kind)
	assert.Equal(t, node.File, decoded.File)
	assert.Equal(t, node.Line, decoded.Line)
	assert.Equal(t, node.Column, decoded.Column)
	assert.Equal(t, node.EndLine, decoded.EndLine)
	assert.Equal(t, node.Signature, decoded.Signature)
	assert.Equal(t, node.Docstring, decoded.Docstring)
}

func TestEdge_Serialization(t *testing.T) {
	edge := Edge{
		From: "main.Greet",
		To:   "main.Hello",
		Kind: EdgeKindCalls,
		File: "main.go",
		Line: 15,
	}

	data, err := json.Marshal(edge)
	require.NoError(t, err)

	var decoded Edge
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, edge.From, decoded.From)
	assert.Equal(t, edge.To, decoded.To)
	assert.Equal(t, edge.Kind, decoded.Kind)
	assert.Equal(t, edge.File, decoded.File)
	assert.Equal(t, edge.Line, decoded.Line)
}

func TestGraph_Serialization(t *testing.T) {
	graph := Graph{
		Version:     IndexVersion,
		ProjectPath: "/tmp/test",
		Files: []FileEntry{
			{Path: "main.go", Language: LangGo},
		},
		Nodes: []Node{
			{ID: "main.Hello", Name: "Hello", Kind: KindFunction, File: "main.go", Line: 10},
			{ID: "main.Greet", Name: "Greet", Kind: KindMethod, File: "main.go", Line: 20, Parent: "User"},
		},
		Edges: []Edge{
			{From: "main.Greet", To: "main.Hello", Kind: EdgeKindCalls, File: "main.go", Line: 21},
		},
	}

	data, err := json.MarshalIndent(graph, "", "  ")
	require.NoError(t, err)

	var decoded Graph
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, graph.Version, decoded.Version)
	assert.Equal(t, graph.ProjectPath, decoded.ProjectPath)
	assert.Len(t, decoded.Files, 1)
	assert.Len(t, decoded.Nodes, 2)
	assert.Len(t, decoded.Edges, 1)

	// Runtime indexes should not be serialized
	assert.Nil(t, decoded.nodeByID)
	assert.Nil(t, decoded.edgesFrom)
	assert.Nil(t, decoded.edgesTo)
}

func TestNodeID_GoPackageQualified(t *testing.T) {
	// Global function: package.Function
	id := GoNodeID("mypkg", "", "DoSomething")
	assert.Equal(t, "mypkg.DoSomething", id)

	// Method: package.Receiver.Method
	id = GoNodeID("mypkg", "User", "Greet")
	assert.Equal(t, "mypkg.User.Greet", id)

	// Method with pointer receiver should strip *
	id = GoNodeID("mypkg", "*User", "Greet")
	assert.Equal(t, "mypkg.User.Greet", id)
}

func TestNodeID_LocationQualified(t *testing.T) {
	id := LocationNodeID("src/main.go", 10, 5)
	assert.Equal(t, "src/main.go:10:5", id)
}

func TestSymbolToNode_Conversion(t *testing.T) {
	sym := Symbol{
		Name:      "Hello",
		Kind:      KindFunction,
		File:      "main.go",
		Line:      10,
		Column:    1,
		EndLine:   12,
		Signature: "func Hello()",
		Docstring: "Says hello.",
	}

	node := sym.ToNode("main.Hello")
	assert.Equal(t, "main.Hello", node.ID)
	assert.Equal(t, "Hello", node.Name)
	assert.Equal(t, KindFunction, node.Kind)
	assert.Equal(t, "main.go", node.File)
	assert.Equal(t, 10, node.Line)
	assert.Equal(t, 1, node.Column)
	assert.Equal(t, 12, node.EndLine)
	assert.Equal(t, "func Hello()", node.Signature)
	assert.Equal(t, "Says hello.", node.Docstring)
}
