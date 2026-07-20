// Package index provides the thin index builder and query engine.
//
// The index stores only name→position mappings. Symbol details are read from
// actual source files on demand — code is the single source of truth.
package index

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yifanmeng/codefuse/internal/parser"
	"github.com/yifanmeng/codefuse/pkg/types"
)

func init() {
	// Register types for gob encoding.
	gob.Register(types.Graph{})
	gob.Register(types.Node{})
	gob.Register(types.Edge{})
	gob.Register(types.FileEntry{})
}

// Graph wraps types.Graph with a prefix trie for fast name lookup.
type Graph struct {
	types.Graph
	nameTrie *symbolTrie
}

// NewGraph creates an empty graph.
func NewGraph(projectPath string) *Graph {
	return &Graph{
		Graph: types.Graph{
			Version:     types.IndexVersion,
			ProjectPath: projectPath,
			Files:       make([]types.FileEntry, 0),
			Nodes:       make([]types.Node, 0),
			Edges:       make([]types.Edge, 0),
		},
	}
}

// BuildGraph builds a thin index from scanned files.
//
// Process:
//  1. Extract nodes + raw edges from all files via tree-sitter (batch mode).
//  2. Build runtime indexes (nodeByID, nodesByName, trie).
//  3. Resolve edge targets (callee names → node IDs).
//  4. Save manifest for incremental indexing.
func BuildGraph(projectPath string, files []types.FileEntry) (*Graph, error) {
	graph := NewGraph(projectPath)
	graph.Files = files

	// Phase 1: Extract nodes and raw edges via tree-sitter batch.
	nodesByFile, edgesByFile, failed := parser.ExtractBatch(files)

	// For failed files, try individual extraction.
	for _, f := range failed {
		nodes, edges, _ := parser.ExtractFile(f.AbsPath, f.Path, f.Language)
		if len(nodes) > 0 {
			nodesByFile[f.Path] = nodes
		}
		if len(edges) > 0 {
			edgesByFile[f.Path] = edges
		}
	}

	// Collect all nodes.
	for _, nodes := range nodesByFile {
		graph.Nodes = append(graph.Nodes, nodes...)
	}

	// Build indexes for edge resolution.
	graph.BuildIndexes()
	graph.buildTrie()

	// Phase 2: Resolve edge targets.
	// Raw edges have callee names in the To field; resolve to node IDs.
	for _, edges := range edgesByFile {
		for _, edge := range edges {
			resolved := resolveEdge(edge, &graph.Graph)
			graph.Edges = append(graph.Edges, resolved...)
		}
	}

	// Rebuild indexes with edges included.
	graph.BuildIndexes()

	// Save manifest.
	saveManifestForGraph(projectPath, files, types.IndexVersion)

	return graph, nil
}

// resolveEdge resolves a raw edge (callee name → node IDs).
// Only creates edges for same-file matches to avoid cross-file explosion.
// Cross-file resolution requires import analysis and is deferred to query time.
func resolveEdge(edge types.Edge, g *types.Graph) []types.Edge {
	calleeName := edge.To // raw edge stores callee NAME, not ID
	callerFile := edge.File

	// Find callee nodes matching the name.
	candidates := g.FindNodeByName(calleeName)
	if len(candidates) == 0 {
		return nil
	}

	// Priority 1: Same file (most reliable).
	for _, callee := range candidates {
		if callee.File == callerFile {
			return []types.Edge{{
				From: edge.From,
				To:   callee.ID,
				Kind: edge.Kind,
				File: edge.File,
				Line: edge.Line,
			}}
		}
	}

	// Priority 2: Same directory.
	callerDir := filepath.Dir(callerFile)
	for _, callee := range candidates {
		if filepath.Dir(callee.File) == callerDir {
			return []types.Edge{{
				From: edge.From,
				To:   callee.ID,
				Kind: edge.Kind,
				File: edge.File,
				Line: edge.Line,
			}}
		}
	}

	// No cross-directory edges — too unreliable without import analysis.
	return nil
}

// =============================================================================
// Persistence — split nodes (grep queries) and edges (call-graph queries)
// =============================================================================

// nodeData is the node-only subset of Graph, used for fast grep queries.
// Edges are stored separately and only loaded for --callers/--callees.
type nodeData struct {
	Version     string
	ProjectPath string
	Files       []types.FileEntry
	Nodes       []types.Node
}

// Save writes the graph: nodes.gob (fast grep), optional edges.gob, and graph.json (debug).
func (g *Graph) Save(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Nodes only — fast load for grep queries.
	nd := nodeData{
		Version:     g.Version,
		ProjectPath: g.ProjectPath,
		Files:       g.Files,
		Nodes:       g.Nodes,
	}
	f, err := os.Create(filepath.Join(dir, "nodes.gob"))
	if err != nil {
		return err
	}
	if err := gob.NewEncoder(f).Encode(nd); err != nil {
		f.Close()
		return err
	}
	f.Close()

	// Edges only — loaded on demand for --callers/--callees.
	if len(g.Edges) > 0 {
		ef, err := os.Create(filepath.Join(dir, "edges.gob"))
		if err != nil {
			return err
		}
		if err := gob.NewEncoder(ef).Encode(g.Edges); err != nil {
			ef.Close()
			return err
		}
		ef.Close()
	}

	// Full JSON (debug / backward compat).
	data, _ := json.MarshalIndent(g.Graph, "", "  ")
	os.WriteFile(filepath.Join(dir, "graph.json"), data, 0644)

	return nil
}

// LoadGraphNodes loads only nodes (not edges) — the fast path for grep queries.
func LoadGraphNodes(dir string) (*Graph, error) {
	// Try binary nodes first.
	nodesPath := filepath.Join(dir, "nodes.gob")
	if data, err := os.ReadFile(nodesPath); err == nil {
		return loadNodesGob(data)
	}
	// Fall back to legacy graph.gob or graph.json.
	return LoadGraph(dir)
}

// LoadGraph loads the full index including edges — for --callers/--callees queries.
func LoadGraph(dir string) (*Graph, error) {
	// Try split format (nodes.gob + edges.gob).
	nodesPath := filepath.Join(dir, "nodes.gob")
	edgesPath := filepath.Join(dir, "edges.gob")
	if nodesData, err := os.ReadFile(nodesPath); err == nil {
		graph, err := loadNodesGob(nodesData)
		if err != nil {
			return nil, err
		}
		// Attach edges if available.
		if edgesData, eErr := os.ReadFile(edgesPath); eErr == nil {
			var edges []types.Edge
			if err := gob.NewDecoder(bytes.NewReader(edgesData)).Decode(&edges); err == nil {
				graph.Edges = edges
				graph.BuildIndexes() // rebuild with edges
			}
		}
		return graph, nil
	}

	// Fall back to legacy full graph.gob.
	gobPath := filepath.Join(dir, "graph.gob")
	if data, err := os.ReadFile(gobPath); err == nil {
		return loadFullGob(data)
	}

	// Last resort: JSON.
	data, err := os.ReadFile(filepath.Join(dir, "graph.json"))
	if err != nil {
		return nil, err
	}
	return loadJSON(data)
}

func loadNodesGob(data []byte) (*Graph, error) {
	var nd nodeData
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&nd); err != nil {
		return nil, fmt.Errorf("gob decode nodes: %w", err)
	}
	if nd.Version != "" && nd.Version != types.IndexVersion {
		return nil, fmt.Errorf("index v%s (expected v%s). Re-index", nd.Version, types.IndexVersion)
	}
	graph := &Graph{
		Graph: types.Graph{
			Version:     nd.Version,
			ProjectPath: nd.ProjectPath,
			Files:       nd.Files,
			Nodes:       nd.Nodes,
		},
	}
	graph.BuildIndexes()
	graph.buildTrie()
	return graph, nil
}

func loadFullGob(data []byte) (*Graph, error) {
	var tg types.Graph
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&tg); err != nil {
		return nil, fmt.Errorf("gob decode: %w", err)
	}
	if tg.Version != "" && tg.Version != types.IndexVersion {
		return nil, fmt.Errorf("index v%s (expected v%s). Re-index with 'codefuse index .'", tg.Version, types.IndexVersion)
	}
	tg.BuildIndexes()
	graph := &Graph{Graph: tg}
	graph.buildTrie()
	return graph, nil
}

func loadJSON(data []byte) (*Graph, error) {
	var tg types.Graph
	if err := json.Unmarshal(data, &tg); err != nil {
		return nil, err
	}
	if tg.Version != "" && tg.Version != types.IndexVersion {
		return nil, fmt.Errorf("index v%s (expected v%s). Re-index with 'codefuse index .'", tg.Version, types.IndexVersion)
	}
	tg.BuildIndexes()
	graph := &Graph{Graph: tg}
	graph.buildTrie()
	return graph, nil
}

// =============================================================================
// Trie-based query
// =============================================================================

// BuildTrie rebuilds the prefix trie from current nodes. Public for watch mode.
func (g *Graph) BuildTrie() {
	g.nameTrie = newSymbolTrie()
	for i := range g.Nodes {
		node := &g.Nodes[i]
		g.nameTrie.Insert(node.Name, node.ID)
	}
}

// buildTrie is kept for backward compatibility within the package.
func (g *Graph) buildTrie() {
	g.BuildTrie()
}

// FindNodeByPrefix returns nodes whose names start with prefix.
func (g *Graph) FindNodeByPrefix(prefix string) []types.Node {
	if g.nameTrie == nil {
		g.buildTrie()
	}
	ids := g.nameTrie.FindPrefix(prefix)
	var results []types.Node
	for _, id := range ids {
		node := g.FindNodeByID(id)
		if node != nil {
			results = append(results, *node)
		}
	}
	return results
}

// FindNodeGlob returns nodes matching a glob pattern.
func (g *Graph) FindNodeGlob(pattern string) []types.Node {
	var results []types.Node
	for _, node := range g.Nodes {
		matched, _ := filepath.Match(pattern, node.Name)
		if matched {
			results = append(results, node)
		}
	}
	return results
}

// Query is the smart entry point for symbol lookup.
// Auto-detects: exact → prefix ("foo*") → glob ("*bar", "b?z").
func (g *Graph) Query(name string) []types.Node {
	if name == "" {
		return nil
	}

	// 1. Prefix pattern: "foo*" (only trailing wildcard).
	if strings.HasSuffix(name, "*") && !strings.ContainsAny(name[:len(name)-1], "*?[") {
		return g.FindNodeByPrefix(name[:len(name)-1])
	}

	// 2. Glob pattern: *, ?, [...]
	if strings.ContainsAny(name, "*?[") {
		return g.FindNodeGlob(name)
	}

	// 3. Exact match — case-insensitive.
	if results := g.findExact(name); len(results) > 0 {
		return results
	}

	// 4. Substring match — for real-world queries like "PageAttention"
	//    when the actual symbol is "PagedAttention".
	if results := g.findSubstring(name); len(results) > 0 {
		return results
	}

	// 5. Nothing found.
	return nil
}

// findExact returns nodes whose name equals the query (case-insensitive).
func (g *Graph) findExact(name string) []types.Node {
	var results []types.Node
	for _, node := range g.Nodes {
		if strings.EqualFold(node.Name, name) {
			results = append(results, node)
		}
	}
	return results
}

// findSubstring returns nodes whose name contains the query substring (case-insensitive).
func (g *Graph) findSubstring(name string) []types.Node {
	lower := strings.ToLower(name)
	var results []types.Node
	for _, node := range g.Nodes {
		if strings.Contains(strings.ToLower(node.Name), lower) {
			results = append(results, node)
		}
	}
	return results
}

// =============================================================================
// Call graph queries
// =============================================================================

// FindCallers returns the nodes that call the given node ID.
func (g *Graph) FindCallers(nodeID string) []EdgeWithNode {
	edges := g.Graph.FindCallers(nodeID)
	var results []EdgeWithNode
	for _, e := range edges {
		caller := g.FindNodeByID(e.From)
		if caller != nil {
			results = append(results, EdgeWithNode{Edge: e, Node: *caller})
		}
	}
	return results
}

// FindCallees returns the nodes called by the given node ID.
func (g *Graph) FindCallees(nodeID string) []EdgeWithNode {
	edges := g.Graph.FindCallees(nodeID)
	var results []EdgeWithNode
	for _, e := range edges {
		callee := g.FindNodeByID(e.To)
		if callee != nil {
			results = append(results, EdgeWithNode{Edge: e, Node: *callee})
		}
	}
	return results
}

// EdgeWithNode pairs an edge with its related node for display.
type EdgeWithNode struct {
	Edge types.Edge
	Node types.Node
}

// =============================================================================
// Manifest (incremental indexing support)
// =============================================================================

// Manifest tracks file hashes for incremental indexing.
type Manifest struct {
	Version string            `json:"version"`
	Files   map[string]int64 `json:"files"` // path → mtime
}

func saveManifestForGraph(projectPath string, files []types.FileEntry, version string) {
	m := &Manifest{
		Version: version,
		Files:   make(map[string]int64),
	}
	for _, f := range files {
		m.Files[f.Path] = f.Mtime
	}
	indexDir := filepath.Join(projectPath, ".codefuse")
	_ = os.MkdirAll(indexDir, 0755)

	data, _ := json.MarshalIndent(m, "", "  ")
	_ = os.WriteFile(filepath.Join(indexDir, "manifest.json"), data, 0644)
}

// LoadManifest reads the manifest from disk.
func LoadManifest(indexDir string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(indexDir, "manifest.json"))
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// =============================================================================
// Source file reader (for live, truth-source lookups)
// =============================================================================

// ReadLine reads a specific line from a source file.
// Used at query time to extract current symbol details from actual code.
func ReadLine(absPath string, line int) (string, error) {
	content, err := os.ReadFile(absPath)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(content), "\n")
	if line < 1 || line > len(lines) {
		return "", fmt.Errorf("line %d out of range", line)
	}
	return strings.TrimSpace(lines[line-1]), nil
}

// ReadLines reads a range of lines [start, end] from a source file (1-based).
func ReadLines(absPath string, start, end int) ([]string, error) {
	content, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(content), "\n")
	if start < 1 {
		start = 1
	}
	if end > len(lines) {
		end = len(lines)
	}
	var result []string
	for i := start - 1; i < end; i++ {
		result = append(result, strings.TrimSpace(lines[i]))
	}
	return result, nil
}
