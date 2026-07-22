package index

import (
	"fmt"
	"testing"

	"github.com/yifanmeng/codefuse/pkg/types"
)

// BenchmarkQuery_Exact measures exact-match lookup speed.
func BenchmarkQuery_Exact(b *testing.B) {
	g := buildBenchGraph(5000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.Query("Symbol2500", false)
	}
}

// BenchmarkQuery_Prefix measures trie prefix lookup speed.
func BenchmarkQuery_Prefix(b *testing.B) {
	g := buildBenchGraph(5000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.Query("Symbol*", false)
	}
}

// BenchmarkQuery_Substring measures substring fallback speed.
func BenchmarkQuery_Substring(b *testing.B) {
	g := buildBenchGraph(5000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.Query("mbol25", false)
	}
}

// BenchmarkQuery_CamelCase measures CamelCase matching speed.
func BenchmarkQuery_CamelCase(b *testing.B) {
	g := buildBenchGraph(5000)
	// Add a CamelCase symbol.
	g.Nodes = append(g.Nodes, types.Node{
		ID: "x:1:1", Name: "PageAttention", File: "x.go", Line: 1, Column: 1,
	})
	g.BuildIndexes()
	g.BuildTrie()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.Query("PA", false)
	}
}

// BenchmarkBuildIndexes measures index building speed.
func BenchmarkBuildIndexes(b *testing.B) {
	g := buildBenchGraph(10000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.BuildIndexes()
	}
}

// BenchmarkBuildTrie measures trie construction speed.
func BenchmarkBuildTrie(b *testing.B) {
	g := buildBenchGraph(10000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.BuildTrie()
	}
}

// BenchmarkReachable measures BFS reachability analysis.
func BenchmarkReachable(b *testing.B) {
	g := buildBenchGraph(100)
	// Create a chain: node0 → node1 → ... → node99.
	for i := 0; i < 99; i++ {
		g.Edges = append(g.Edges, types.Edge{
			From: fmt.Sprintf("f%d.go:1:1", i),
			To:   fmt.Sprintf("f%d.go:1:1", i+1),
			Kind: types.EdgeKindCalls,
		})
	}
	// Add a sink at the end.
	g.Sinks = append(g.Sinks, types.Sink{
		From: "f99.go:1:1", CalleeName: "sql.Query", Pkg: "sql",
	})
	g.BuildIndexes()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.Graph.Reachable("f0.go:1:1", "sql", 100)
	}
}

// buildBenchGraph creates a graph with n synthetic symbols for benchmarking.
func buildBenchGraph(n int) *Graph {
	g := NewGraph("/bench")
	for i := 0; i < n; i++ {
		g.Nodes = append(g.Nodes, types.Node{
			ID:     fmt.Sprintf("f%d.go:1:1", i),
			Name:   fmt.Sprintf("Symbol%d", i),
			File:   fmt.Sprintf("f%d.go", i),
			Line:   1,
			Column: 1,
		})
	}
	g.BuildIndexes()
	g.BuildTrie()
	return g
}
