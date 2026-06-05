package fusefs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yifanmeng/codefuse/internal/index"
	"github.com/yifanmeng/codefuse/pkg/types"
)

func TestFormatSymbolContent(t *testing.T) {
	nodes := []types.Node{
		{ID: "pkg.Hello", Name: "Hello", Kind: types.KindFunction, File: "a.go", Line: 1, Signature: "func Hello()"},
		{ID: "pkg.Greeter.Hello", Name: "Hello", Kind: types.KindMethod, File: "b.go", Line: 5, Parent: "Greeter"},
	}
	content := formatSymbolContent("Hello", nodes)
	assert.Contains(t, content, "Symbol: Hello")
	assert.Contains(t, content, "a.go:1")
	assert.Contains(t, content, "b.go:5")
	assert.Contains(t, content, "Parent: Greeter")
}

func TestFormatOutlineContent(t *testing.T) {
	nodes := []types.Node{
		{ID: "pkg.main", Name: "main", Kind: types.KindFunction, File: "main.go", Line: 1},
		{ID: "pkg.Hello", Name: "Hello", Kind: types.KindFunction, File: "main.go", Line: 5},
	}
	content := formatOutlineContent("main.go", nodes)
	assert.Contains(t, content, "Outline: main.go")
	assert.Contains(t, content, "L001")
	assert.Contains(t, content, "L005")
	assert.Contains(t, content, "Hello")
}

func TestFormatReferencesContent(t *testing.T) {
	graph := &index.Graph{Graph: types.Graph{
		Nodes: []types.Node{
			{ID: "auth.Authenticate", Name: "Authenticate", Kind: types.KindFunction, File: "auth.go", Line: 5},
			{ID: "main.Login", Name: "Login", Kind: types.KindFunction, File: "main.go", Line: 3},
		},
		Edges: []types.Edge{
			{From: "main.Login", To: "auth.Authenticate", Kind: types.EdgeKindCalls, File: "main.go", Line: 4},
		},
	}}
	graph.BuildIndexes()

	nodes := []types.Node{
		{ID: "auth.Authenticate", Name: "Authenticate", Kind: types.KindFunction, File: "auth.go", Line: 5},
	}
	content := formatReferencesContent("Authenticate", nodes, graph)
	assert.Contains(t, content, "References: Authenticate")
	assert.Contains(t, content, "Login")
	assert.Contains(t, content, "main.go:4")
}

func TestSanitizeName(t *testing.T) {
	assert.Equal(t, "foo_bar", sanitizeName("foo/bar"))
	assert.Equal(t, "foo_bar", sanitizeName("foo\\bar"))
	assert.Equal(t, "foo_bar", sanitizeName("foo:bar"))
}

func TestVFSRoot_Readdir(t *testing.T) {
	graph := &index.Graph{Graph: types.Graph{
		Nodes: []types.Node{
			{ID: "pkg.main", Name: "main", Kind: types.KindFunction, File: "main.go", Line: 1},
		},
	}}

	root := NewVFSRoot(graph)
	require.NotNil(t, root)

	// OnAdd should create children
	// Note: full FUSE testing requires kernel module, so we test structure only
	assert.NotNil(t, root)
}
