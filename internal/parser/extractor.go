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
	"bufio"
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yifanmeng/codefuse/pkg/config"
	"github.com/yifanmeng/codefuse/pkg/types"
)

// Import patterns per language.
var (
	pyFromPat   = regexp.MustCompile(`^from\s+([\w.]+)\s+import\s+(.+)`)
	pyImportPat = regexp.MustCompile(`^import\s+([\w.]+)(?:\s+as\s+(\w+))?`)
	javaImportPat = regexp.MustCompile(`^import\s+(static\s+)?([\w.]+)(?:\.(\*|\w+))?\s*;`)
	goImportPat   = regexp.MustCompile(`"([^"]+)"`)
)

// TreeSitterAvailable reports whether the tree-sitter CLI is installed.
func TreeSitterAvailable() bool {
	_, err := exec.LookPath("tree-sitter")
	return err == nil
}

// =============================================================================
// Unified extraction entry points
// =============================================================================

// ParseError describes a tree-sitter parse failure.
type ParseError struct {
	File   string
	Lang   string
	Stderr string
	Err    error
}

func (e *ParseError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("parse %s (%s): %v (stderr: %s)", e.File, e.Lang, e.Err, e.Stderr)
	}
	return fmt.Sprintf("parse %s (%s): %s", e.File, e.Lang, e.Stderr)
}

// ExtractFromXML parses tree-sitter XML output directly.
// Useful for testing with pre-generated XML fixtures (no tree-sitter needed).
func ExtractFromXML(xmlData []byte, relPath, language string) ([]types.Node, []types.Edge, []types.Sink, error) {
	cfg, ok := config.Builtin[language]
	if !ok {
		return nil, nil, nil, fmt.Errorf("unsupported language: %s", language)
	}
	return parseXML(xmlData, relPath, &cfg)
}

// ExtractFile parses a single source file and extracts thin symbol nodes, edges, and sinks.
func ExtractFile(absPath, relPath, language string) ([]types.Node, []types.Edge, []types.Sink, error) {
	cfg := config.Builtin[language]
	if !TreeSitterAvailable() {
		return nil, nil, nil, nil
	}

	cmd := exec.Command("tree-sitter", "parse", "--xml", absPath)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, nil, nil, nil
	}

	return parseXML(out.Bytes(), relPath, &cfg)
}

// ExtractBatch parses multiple files in one tree-sitter invocation.
func ExtractBatch(files []types.FileEntry) (map[string][]types.Node, map[string][]types.Edge, map[string][]types.Sink, []types.FileEntry) {
	if !TreeSitterAvailable() {
		return nil, nil, nil, files
	}

	nodesResult := make(map[string][]types.Node)
	edgesResult := make(map[string][]types.Edge)
	sinksResult := make(map[string][]types.Sink)
	var failed []types.FileEntry

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
			nodesMap, edgesMap, sinksMap, chunkFailed := parseChunk(chunk, &cfg)
			for path, nodes := range nodesMap {
				nodesResult[path] = nodes
			}
			for path, e := range edgesMap {
				edgesResult[path] = e
			}
			for path, s := range sinksMap {
				sinksResult[path] = s
			}
			failed = append(failed, chunkFailed...)
		}
	}

	return nodesResult, edgesResult, sinksResult, failed
}

// parseChunk runs tree-sitter on a batch of files (via --paths flag).
func parseChunk(files []types.FileEntry, cfg *config.LangConfig) (map[string][]types.Node, map[string][]types.Edge, map[string][]types.Sink, []types.FileEntry) {
	tmpDir, err := os.MkdirTemp("", "codefuse-ts-*")
	if err != nil {
		return nil, nil, nil, files
	}
	defer os.RemoveAll(tmpDir)

	pathsFile := filepath.Join(tmpDir, "paths.txt")
	var sb strings.Builder
	for _, f := range files {
		sb.WriteString(f.AbsPath)
		sb.WriteByte('\n')
	}
	if err := os.WriteFile(pathsFile, []byte(sb.String()), 0644); err != nil {
		return nil, nil, nil, files
	}

	cmd := exec.Command("tree-sitter", "parse", "--paths", pathsFile, "--xml")
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, nil, nil, files
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

func parseXML(data []byte, relPath string, cfg *config.LangConfig) ([]types.Node, []types.Edge, []types.Sink, error) {
	var doc struct {
		Sources []tsSource `xml:"source"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, nil, nil, err
	}
	if len(doc.Sources) == 0 {
		return nil, nil, nil, nil
	}

	var nodes []types.Node
	var edges []types.Edge
	var sinks []types.Sink
	for _, node := range doc.Sources[0].Nodes {
		extractFromTree(&nodes, &edges, &sinks, node, relPath, "", cfg)
	}
	return nodes, edges, sinks, nil
}

func parseBatchXML(data []byte, files []types.FileEntry, cfg *config.LangConfig) (map[string][]types.Node, map[string][]types.Edge, map[string][]types.Sink, []types.FileEntry) {
	var doc struct {
		Sources []tsSource `xml:"source"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, nil, nil, files
	}

	absToRel := make(map[string]types.FileEntry, len(files))
	for _, f := range files {
		absToRel[f.AbsPath] = f
	}

	nodesResult := make(map[string][]types.Node)
	edgesResult := make(map[string][]types.Edge)
	sinksResult := make(map[string][]types.Sink)

	for _, src := range doc.Sources {
		f, ok := absToRel[src.Name]
		if !ok {
			continue
		}
		var nodes []types.Node
		var edges []types.Edge
		var sinks []types.Sink
		for _, node := range src.Nodes {
			extractFromTree(&nodes, &edges, &sinks, node, f.Path, "", cfg)
		}
		nodesResult[f.Path] = nodes
		edgesResult[f.Path] = edges
		sinksResult[f.Path] = sinks
	}

	return nodesResult, edgesResult, sinksResult, nil
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
	sinks *[]types.Sink,
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

	// 2. Call-site node → extract edge (if inside a function) or sink (always).
	if isCallNode(nodeType, cfg) {
		if calleeName := extractCallee(node, cfg.CalleeTags); calleeName != "" {
			// Dotted calls (pkg.Func, obj.Method) → always recorded as Sinks.
			// Sinks capture external call sites regardless of enclosing function.
			if isExternalCall(calleeName) {
				// Try to find the enclosing function for attribution.
				callerID := ""
				if enclosingFunc != "" {
					callerID = findNodeInList(*nodes, enclosingFunc, relPath)
				}
				*sinks = append(*sinks, types.Sink{
					From:       callerID,
					CalleeName: calleeName,
					Pkg:        types.ExtractPkg(calleeName),
					File:       relPath,
					Line:       node.SRow + 1,
				})
			} else if enclosingFunc != "" {
				// Simple name inside a function → internal edge, resolved later.
				if callerID := findNodeInList(*nodes, enclosingFunc, relPath); callerID != "" {
					*edges = append(*edges, types.Edge{
						From: callerID,
						To:   calleeName,
						Kind: types.EdgeKindCalls,
						File: relPath,
						Line: node.SRow + 1,
					})
				}
			}
		}
	}

	// Recurse into children.
	for _, child := range node.Nodes {
		extractFromTree(nodes, edges, sinks, child, relPath, enclosingFunc, cfg)
	}
}

// isExternalCall returns true if the callee looks like an external call (pkg.Func or obj.Method).
// Simple names like "foo()" without dots are assumed to be internal.
func isExternalCall(callee string) bool {
	return strings.ContainsRune(callee, '.')
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
	if node.Name != "" && isNameTag(node.XMLName.Local, nameTags) {
		return node.Name
	}

	// First pass: prefer plain "identifier" over "type_identifier" (for Java methods).
	for _, preferTag := range []string{"identifier", ""} {
		for _, child := range node.Nodes {
			if preferTag != "" && child.XMLName.Local != preferTag {
				continue
			}
			if isNameTag(child.XMLName.Local, nameTags) {
				if child.Name != "" {
					return child.Name
				}
				if text := strings.TrimSpace(child.Chardata); text != "" {
					return text
				}
			}
			// Recurse one level for nested identifiers.
			if result := extractName(child, nameTags); result != "" {
				return result
			}
		}
	}
	return ""
}

// extractCallee extracts the callee name from a call-site node.
// For dotted calls (obj.method, pkg.Func), returns the full dotted name.
func extractCallee(node tsNode, calleeTags []string) string {
	if calleeTags == nil {
		calleeTags = config.DefaultCalleeTags
	}

	for _, child := range node.Nodes {
		if isNameTag(child.XMLName.Local, calleeTags) {
			if child.Name != "" {
				return child.Name
			}
			if text := strings.TrimSpace(child.Chardata); text != "" {
				return text
			}
		}
		// member_expression (JS/TS): obj.method()
		if child.XMLName.Local == "member_expression" || child.XMLName.Local == "field_expression" {
			// Try to get full dotted name: obj.method
			if full := extractDottedName(child); full != "" {
				return full
			}
			if name := extractName(child, calleeTags); name != "" {
				return name
			}
		}
		// attribute (Python): obj.method, pkg.func
		// Tree-sitter: <attribute><identifier field="object">X</identifier>.<identifier field="attribute">Y</identifier></attribute>
		if child.XMLName.Local == "attribute" || child.XMLName.Local == "field_access" {
			if full := extractDottedName(child); full != "" {
				return full
			}
		}
		// scoped_identifier (Rust): std::foo::bar()
		if child.XMLName.Local == "scoped_identifier" {
			if name := extractName(child, calleeTags); name != "" {
				return name
			}
		}
	}

	return ""
}

// extractDottedName reconstructs a dotted name from a qualified node like:
//   <attribute> → "obj.method"
//   <member_expression> → "obj.method"
func extractDottedName(node tsNode) string {
	var object, attribute string
	for _, child := range node.Nodes {
		switch child.XMLName.Local {
		case "identifier":
			if child.Name != "" {
				if object == "" {
					object = child.Name
				} else {
					attribute = child.Name
				}
			} else if text := strings.TrimSpace(child.Chardata); text != "" {
				if object == "" {
					object = text
				} else {
					attribute = text
				}
			}
		default:
			// Recurse for nested attributes
			if result := extractDottedName(child); result != "" {
				if object == "" {
					object = result
				} else {
					attribute = result
				}
			}
		}
	}
	if object != "" && attribute != "" {
		return object + "." + attribute
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

// =============================================================================
// Import parsing — language-aware, regex-based.
// =============================================================================

// ParseImports extracts import mappings from a source file.
// Returns (file-level imports, updated module map).
// The module map maps dotted module names → file paths.
func ParseImports(content, relPath, language string) ([]types.FileImport, types.ModuleMap) {
	var imports []types.FileImport
	modMap := make(types.ModuleMap)

	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		trimmed := strings.TrimSpace(scanner.Text())
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
			continue
		}

		switch language {
		case "python":
			parsePythonImportLine(trimmed, relPath, &imports, modMap)
		case "java":
			parseJavaImportLine(trimmed, relPath, &imports, modMap)
		case "go":
			parseGoImportLine(trimmed, relPath, &imports, modMap)
		case "rust":
			parseRustImportLine(trimmed, relPath, &imports, modMap)
		}
	}

	return imports, modMap
}

func parsePythonImportLine(line, relPath string, imports *[]types.FileImport, modMap types.ModuleMap) {
	// from X import Y, Z as W
	if m := pyFromPat.FindStringSubmatch(line); m != nil {
		module := m[1]                  // e.g. "db.user_dao"
		targetFile := moduleToPath(module, ".py")
		modMap[module] = targetFile

		items := strings.Split(m[2], ",")
		for _, item := range items {
			item = strings.TrimSpace(item)
			if item == "*" || item == "" {
				continue
			}
			parts := strings.SplitN(item, " as ", 2)
			if len(parts) == 2 {
				*imports = append(*imports, types.FileImport{
					ShortName: strings.TrimSpace(parts[0]), // original: "decrypt"
					FullPath:  targetFile,
					Alias:     strings.TrimSpace(parts[1]), // alias: "dec"
				})
			} else {
				*imports = append(*imports, types.FileImport{
					ShortName: item,
					FullPath:  targetFile,
				})
			}
		}
		return
	}

	// import X or import X as Y
	if m := pyImportPat.FindStringSubmatch(line); m != nil {
		module := m[1]
		modMap[module] = moduleToPath(module, ".py")
		if m[2] != "" {
			*imports = append(*imports, types.FileImport{
				ShortName: m[2],
				FullPath:  moduleToPath(module, ".py"),
				Alias:     module,
			})
		}
	}
}

func parseJavaImportLine(line, relPath string, imports *[]types.FileImport, modMap types.ModuleMap) {
	// import com.foo.UserDao → dotted="com.foo.UserDao", suffix="UserDao"
	// import java.util.*     → dotted="java.util", suffix="*"
	dotted := extractJavaImport(line)
	if dotted == "" {
		return
	}

	// Split by last dot to get package vs class name.
	lastDot := strings.LastIndex(dotted, ".")
	if lastDot < 0 {
		return
	}
	suffix := dotted[lastDot+1:]
	pkg := dotted[:lastDot]

	if suffix == "*" {
		modMap[dotted] = strings.ReplaceAll(pkg, ".", "/") + "/"
	} else {
		targetFile := strings.ReplaceAll(dotted, ".", "/") + ".java"
		modMap[dotted] = targetFile
		*imports = append(*imports, types.FileImport{
			ShortName: suffix,
			FullPath:  targetFile,
		})
	}
	_ = relPath
}

func extractJavaImport(line string) string {
	// Match: import [static] com.foo.UserDao;
	line = strings.TrimPrefix(line, "import ")
	line = strings.TrimPrefix(line, "static ")
	line = strings.TrimSuffix(line, ";")
	return strings.TrimSpace(line)
}

func parseGoImportLine(line, relPath string, imports *[]types.FileImport, modMap types.ModuleMap) {
	matches := goImportPat.FindAllStringSubmatch(line, -1)
	for _, m := range matches {
		pkgPath := m[1]
		parts := strings.Split(pkgPath, "/")
		pkgName := parts[len(parts)-1]
		// External packages (github.com/..., golang.org/...) don't get trailing /.
		if strings.Contains(pkgPath, ".") {
			modMap[pkgName] = pkgPath
		} else {
			modMap[pkgName] = pkgPath + "/"
		}
		// Check for alias.
		if idx := strings.LastIndex(line, pkgPath); idx > 0 {
			prefix := strings.TrimSpace(line[:idx-1])
			if prefix != "" && !strings.Contains(prefix, "\"") {
				alias := strings.Fields(prefix)
				if len(alias) > 0 {
					modMap[alias[len(alias)-1]] = pkgPath
				}
			}
		}
	}
	_ = imports
	_ = relPath
}

func parseRustImportLine(line, relPath string, imports *[]types.FileImport, modMap types.ModuleMap) {
	// use crate::db::UserDao;
	// use std::collections::HashMap;
	trimmed := strings.TrimPrefix(line, "use ")
	if trimmed == line {
		return
	}
	// Split by "::" and extract last segment as the imported name.
	parts := strings.Split(trimmed, "::")
	if len(parts) > 0 {
		name := strings.TrimSuffix(strings.TrimSuffix(parts[len(parts)-1], ";"), "}")
		cratePath := strings.Join(parts[:len(parts)-1], "/")
		targetFile := cratePath + ".rs"
		modMap[name] = targetFile
		*imports = append(*imports, types.FileImport{
			ShortName: name,
			FullPath:  targetFile,
		})
	}
	_ = relPath
	_ = modMap
}

// moduleToPath converts a dotted module name to a file path.
func moduleToPath(dotted, ext string) string {
	return strings.ReplaceAll(dotted, ".", "/") + ext
}

// =============================================================================
// VarMap — variable→type inference for cross-file edge resolution.
// Regex-based, no extra tree-sitter invocation.
// =============================================================================

var (
	pyAssignPat   = regexp.MustCompile(`(\w+)\s*=\s*(\w+)\(`)
	pyParamPat    = regexp.MustCompile(`(\w+)\s*:\s*(\w+)`)
	pyGenericPat  = regexp.MustCompile(`(\w+)\s*:\s*(?:List|Optional|Dict)\[(\w+)`)
	javaVarDeclPat = regexp.MustCompile(`(\w+)\s+(\w+)\s*=`)
	javaGenericPat = regexp.MustCompile(`(?:List|Optional|Map)<(\w+)>\s+(\w+)`)
)

// ExtractVarMap extracts variable→type mappings from source code.
func ExtractVarMap(content, language string) map[string]string {
	vm := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(content))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}

		switch language {
		case "python":
			// x = Foo() → {"x": "Foo"}
			if m := pyAssignPat.FindStringSubmatch(line); m != nil {
				vm[m[1]] = m[2]
				continue
			}
			// def f(x: Foo): → {"x": "Foo"}
			if m := pyParamPat.FindStringSubmatch(line); m != nil {
				vm[m[1]] = m[2]
				continue
			}
			// x: List[Foo] = → {"x": "Foo"}
			if m := pyGenericPat.FindStringSubmatch(line); m != nil {
				vm[m[1]] = m[2]
			}
		case "java":
			// Foo x = new Foo() → {"x": "Foo"}
			if m := javaVarDeclPat.FindStringSubmatch(line); m != nil {
				vm[m[2]] = m[1]
				continue
			}
			// List<Foo> x = → {"x": "Foo"}
			if m := javaGenericPat.FindStringSubmatch(line); m != nil {
				vm[m[2]] = m[1]
			}
		}
	}
	return vm
}

