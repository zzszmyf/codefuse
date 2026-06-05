package types

import "strings"

// =============================================================================
// Graph Data Model (v0.2+)
// Replaces the flat Symbol[] with Node+Edge for cross-file relationship analysis.
// =============================================================================

// IndexVersion is the current index format version.
const IndexVersion = "2"

// EdgeKind constants
const (
	EdgeKindCalls     = "calls"
	EdgeKindContains  = "contains"
	EdgeKindImports   = "imports"
	EdgeKindImplements = "implements"
	EdgeKindReferences = "references"
)

// Node represents a code symbol with a globally unique ID.
type Node struct {
	ID        string `json:"id"`        // Globally unique identifier
	Name      string `json:"name"`      // Short name (e.g. "Hello")
	Kind      string `json:"kind"`      // function | method | struct | interface | ...
	File      string `json:"file"`      // Relative path
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	EndLine   int    `json:"end_line"`
	Parent    string `json:"parent,omitempty"`    // Parent node ID or name
	Signature string `json:"signature,omitempty"`
	Docstring string `json:"docstring,omitempty"`
}

// Edge represents a relationship between two nodes.
type Edge struct {
	From string `json:"from"`   // Source node ID
	To   string `json:"to"`     // Target node ID
	Kind string `json:"kind"`   // calls | contains | imports | implements
	File string `json:"file"`   // File where the relationship occurs
	Line int    `json:"line"`   // Line number
}

// Graph is the complete code index with nodes and edges.
type Graph struct {
	Version     string      `json:"version"`
	ProjectPath string      `json:"project_path"`
	Files       []FileEntry `json:"files"`
	Nodes       []Node      `json:"nodes"`
	Edges       []Edge      `json:"edges"`

	// Runtime indexes (not serialized)
	nodeByID    map[string]*Node `json:"-"`
	edgesFrom   map[string][]Edge `json:"-"`
	edgesTo     map[string][]Edge `json:"-"`
	nodesByName map[string][]*Node `json:"-"`
}

// BuildIndexes builds the runtime lookup indexes.
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

// FindNodeByName returns nodes matching the given name, optionally filtered by kind.
func (g *Graph) FindNodeByName(name, kind string) []Node {
	if g.nodesByName == nil {
		g.BuildIndexes()
	}
	var results []Node
	for _, node := range g.nodesByName[name] {
		if kind == "" || node.Kind == kind {
			results = append(results, *node)
		}
	}
	return results
}

// FindCallers returns all edges where the given node is the callee (who calls me).
func (g *Graph) FindCallers(nodeID string) []Edge {
	if g.edgesTo == nil {
		g.BuildIndexes()
	}
	return g.edgesTo[nodeID]
}

// FindCallees returns all edges where the given node is the caller (who do I call).
func (g *Graph) FindCallees(nodeID string) []Edge {
	if g.edgesFrom == nil {
		g.BuildIndexes()
	}
	return g.edgesFrom[nodeID]
}

// =============================================================================
// Node ID helpers
// =============================================================================

// GoNodeID generates a globally unique node ID for Go symbols.
// Format: package.Receiver.Name (for methods) or package.Name (for functions).
func GoNodeID(pkg, receiver, name string) string {
	receiver = strings.TrimPrefix(receiver, "*")
	if receiver != "" {
		return pkg + "." + receiver + "." + name
	}
	return pkg + "." + name
}

// LocationNodeID generates a node ID based on file location.
// Used for non-Go languages where package-qualified IDs are not available.
func LocationNodeID(file string, line, col int) string {
	return file + ":" + itoa(line) + ":" + itoa(col)
}

// itoa is a small int-to-string helper to avoid importing strconv.
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

// =============================================================================
// Conversion helpers
// =============================================================================

// ToNode converts a Symbol to a Node with the given ID.
func (s Symbol) ToNode(id string) Node {
	return Node{
		ID:        id,
		Name:      s.Name,
		Kind:      s.Kind,
		File:      s.File,
		Line:      s.Line,
		Column:    s.Column,
		EndLine:   s.EndLine,
		Parent:    s.Parent,
		Signature: s.Signature,
		Docstring: s.Docstring,
	}
}

// ToNodes converts a slice of Symbols to Nodes.
// nodeIDFn is called for each symbol to generate its unique ID.
func ToNodes(symbols []Symbol, nodeIDFn func(Symbol) string) []Node {
	nodes := make([]Node, len(symbols))
	for i, sym := range symbols {
		nodes[i] = sym.ToNode(nodeIDFn(sym))
	}
	return nodes
}
