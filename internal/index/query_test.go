package index

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yifanmeng/codefuse/pkg/types"
)

func TestGraph_Query_ExactMatch(t *testing.T) {
	graph := &Graph{Graph: types.Graph{
		Nodes: []types.Node{
			{ID: "a", Name: "Hello", Kind: types.KindFunction, File: "a.go"},
			{ID: "b", Name: "HelloWorld", Kind: types.KindFunction, File: "b.go"},
			{ID: "c", Name: "Hello", Kind: types.KindStruct, File: "c.go"},
		},
	}}
	graph.buildTrie()

	results := graph.Query("Hello", "")
	assert.Len(t, results, 2)

	results = graph.Query("Hello", types.KindFunction)
	assert.Len(t, results, 1)
	assert.Equal(t, "Hello", results[0].Name)
}

func TestGraph_Query_PrefixMatch(t *testing.T) {
	graph := &Graph{Graph: types.Graph{
		Nodes: []types.Node{
			{ID: "a", Name: "Authenticate", Kind: types.KindFunction, File: "a.go"},
			{ID: "b", Name: "Auth", Kind: types.KindFunction, File: "b.go"},
			{ID: "c", Name: "Authorize", Kind: types.KindFunction, File: "c.go"},
			{ID: "d", Name: "Add", Kind: types.KindFunction, File: "d.go"},
		},
	}}
	graph.buildTrie()

	// Prefix query "Auth*"
	results := graph.Query("Auth*", "")
	assert.Len(t, results, 3)
	names := make([]string, len(results))
	for i, r := range results {
		names[i] = r.Name
	}
	assert.ElementsMatch(t, []string{"Authenticate", "Auth", "Authorize"}, names)
}

func TestGraph_Query_GlobMatch(t *testing.T) {
	graph := &Graph{Graph: types.Graph{
		Nodes: []types.Node{
			{ID: "a", Name: "FooBar", Kind: types.KindFunction, File: "a.go"},
			{ID: "b", Name: "FooBaz", Kind: types.KindFunction, File: "b.go"},
			{ID: "c", Name: "Foo", Kind: types.KindFunction, File: "c.go"},
		},
	}}
	graph.buildTrie()

	// Glob query "FooB??"
	results := graph.Query("FooB??", "")
	assert.Len(t, results, 2)
}

func TestGraph_Query_NoMatch(t *testing.T) {
	graph := &Graph{Graph: types.Graph{
		Nodes: []types.Node{
			{ID: "a", Name: "Foo", Kind: types.KindFunction, File: "a.go"},
		},
	}}
	graph.buildTrie()

	results := graph.Query("Bar", "")
	assert.Empty(t, results)
}

// BenchmarkGraph_Query compares prefix trie vs linear scan.
func BenchmarkGraph_Query_Trie(b *testing.B) {
	var nodes []types.Node
	for i := 0; i < 10000; i++ {
		name := "Func" + string(rune('A'+i%26)) + string(rune('0'+i%10))
		nodes = append(nodes, types.Node{
			ID:   name,
			Name: name,
			Kind: types.KindFunction,
			File: "file.go",
		})
	}
	graph := &Graph{Graph: types.Graph{Nodes: nodes}}
	graph.buildTrie()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		graph.Query("FuncA*", "")
	}
}

func BenchmarkGraph_Query_Linear(b *testing.B) {
	var nodes []types.Node
	for i := 0; i < 10000; i++ {
		name := "Func" + string(rune('A'+i%26)) + string(rune('0'+i%10))
		nodes = append(nodes, types.Node{
			ID:   name,
			Name: name,
			Kind: types.KindFunction,
			File: "file.go",
		})
	}
	graph := &Graph{Graph: types.Graph{Nodes: nodes}}
	graph.buildTrie()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Force linear scan by using a glob that can't use trie
		graph.Query("Func?1", "")
	}
}
