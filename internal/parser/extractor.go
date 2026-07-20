// Package parser provides unified, config-driven symbol extraction via tree-sitter.
//
// Design: there is exactly ONE extraction engine. Language differences are
// expressed as configuration (LangConfig), not code. Adding a new language
// requires zero Go changes — just add an entry to pkg/config/config.go.
//
// The extractor outputs thin symbols (name + position only). Kind, signature,
// parent, and docstring are NOT stored in the index — they are extracted
// on-demand from actual source files.
package parser

import (
	"bytes"
	"encoding/xml"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/yifanmeng/codefuse/pkg/config"
	"github.com/yifanmeng/codefuse/pkg/types"
)

// TreeSitterAvailable reports whether the tree-sitter CLI is installed.
func TreeSitterAvailable() bool {
	_, err := exec.LookPath("tree-sitter")
	return err == nil
}

// =============================================================================
// Unified extraction entry points
// =============================================================================

// ExtractFile parses a single source file and extracts thin symbol nodes.
// Returns (nodes, edges, error). On tree-sitter failure, returns empty slices.
func ExtractFile(absPath, relPath, language string) ([]types.Node, []types.Edge, error) {
	cfg := config.Builtin[language]
	if !TreeSitterAvailable() {
		return nil, nil, nil
	}

	cmd := exec.Command("tree-sitter", "parse", "--xml", absPath)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, nil, nil // tree-sitter fails → return empty, caller falls back
	}

	return parseXML(out.Bytes(), relPath, &cfg)
}

// ExtractBatch parses multiple files in one tree-sitter invocation.
// Files are grouped by language; tree-sitter is invoked per chunk (up to 500 files).
// Returns (filePath → nodes, filePath → edges, failed files).
func ExtractBatch(files []types.FileEntry) (map[string][]types.Node, map[string][]types.Edge, []types.FileEntry) {
	if !TreeSitterAvailable() {
		return nil, nil, files
	}

	nodesResult := make(map[string][]types.Node)
	edgesResult := make(map[string][]types.Edge)
	var failed []types.FileEntry

	// Group files by language.
	byLang := make(map[string][]types.FileEntry)
	for _, f := range files {
		byLang[f.Language] = append(byLang[f.Language], f)
	}

	for lang, group := range byLang {
		cfg, ok := config.Builtin[lang]
		if !ok {
			failed = append(failed, group...)
			continue
		}

		const chunkSize = 500
		for i := 0; i < len(group); i += chunkSize {
			end := i + chunkSize
			if end > len(group) {
				end = len(group)
			}
			chunk := group[i:end]
			nodesMap, edgesMap, chunkFailed := parseChunk(chunk, &cfg)
			for path, nodes := range nodesMap {
				nodesResult[path] = nodes
			}
			for path, e := range edgesMap {
				edgesResult[path] = e
			}
			failed = append(failed, chunkFailed...)
		}
	}

	return nodesResult, edgesResult, failed
}

// parseChunk runs tree-sitter on a batch of files (via --paths flag).
func parseChunk(files []types.FileEntry, cfg *config.LangConfig) (map[string][]types.Node, map[string][]types.Edge, []types.FileEntry) {
	tmpDir, err := os.MkdirTemp("", "codefuse-ts-*")
	if err != nil {
		return nil, nil, files
	}
	defer os.RemoveAll(tmpDir)

	pathsFile := filepath.Join(tmpDir, "paths.txt")
	var sb strings.Builder
	for _, f := range files {
		sb.WriteString(f.AbsPath)
		sb.WriteByte('\n')
	}
	if err := os.WriteFile(pathsFile, []byte(sb.String()), 0644); err != nil {
		return nil, nil, files
	}

	cmd := exec.Command("tree-sitter", "parse", "--paths", pathsFile, "--xml")
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, nil, files // all files in chunk failed
	}

	return parseBatchXML(out.Bytes(), files, cfg)
}

// =============================================================================
// XML AST types (internal to parser)
// =============================================================================

// tsSource captures a single <source> element from tree-sitter XML output.
// The root AST node varies by language: Python→<module>, Java→<program>,
// Go/Rust→<source_file>, C/C++→<translation_unit>. Using xml:",any" captures
// the root node regardless of its tag name.
type tsSource struct {
	XMLName xml.Name `xml:"source"`
	Name    string   `xml:"name,attr"`
	Nodes   []tsNode `xml:",any"` // root node(s) — tag varies by language
}

type tsNode struct {
	XMLName  xml.Name `xml:""`
	SRow     int      `xml:"srow,attr"`
	SCol     int      `xml:"scol,attr"`
	ERow     int      `xml:"erow,attr"`
	ECol     int      `xml:"ecol,attr"`
	Name     string   `xml:"name,attr"`
	Value    string   `xml:"value,attr"`
	Chardata string   `xml:",chardata"`
	Nodes    []tsNode `xml:",any"`
}

// =============================================================================
// XML parsing
// =============================================================================

func parseXML(data []byte, relPath string, cfg *config.LangConfig) ([]types.Node, []types.Edge, error) {
	var doc struct {
		Sources []tsSource `xml:"source"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, nil, err
	}
	if len(doc.Sources) == 0 {
		return nil, nil, nil
	}

	var nodes []types.Node
	var edges []types.Edge
	for _, node := range doc.Sources[0].Nodes {
		extractFromTree(&nodes, &edges, node, relPath, "", cfg)
	}
	return nodes, edges, nil
}

func parseBatchXML(data []byte, files []types.FileEntry, cfg *config.LangConfig) (map[string][]types.Node, map[string][]types.Edge, []types.FileEntry) {
	var doc struct {
		Sources []tsSource `xml:"source"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, nil, files
	}

	absToRel := make(map[string]types.FileEntry, len(files))
	for _, f := range files {
		absToRel[f.AbsPath] = f
	}

	nodesResult := make(map[string][]types.Node)
	edgesResult := make(map[string][]types.Edge)

	for _, src := range doc.Sources {
		f, ok := absToRel[src.Name]
		if !ok {
			continue
		}
		var nodes []types.Node
		var edges []types.Edge
		for _, node := range src.Nodes {
			extractFromTree(&nodes, &edges, node, f.Path, "", cfg)
		}
		nodesResult[f.Path] = nodes
		edgesResult[f.Path] = edges
	}

	return nodesResult, edgesResult, nil
}

// =============================================================================
// Config-driven tree walker — the heart of the extractor
// =============================================================================

// extractFromTree recursively walks tree-sitter AST nodes and extracts:
//   - Thin symbols (name + position) from declaration nodes
//   - Call edges from call-site nodes inside function bodies
//
// langConfig tells us which node types are declarations and which are call sites.
// Everything else is language-agnostic tree walking.
func extractFromTree(
	nodes *[]types.Node,
	edges *[]types.Edge,
	node tsNode,
	relPath string,
	enclosingFunc string,
	cfg *config.LangConfig,
) {
	// Track enclosing function for edge attribution.
	if newFunc := findEnclosingFunc(node, cfg); newFunc != "" {
		enclosingFunc = newFunc
	}

	nodeType := node.XMLName.Local

	// 1. Declaration node → extract symbol name + position.
	if isDeclNode(nodeType, cfg) {
		if name := extractName(node, cfg.NameTags); name != "" {
			id := types.LocationNodeID(relPath, node.SRow+1, node.SCol+1)
			*nodes = append(*nodes, types.Node{
				ID:     id,
				Name:   name,
				File:   relPath,
				Line:   node.SRow + 1, // tree-sitter uses 0-based rows
				Column: node.SCol + 1,
			})
		}
	}

	// 2. Call-site node inside a function body → extract edge.
	if isCallNode(nodeType, cfg) && enclosingFunc != "" {
		if calleeName := extractCallee(node, cfg.CalleeTags); calleeName != "" {
			callerID := findNodeInList(*nodes, enclosingFunc, relPath)
			// Edge uses callee name (not ID) — resolved at query time against the graph.
			// We store callee name in the To field; the graph builder resolves it.
			if callerID != "" {
				*edges = append(*edges, types.Edge{
					From: callerID,
					To:   calleeName, // will be resolved by graph builder
					Kind: types.EdgeKindCalls,
					File: relPath,
					Line: node.SRow + 1,
				})
			}
		}
	}

	// Recurse into children.
	for _, child := range node.Nodes {
		extractFromTree(nodes, edges, child, relPath, enclosingFunc, cfg)
	}
}

// =============================================================================
// Node type matching
// =============================================================================

func isDeclNode(nodeType string, cfg *config.LangConfig) bool {
	for _, dt := range cfg.DeclNodes {
		if nodeType == dt {
			return true
		}
	}
	return false
}

func isCallNode(nodeType string, cfg *config.LangConfig) bool {
	for _, ct := range cfg.CallNodes {
		if nodeType == ct {
			return true
		}
	}
	return false
}

// findEnclosingFunc returns the function name if this node is a function-like
// declaration (any DeclNode that looks like a callable: function/method/constructor).
func findEnclosingFunc(node tsNode, cfg *config.LangConfig) string {
	callableTypes := map[string]bool{
		"function_declaration":    true,
		"function_definition":     true,
		"function_item":           true,
		"method_declaration":      true,
		"method_definition":       true,
		"constructor_declaration": true,
		"arrow_function":          true,
	}

	nodeType := node.XMLName.Local
	if callableTypes[nodeType] || isDeclNode(nodeType, cfg) {
		return extractName(node, cfg.NameTags)
	}
	return ""
}

// =============================================================================
// Name / identifier extraction from AST nodes
// =============================================================================

// extractName extracts the symbol name from a declaration node by searching
// child nodes for tags listed in nameTags.
func extractName(node tsNode, nameTags []string) string {
	if nameTags == nil {
		nameTags = config.DefaultNameTags
	}

	// Direct attribute: tree-sitter puts the name in the "name" attribute
	// for certain node types (e.g., <identifier name="foo"/>).
	if node.Name != "" {
		for _, tag := range node.XMLName.Local {
			_ = tag
		}
		// Check if this node itself is an identifier-like tag.
		if isNameTag(node.XMLName.Local, nameTags) && node.Name != "" {
			return node.Name
		}
	}

	// Search children for name-carrying nodes.
	for _, child := range node.Nodes {
		if isNameTag(child.XMLName.Local, nameTags) {
			if child.Name != "" {
				return child.Name
			}
			if text := strings.TrimSpace(child.Chardata); text != "" {
				return text
			}
		}
		// Recurse one level for nested identifiers (e.g., type_spec → type_identifier).
		if result := extractName(child, nameTags); result != "" {
			return result
		}
	}
	return ""
}

// extractCallee extracts the callee name from a call-site node.
func extractCallee(node tsNode, calleeTags []string) string {
	if calleeTags == nil {
		calleeTags = config.DefaultCalleeTags
	}

	// Direct: <identifier name="foo"/>
	for _, child := range node.Nodes {
		if isNameTag(child.XMLName.Local, calleeTags) {
			if child.Name != "" {
				return child.Name
			}
			if text := strings.TrimSpace(child.Chardata); text != "" {
				return text
			}
		}
		// member_expression: obj.method() → return method name
		if child.XMLName.Local == "member_expression" || child.XMLName.Local == "field_expression" {
			if name := extractName(child, calleeTags); name != "" {
				return name
			}
		}
		// scoped_identifier: std::foo::bar() → return last segment
		if child.XMLName.Local == "scoped_identifier" {
			if name := extractName(child, calleeTags); name != "" {
				return name
			}
		}
	}

	return ""
}

func isNameTag(tag string, tags []string) bool {
	for _, t := range tags {
		if tag == t {
			return true
		}
	}
	return false
}

// =============================================================================
// Helpers
// =============================================================================

// findNodeInList finds a node ID by name and file in an already-extracted node list.
func findNodeInList(nodes []types.Node, name, file string) string {
	for _, n := range nodes {
		if n.Name == name && n.File == file {
			return n.ID
		}
	}
	return ""
}

// BuiltinConfig returns the builtin language configuration.
func BuiltinConfig() map[string]config.LangConfig {
	return config.Builtin
}

// BuildExtToLang builds an extension→language map from the builtin config.
func BuildExtToLang() map[string]string {
	return config.ExtToLang
}

// SanitizeFilename replaces path separators and special characters.
func SanitizeFilename(name string) string {
	for _, r := range []struct{ old, new string }{
		{"/", "_"}, {"\\", "_"}, {":", "_"},
		{"*", "_"}, {"?", "_"}, {"\"", "_"},
		{"<", "_"}, {">", "_"}, {"|", "_"},
	} {
		name = strings.ReplaceAll(name, r.old, r.new)
	}
	return name
}

// init ensures the XML parser is properly initialized.
func init() {
	// Verify xml.Unmarshal is available (compile-time check).
	var _ = xml.Unmarshal
}
