package index

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/yifanmeng/codefuse/internal/parser"
	"github.com/yifanmeng/codefuse/pkg/types"
)

// Graph wraps types.Graph to allow defining methods in this package.
type Graph struct {
	types.Graph
	nameTrie *symbolTrie // prefix index for fast name lookup
}

// NewGraph creates a new empty graph for the given project path.
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

// BuildGraph creates a Graph from scanned files, including call graph analysis.
// Two-phase process:
//   1. Extract all Nodes (symbols) from each file
//   2. Extract all Edges (call relationships) from each file
func BuildGraph(projectPath string, files []types.FileEntry, useTreeSitter bool) (*Graph, error) {
	graph := NewGraph(projectPath)
	graph.Files = files

	// Phase 1: Extract nodes from all files in parallel.
	nodes, pkgNames := buildNodesParallel(files)
	graph.Nodes = nodes

	// Build node lookup index for cross-reference resolution.
	graph.Graph.BuildIndexes()
	graph.buildTrie()

	// Phase 2: Extract edges (call graph) from all files in parallel.
	graph.Edges = buildEdgesParallel(files, pkgNames, &graph.Graph)

	// Rebuild indexes with edges included.
	graph.Graph.BuildIndexes()

	// Save manifest for incremental indexing.
	manifest := &Manifest{
		Version: types.IndexVersion,
		Files:   make(map[string]int64),
	}
	for _, f := range files {
		manifest.Files[f.Path] = f.Mtime
	}
	indexDir := filepath.Join(projectPath, ".codefuse")
	_ = os.MkdirAll(indexDir, 0755)
	_ = saveManifest(indexDir, manifest)

	return graph, nil
}

// Save writes the graph to disk.
func (g *Graph) Save(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(g.Graph, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "graph.json"), data, 0644)
}

// LoadGraph reads a graph from disk.
func LoadGraph(dir string) (*Graph, error) {
	// Check manifest version first
	manifest, err := loadManifest(dir)
	if err == nil && manifest.Version != "" && manifest.Version != types.IndexVersion {
		return nil, fmt.Errorf("index format version %s is incompatible (expected %s). Run 'codefuse index .' to re-index", manifest.Version, types.IndexVersion)
	}

	data, err := os.ReadFile(filepath.Join(dir, "graph.json"))
	if err != nil {
		return nil, err
	}
	var tg types.Graph
	if err := json.Unmarshal(data, &tg); err != nil {
		return nil, err
	}
	tg.BuildIndexes()
	graph := &Graph{Graph: tg}
	graph.buildTrie()
	return graph, nil
}

// LoadAny attempts to load a graph, falling back to v0.1 index format if needed.
// If only an old index.json exists, it is converted to a Graph (without edges).
func LoadAny(dir string) (*Graph, error) {
	// Try graph.json first (v0.2+)
	graph, err := LoadGraph(dir)
	if err == nil {
		return graph, nil
	}

	// Fall back to index.json (v0.1)
	idx, err := Load(dir)
	if err != nil {
		return nil, fmt.Errorf("no index found. Run 'codefuse index <path>' first")
	}

	// Convert v0.1 Index to v0.2 Graph.
	converted := NewGraph(idx.ProjectPath)
	converted.Files = idx.Files
	converted.Nodes = make([]types.Node, len(idx.Symbols))
	for i, sym := range idx.Symbols {
		converted.Nodes[i] = sym.ToNode(types.LocationNodeID(sym.File, sym.Line, sym.Column))
	}
	converted.BuildIndexes()
	converted.buildTrie()
	return converted, nil
}

// =============================================================================
// Trie-based prefix lookup
// =============================================================================

// buildTrie builds the prefix trie from all nodes.
func (g *Graph) buildTrie() {
	g.nameTrie = newSymbolTrie()
	for i := range g.Nodes {
		node := &g.Nodes[i]
		g.nameTrie.Insert(node.Name, node.ID)
	}
}

// FindNodeByPrefix returns nodes whose names start with the given prefix,
// optionally filtered by kind. Uses the trie index for O(m + k) performance.
func (g *Graph) FindNodeByPrefix(prefix, kind string) []types.Node {
	if g.nameTrie == nil {
		g.buildTrie()
	}
	ids := g.nameTrie.FindPrefix(prefix)
	var results []types.Node
	for _, id := range ids {
		node := g.FindNodeByID(id)
		if node != nil {
			if kind == "" || node.Kind == kind {
				results = append(results, *node)
			}
		}
	}
	return results
}

// FindNodeGlob returns nodes matching a glob pattern (*, ?, [abc]).
func (g *Graph) FindNodeGlob(pattern, kind string) []types.Node {
	var results []types.Node
	for _, node := range g.Nodes {
		matched, _ := path.Match(pattern, node.Name)
		if matched {
			if kind == "" || node.Kind == kind {
				results = append(results, node)
			}
		}
	}
	return results
}

// Query is the smart entry point for symbol lookup.
// Auto-detects query type: exact | prefix (foo*) | glob (*bar, b?r).
func (g *Graph) Query(name, kind string) []types.Node {
	if name == "" {
		return nil
	}
	// Prefix pattern: "foo*" (only trailing wildcard, no others)
	if strings.HasSuffix(name, "*") && !strings.ContainsAny(name[:len(name)-1], "*?[") {
		return g.FindNodeByPrefix(name[:len(name)-1], kind)
	}
	// Glob pattern
	if strings.ContainsAny(name, "*?[") {
		return g.FindNodeGlob(name, kind)
	}
	// Exact match
	return g.FindNodeByName(name, kind)
}

// extractNodes extracts nodes (symbols) from a single file.
// Returns the nodes, the package name (for Go), and any error.
func extractNodes(file types.FileEntry) ([]types.Node, string, error) {
	switch file.Language {
	case types.LangGo:
		content, err := os.ReadFile(file.AbsPath)
		if err != nil {
			return nil, "", err
		}
		return parser.ExtractGoNodes(file.Path, content)
	}

	// Try tree-sitter CLI for non-Go languages.
	if syms, err := parser.ExtractWithTreeSitter(file.AbsPath, file.Path, file.Language); err == nil && len(syms) > 0 {
		nodes := make([]types.Node, len(syms))
		for i, sym := range syms {
			nodes[i] = sym.ToNode(types.LocationNodeID(file.Path, sym.Line, sym.Column))
		}
		return nodes, "", nil
	}

	// Fallback to regex.
	content, err := os.ReadFile(file.AbsPath)
	if err != nil {
		return nil, "", err
	}
	var syms []types.Symbol
	switch file.Language {
	case types.LangPython:
		syms, _ = extractPythonSymbols(file.Path, string(content))
	case types.LangRust:
		syms, _ = extractRustSymbols(file.Path, string(content))
	case types.LangJS, types.LangTS:
		syms, _ = extractJSSymbols(file.Path, string(content))
	}

	nodes := make([]types.Node, len(syms))
	for i, sym := range syms {
		nodes[i] = sym.ToNode(types.LocationNodeID(file.Path, sym.Line, sym.Column))
	}
	return nodes, "", nil
}

// extractEdges extracts call graph edges from a single file.
func extractEdges(file types.FileEntry, pkgNames map[string]string, graph *types.Graph) ([]types.Edge, error) {
	switch file.Language {
	case types.LangGo:
		content, err := os.ReadFile(file.AbsPath)
		if err != nil {
			return nil, err
		}
		pkgName := pkgNames[file.Path]
		return parser.ExtractGoCallGraph(file.Path, content, pkgName, pkgNames, graph)
	default:
		// Try tree-sitter for call graph extraction on non-Go languages.
		if parser.TreeSitterAvailable() {
			return parser.ExtractTreeSitterCallGraph(file.AbsPath, file.Path, file.Language, graph)
		}
	}
	return nil, nil
}

// sanitizeFilename replaces path separators and other problematic characters.
func sanitizeFilename(name string) string {
	replacer := []struct {
		old, new string
	}{
		{"/", "_"}, {"\\", "_"}, {":", "_"},
		{"*", "_"}, {"?", "_"}, {"\"", "_"},
		{"<", "_"}, {">", "_"}, {"|", "_"},
	}
	for _, r := range replacer {
		name = replaceAll(name, r.old, r.new)
	}
	return name
}

func replaceAll(s, old, new string) string {
	for {
		i := 0
		found := false
		for i <= len(s)-len(old) {
			if s[i:i+len(old)] == old {
				s = s[:i] + new + s[i+len(old):]
				found = true
				break
			}
			i++
		}
		if !found {
			break
		}
	}
	return s
}
