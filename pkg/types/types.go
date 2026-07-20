// Package types defines the core data types for codefuse.
//
// Design principle: the index stores only name→position mappings (a "thin index").
// Symbol details (kind, signature, parent, docstring) are extracted on-demand from
// actual source files — the code is the single source of truth.
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

// Node is a lightweight symbol position. It stores only the name and location —
// no kind, signature, parent, or docstring. Those are read from actual source
// files on demand, ensuring the code remains the single source of truth.
type Node struct {
	ID     string `json:"id"`   // Globally unique: "file:line:col"
	Name   string `json:"name"` // Short symbol name
	File   string `json:"file"` // Relative file path
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

// Edge represents a relationship between two nodes (call graph edge).
type Edge struct {
	From string `json:"from"` // Source node ID (caller)
	To   string `json:"to"`   // Target node ID (callee)
	Kind string `json:"kind"` // "calls"
	File string `json:"file"` // File where the relationship occurs
	Line int    `json:"line"` // Line number of the call site
}

// EdgeKind constants.
const (
	EdgeKindCalls = "calls"
)

// Graph is the complete code index with thin nodes and edges.
type Graph struct {
	Version     string      `json:"version"`
	ProjectPath string      `json:"project_path"`
	Files       []FileEntry `json:"files"`
	Nodes       []Node      `json:"nodes"`
	Edges       []Edge      `json:"edges"`

	// Runtime indexes (not serialized)
	nodeByID    map[string]*Node   `json:"-"`
	nodesByName map[string][]*Node `json:"-"`
	edgesFrom   map[string][]Edge  `json:"-"`
	edgesTo     map[string][]Edge  `json:"-"`
}

// BuildIndexes rebuilds the runtime lookup indexes from Nodes and Edges.
func (g *Graph) BuildIndexes() {
	g.nodeByID = make(map[string]*Node, len(g.Nodes))
	g.nodesByName = make(map[string][]*Node, len(g.Nodes))
	g.edgesFrom = make(map[string][]Edge, len(g.Edges))
	g.edgesTo = make(map[string][]Edge, len(g.Edges))

	for i := range g.Nodes {
		node := &g.Nodes[i]
		g.nodeByID[node.ID] = node
		g.nodesByName[node.Name] = append(g.nodesByName[node.Name], node)
	}

	for _, edge := range g.Edges {
		g.edgesFrom[edge.From] = append(g.edgesFrom[edge.From], edge)
		g.edgesTo[edge.To] = append(g.edgesTo[edge.To], edge)
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

// LocationNodeID generates a node ID based on file + line + column.
func LocationNodeID(file string, line, col int) string {
	return file + ":" + itoa(line) + ":" + itoa(col)
}

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
