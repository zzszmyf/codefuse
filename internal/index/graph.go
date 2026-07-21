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
	gob.Register(types.Graph{})
	gob.Register(types.Node{})
	gob.Register(types.Edge{})
	gob.Register(types.FileEntry{})
	gob.Register(types.Sink{})
	gob.Register(types.Annotation{})
	gob.Register(types.FileImport{})
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
//  1. Extract nodes + edges + sinks from all files via tree-sitter (batch mode).
//  2. Build runtime indexes.
//  3. Resolve edge targets (callee names → node IDs).
//  4. Collect sinks (unresolved external calls, auto-tagged by package name).
//  5. Save manifest for incremental indexing.
func BuildGraph(projectPath string, files []types.FileEntry) (*Graph, error) {
	graph := NewGraph(projectPath)
	graph.Files = files

	// Phase 1: Extract nodes, edges, and sinks via tree-sitter batch.
	nodesByFile, edgesByFile, sinksByFile, failed := parser.ExtractBatch(files)

	// For failed files, try individual extraction.
	for _, f := range failed {
		nodes, edges, sinks, _ := parser.ExtractFile(f.AbsPath, f.Path, f.Language)
		if len(nodes) > 0 {
			nodesByFile[f.Path] = nodes
		}
		if len(edges) > 0 {
			edgesByFile[f.Path] = edges
		}
		if len(sinks) > 0 {
			sinksByFile[f.Path] = sinks
		}
	}

	// Collect all nodes.
	for _, nodes := range nodesByFile {
		graph.Nodes = append(graph.Nodes, nodes...)
	}

	// Build indexes for edge resolution.
	graph.BuildIndexes()
	graph.buildTrie()

	// Build module map from all nodes (project-level: dotted name → file).
	graph.ModMap = BuildModuleMap(graph.Nodes, projectPath)
	graph.Imports = make(map[string][]types.FileImport)

	// Parse imports for each file (for cross-file edge resolution).
	for _, f := range files {
		content, err := os.ReadFile(f.AbsPath)
		if err != nil {
			continue
		}
		imports, modMap := parser.ParseImports(string(content), f.Path, f.Language)
		if len(imports) > 0 {
			graph.Imports[f.Path] = imports
		}
		// Merge file-level modMap into project-level.
		for k, v := range modMap {
			if _, ok := graph.ModMap[k]; !ok {
				graph.ModMap[k] = v
			}
		}
	}

	// Phase 2: Resolve edge targets (now with imports) and collect sinks.
	for filePath, edges := range edgesByFile {
		imports := graph.Imports[filePath]
		for _, edge := range edges {
			resolved := resolveEdgeWithImports(edge, imports, graph.ModMap, &graph.Graph)
			graph.Edges = append(graph.Edges, resolved...)
		}
	}
	for _, sinks := range sinksByFile {
		graph.Sinks = append(graph.Sinks, sinks...)
	}

	// Rebuild indexes with edges and sinks included.
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

	// Edges — loaded on demand for --callers/--callees.
	if len(g.Edges) > 0 {
		saveGobFile(dir, "edges.gob", g.Edges)
	}

	// Sinks — external call analysis.
	if len(g.Sinks) > 0 {
		saveGobFile(dir, "sinks.gob", g.Sinks)
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

// LoadGraph loads the full index including edges and sinks — for call-graph queries.
func LoadGraph(dir string) (*Graph, error) {
	nodesPath := filepath.Join(dir, "nodes.gob")
	if nodesData, err := os.ReadFile(nodesPath); err == nil {
		graph, err := loadNodesGob(nodesData)
		if err != nil {
			return nil, err
		}
		// Attach edges if available.
		if edgesData, eErr := os.ReadFile(filepath.Join(dir, "edges.gob")); eErr == nil {
			var edges []types.Edge
			if err := gob.NewDecoder(bytes.NewReader(edgesData)).Decode(&edges); err == nil {
				graph.Edges = edges
			}
		}
		// Attach sinks if available.
		if sinksData, sErr := os.ReadFile(filepath.Join(dir, "sinks.gob")); sErr == nil {
			var sinks []types.Sink
			if err := gob.NewDecoder(bytes.NewReader(sinksData)).Decode(&sinks); err == nil {
				graph.Sinks = sinks
			}
		}
		graph.BuildIndexes()
		return graph, nil
	}

	// Fall backs.
	gobPath := filepath.Join(dir, "graph.gob")
	if data, err := os.ReadFile(gobPath); err == nil {
		return loadFullGob(data)
	}
	data, err := os.ReadFile(filepath.Join(dir, "graph.json"))
	if err != nil {
		return nil, err
	}
	return loadJSON(data)
}

func saveGobFile(dir, name string, data interface{}) error {
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return err
	}
	defer f.Close()
	return gob.NewEncoder(f).Encode(data)
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
// Query is the smart entry point for symbol lookup.
// ignoreCase affects only prefix (trie) and exact match behavior.
// Substring and camelCase matches are always case-insensitive.
func (g *Graph) Query(name string, ignoreCase bool) []types.Node {
	if name == "" {
		return nil
	}

	// 1. Prefix pattern: "foo*" (only trailing wildcard).
	if strings.HasSuffix(name, "*") && !strings.ContainsAny(name[:len(name)-1], "*?[") {
		prefix := name[:len(name)-1]
		if ignoreCase {
			// Trie is case-sensitive; for -i, try both cases.
			results := g.FindNodeByPrefix(prefix)
			if len(results) == 0 {
				results = g.FindNodeByPrefix(strings.ToUpper(prefix[:1]) + prefix[1:])
			}
			return results
		}
		return g.FindNodeByPrefix(prefix)
	}

	// 2. Glob pattern: *, ?, [...]
	if strings.ContainsAny(name, "*?[") {
		return g.FindNodeGlob(name)
	}

	// 3. Exact match.
	if ignoreCase {
		if results := g.findExact(name); len(results) > 0 {
			return results
		}
	} else {
		if results := g.findCaseSensitive(name); len(results) > 0 {
			return results
		}
	}

	// 4. CamelCase match: "PA" → "PageAttention", "SC" → "ServiceConfig".
	if results := g.findCamelCase(name); len(results) > 0 {
		return results
	}

	// 5. Substring match — for real-world queries like "PageAttention"
	//    when the actual symbol is "PagedAttention".
	if results := g.findSubstring(name); len(results) > 0 {
		return results
	}

	return nil
}

// findCaseSensitive returns nodes whose name matches (case-sensitive).
func (g *Graph) findCaseSensitive(name string) []types.Node {
	var results []types.Node
	for _, node := range g.Nodes {
		if node.Name == name {
			results = append(results, node)
		}
	}
	return results
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

// findCamelCase matches CamelCase abbreviations: "PA" → "PageAttention", "SC" → "ServiceConfig".
// Only matches queries that are 2+ uppercase letters and contain no lowercase.
func (g *Graph) findCamelCase(name string) []types.Node {
	if len(name) < 2 || strings.ToUpper(name) != name {
		return nil // only pure uppercase abbreviations trigger camelCase match
	}
	var results []types.Node
	for _, node := range g.Nodes {
		if camelMatch(node.Name, name) {
			results = append(results, node)
		}
	}
	return results
}

// camelMatch checks if an uppercase abbreviation matches a CamelCase symbol name.
// e.g., "PA" matches "PageAttention", "SC" matches "ServiceConfig".
func camelMatch(symbolName, abbrev string) bool {
	if len(abbrev) < 2 {
		return false
	}
	si := 0 // position in symbolName
	ai := 0 // position in abbrev
	for si < len(symbolName) && ai < len(abbrev) {
		c := symbolName[si]
		if c >= 'A' && c <= 'Z' {
			if c == abbrev[ai] {
				ai++
			}
			// If an uppercase letter doesn't match our abbrev,
			// and we've already started matching, it breaks the sequence.
		}
		si++
	}
	return ai == len(abbrev) // all abbrev letters matched in order
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

// =============================================================================
// Annotation persistence (agent-writable metadata layer)
// =============================================================================

// LoadAnnotations reads annotations from disk.
func LoadAnnotations(dir string) ([]types.Annotation, error) {
	path := filepath.Join(dir, "annotations.gob")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var anns []types.Annotation
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&anns); err != nil {
		return nil, err
	}
	return anns, nil
}

// SaveAnnotations writes annotations to disk.
func SaveAnnotations(dir string, anns []types.Annotation) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(dir, "annotations.gob"))
	if err != nil {
		return err
	}
	if err := gob.NewEncoder(f).Encode(anns); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// =============================================================================
// Sink and Reachable query helpers
// =============================================================================

// SinkGroup summarizes sinks by package name.
type SinkGroup struct {
	Pkg   string
	Count int
	Sinks []types.Sink
}

// GroupSinksByPkg groups sinks by package name.
func (g *Graph) GroupSinksByPkg() []SinkGroup {
	byPkg := make(map[string][]types.Sink)
	for _, s := range g.Sinks {
		byPkg[s.Pkg] = append(byPkg[s.Pkg], s)
	}
	var groups []SinkGroup
	for pkg, sinks := range byPkg {
		groups = append(groups, SinkGroup{Pkg: pkg, Count: len(sinks), Sinks: sinks})
	}
	return groups
}

// SinksForNodeID returns all sinks originating from a given node ID.
func (g *Graph) SinksForNodeID(nodeID string) []types.Sink {
	return g.Graph.SinksForNode(nodeID)
}

// ReachableFrom finds all paths from fromID to sinks matching pkgPattern.
func (g *Graph) ReachableFrom(fromID, pkgPattern string, maxDepth int) [][]string {
	return g.Graph.Reachable(fromID, pkgPattern, maxDepth)
}

// =============================================================================
// Import-based cross-file edge resolution
// =============================================================================

// BuildModuleMap builds a dotted-name → file-path map from all indexed nodes.
// Python: "db.user_dao" → "db/user_dao.py"
// Java: "com.foo.UserDao" → "com/foo/UserDao.java"
func BuildModuleMap(nodes []types.Node, projectPath string) types.ModuleMap {
	mm := make(types.ModuleMap)
	for _, node := range nodes {
		file := node.File
		// Python: convert db/user_dao.py → db.user_dao
		if strings.HasSuffix(file, ".py") {
			mod := strings.TrimSuffix(file, ".py")
			mod = strings.ReplaceAll(mod, "/", ".")
			if strings.HasSuffix(mod, ".__init__") {
				mod = strings.TrimSuffix(mod, ".__init__")
			}
			mm[mod] = file
		}
		// Java: convert com/foo/UserDao.java → com.foo.UserDao
		if strings.HasSuffix(file, ".java") {
			dotted := strings.TrimSuffix(file, ".java")
			dotted = strings.ReplaceAll(dotted, "/", ".")
			mm[dotted] = file
		}
		// Go/Rust/JS/TS: directory-based modules
		dir := filepath.Dir(file)
		if dir != "." {
			pkg := filepath.Base(dir)
			if _, ok := mm[pkg]; !ok {
				mm[pkg] = dir + "/"
			}
		}
	}
	return mm
}

// resolveEdgeWithImports resolves a raw callee name to node IDs using same-file
// matching first, then import-based cross-file matching.
func resolveEdgeWithImports(edge types.Edge, fileImports []types.FileImport, modMap types.ModuleMap, g *types.Graph) []types.Edge {
	calleeName := edge.To
	callerFile := edge.File

	// Parse dotted name: "userDao.findById" → obj="userDao", method="findById"
	objName, methodName := splitDotted(calleeName)

	// Strategy 1: Same-file match.
	if result := matchSameFile(g, calleeName, callerFile); len(result) > 0 {
		return stampCaller(result, edge.From)
	}

	// Strategy 2: Dotted call → resolve object via imports → find method in target file.
	if objName != "" && methodName != "" {
		targetFile := resolveImport(objName, fileImports, modMap)
		if targetFile != "" {
			if result := matchInFile(g, methodName, targetFile); len(result) > 0 {
				return stampCaller(result, edge.From)
			}
		}
	}

	// Strategy 3: Simple name → try all explicit imports.
	if objName == "" && methodName != "" {
		for _, imp := range fileImports {
			if result := matchInFile(g, methodName, imp.FullPath); len(result) > 0 {
				return stampCaller(result, edge.From)
			}
		}
	}

	return nil
}

// stampCaller sets the From field on edges to the given caller ID.
func stampCaller(edges []types.Edge, callerID string) []types.Edge {
	for i := range edges {
		edges[i].From = callerID
	}
	return edges
}

// splitDotted splits "userDao.findById" into ("userDao", "findById").
func splitDotted(name string) (string, string) {
	idx := strings.IndexRune(name, '.')
	if idx < 0 {
		return "", name
	}
	return name[:idx], name[idx+1:]
}

// resolveImport resolves an object name to a file path using imports.
// Matching is case-insensitive (userDao → UserDao).
func resolveImport(name string, fileImports []types.FileImport, modMap types.ModuleMap) string {
	lower := strings.ToLower(name)
	for _, imp := range fileImports {
		if strings.ToLower(imp.ShortName) == lower || strings.ToLower(imp.Alias) == lower {
			return imp.FullPath
		}
	}
	// Try module map (case-insensitive).
	for k, v := range modMap {
		if strings.ToLower(k) == lower {
			return v
		}
	}
	return ""
}

// matchSameFile finds a callee by name in the caller's file.
func matchSameFile(g *types.Graph, calleeName, callerFile string) []types.Edge {
	candidates := g.FindNodeByName(calleeName)
	if len(candidates) == 0 {
		return nil
	}
	for _, callee := range candidates {
		if callee.File == callerFile {
			return []types.Edge{{
				To:   callee.ID,
				Kind: types.EdgeKindCalls,
				File: callerFile,
			}}
		}
	}
	return nil
}

// matchInFile finds a node by name in a specific file.
func matchInFile(g *types.Graph, name, targetFile string) []types.Edge {
	candidates := g.FindNodeByName(name)
	if len(candidates) == 0 {
		return nil
	}
	for _, callee := range candidates {
		if callee.File == targetFile {
			return []types.Edge{{To: callee.ID, Kind: types.EdgeKindCalls, File: targetFile}}
		}
	}
	return nil
}
