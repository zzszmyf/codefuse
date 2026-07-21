// Package types defines the core data types for codefuse.
//
// Design principles:
//   1. Thin index: stores only name→position mappings. Code is the truth source.
//   2. Sinks: unresolved external calls auto-tagged by package name (no hardcoded categories).
//   3. Annotations: agent-writable metadata layer, persisted alongside the index.
package types

// IndexVersion is the current index format version.
const IndexVersion = "3"

// FileEntry represents a scanned source file.
type FileEntry struct {
	Path     string `json:"path"`
	AbsPath  string `json:"abs_path"`
	Language string `json:"language"`
	Size     int64  `json:"size"`
	Mtime    int64  `json:"mtime"` // nanoseconds since epoch
	IsTest   bool   `json:"is_test"`
}

// Node is a lightweight symbol position.
type Node struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

// Edge represents a relationship between two nodes (call graph edge).
type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"` // "calls"
	File string `json:"file"`
	Line int    `json:"line"`
}

// Sink is an unresolved external call — the callee is NOT in the index.
// Pkg is extracted from the call expression (e.g. "sql.Query" → "sql").
// Sinks enable answering "does method A call the DB?" without hardcoded
// categories — the package name speaks for itself.
type Sink struct {
	From       string `json:"from"`        // caller node ID
	CalleeName string `json:"callee_name"` // full call expression: "sql.Query"
	Pkg        string `json:"pkg"`         // extracted package/module name
	File       string `json:"file"`
	Line       int    `json:"line"`
}

// Annotation is a key-value tag written by an agent (or human) back into the index.
// Agent analyzes sinks, identifies patterns, and annotates symbols with metadata
// like sink_type=db, deprecated=true, entry_point=true.
type Annotation struct {
	ID       string `json:"id"`        // unique id, e.g. uuid
	NodeID   string `json:"node_id"`   // which symbol this annotates
	Key      string `json:"key"`       // "sink_type", "entry_point", "deprecated", etc.
	Value    string `json:"value"`     // "db", "true", etc.
	Source   string `json:"source"`    // "agent", "human", "ci"
	Evidence string `json:"evidence"`  // why: "calls sql.Query via gorm"
}

// EdgeKind constants.
const (
	EdgeKindCalls = "calls"
)

// Graph is the complete code index.
type Graph struct {
	Version     string      `json:"version"`
	ProjectPath string      `json:"project_path"`
	Files       []FileEntry `json:"files"`
	Nodes       []Node      `json:"nodes"`
	Edges       []Edge      `json:"edges"`
	Sinks       []Sink      `json:"sinks"` // unresolved external calls

	// Runtime indexes (not serialized)
	nodeByID    map[string]*Node   `json:"-"`
	nodesByName map[string][]*Node `json:"-"`
	edgesFrom   map[string][]Edge  `json:"-"`
	edgesTo     map[string][]Edge  `json:"-"`
	sinksFrom   map[string][]Sink  `json:"-"` // nodeID → its sinks
}

// BuildIndexes rebuilds the runtime lookup indexes.
func (g *Graph) BuildIndexes() {
	g.nodeByID = make(map[string]*Node, len(g.Nodes))
	g.nodesByName = make(map[string][]*Node, len(g.Nodes))
	g.edgesFrom = make(map[string][]Edge, len(g.Edges))
	g.edgesTo = make(map[string][]Edge, len(g.Edges))
	g.sinksFrom = make(map[string][]Sink, len(g.Sinks))

	for i := range g.Nodes {
		node := &g.Nodes[i]
		g.nodeByID[node.ID] = node
		g.nodesByName[node.Name] = append(g.nodesByName[node.Name], node)
	}

	for _, edge := range g.Edges {
		g.edgesFrom[edge.From] = append(g.edgesFrom[edge.From], edge)
		g.edgesTo[edge.To] = append(g.edgesTo[edge.To], edge)
	}

	for _, sink := range g.Sinks {
		g.sinksFrom[sink.From] = append(g.sinksFrom[sink.From], sink)
	}
}

// FindNodeByID returns a node by its globally unique ID.
func (g *Graph) FindNodeByID(id string) *Node {
	if g.nodeByID == nil {
		g.BuildIndexes()
	}
	return g.nodeByID[id]
}

// FindNodeByName returns nodes matching the given name.
func (g *Graph) FindNodeByName(name string) []Node {
	if g.nodesByName == nil {
		g.BuildIndexes()
	}
	var results []Node
	for _, node := range g.nodesByName[name] {
		results = append(results, *node)
	}
	return results
}

// FindCallers returns all edges where the given node is the callee.
func (g *Graph) FindCallers(nodeID string) []Edge {
	if g.edgesTo == nil {
		g.BuildIndexes()
	}
	return g.edgesTo[nodeID]
}

// FindCallees returns all edges where the given node is the caller.
func (g *Graph) FindCallees(nodeID string) []Edge {
	if g.edgesFrom == nil {
		g.BuildIndexes()
	}
	return g.edgesFrom[nodeID]
}

// SinksForNode returns all external sinks originating from the given node.
func (g *Graph) SinksForNode(nodeID string) []Sink {
	if g.sinksFrom == nil {
		g.BuildIndexes()
	}
	return g.sinksFrom[nodeID]
}

// FilterSinks filters sinks by package name (case-insensitive glob match).
func (g *Graph) FilterSinks(sinks []Sink, pkgPattern string) []Sink {
	var filtered []Sink
	for _, s := range sinks {
		if matchPkg(s.Pkg, pkgPattern) {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

// Reachable finds all paths from fromID to any sink matching pkgPattern.
// Uses BFS with maxDepth limit. Returns paths as sequences of node IDs.
func (g *Graph) Reachable(fromID string, pkgPattern string, maxDepth int) [][]string {
	if g.nodeByID == nil {
		g.BuildIndexes()
	}

	// Check if fromID itself has a matching sink (distance 0).
	var paths [][]string
	if sinks := g.SinksForNode(fromID); len(sinks) > 0 {
		if len(g.FilterSinks(sinks, pkgPattern)) > 0 {
			// For direct sink, path is just [fromID, sink info collected below].
			// We add placeholder and fill actual callee names later.
		}
	}

	// BFS: each entry is (current node ID, path so far).
	type bfsEntry struct {
		nodeID string
		path   []string
	}
	queue := []bfsEntry{{nodeID: fromID, path: []string{fromID}}}
	visited := map[string]bool{fromID: true}

	for len(queue) > 0 {
		entry := queue[0]
		queue = queue[1:]

		if len(entry.path) > maxDepth {
			continue
		}

		// Check this node's sinks.
		if sinks := g.SinksForNode(entry.nodeID); len(sinks) > 0 {
			matched := g.FilterSinks(sinks, pkgPattern)
			if len(matched) > 0 {
				// Found a matching sink at this node.
				// Append matching sink callee names as final step.
				for _, m := range matched {
					fullPath := make([]string, len(entry.path)+1)
					copy(fullPath, entry.path)
					fullPath[len(entry.path)] = m.CalleeName
					paths = append(paths, fullPath)
				}
			}
		}

		// Follow edges to callees.
		for _, edge := range g.FindCallees(entry.nodeID) {
			if !visited[edge.To] {
				visited[edge.To] = true
				newPath := make([]string, len(entry.path)+1)
				copy(newPath, entry.path)
				newPath[len(entry.path)] = edge.To
				queue = append(queue, bfsEntry{nodeID: edge.To, path: newPath})
			}
		}
	}

	return paths
}

// matchPkg performs case-insensitive glob match on a package name.
func matchPkg(pkg, pattern string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}
	lowerPkg := toLower(pkg)
	lowerPat := toLower(pattern)
	if stringsContains(lowerPkg, lowerPat) {
		return true
	}
	// Exact match after stripping dots.
	return lowerPkg == lowerPat
}

// LocationNodeID generates a node ID based on file + line + column.
func LocationNodeID(file string, line, col int) string {
	return file + ":" + itoa(line) + ":" + itoa(col)
}

// ExtractPkg extracts the package name from a dotted callee expression.
// "sql.Query" → "sql", "http.Client.Get" → "http", "gorm.DB.Find" → "gorm"
func ExtractPkg(calleeName string) string {
	if idx := stringsIndexByte(calleeName, '.'); idx > 0 {
		return calleeName[:idx]
	}
	return calleeName
}

// =============================================================================
// Internal helpers (avoid importing strings for few functions)
// =============================================================================

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf) - 1
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		buf[i] = byte('0' + n%10)
		n /= 10
		i--
	}
	if neg {
		buf[i] = '-'
		i--
	}
	return string(buf[i+1:])
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		b[i] = c
	}
	return string(b)
}

func stringsContains(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func stringsIndexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}
