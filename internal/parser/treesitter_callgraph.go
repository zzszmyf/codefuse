package parser

import (
	"bytes"
	"encoding/xml"
	"os/exec"
	"strings"

	"github.com/yifanmeng/codefuse/pkg/types"
)

// ExtractTreeSitterCallGraph extracts call edges from a source file using
// tree-sitter CLI XML output. It performs heuristic matching since tree-sitter
// languages lack static type information for cross-file resolution.
//
// Resolution strategy:
//   - call_expression with identifier: same-file function call
//   - call_expression with member_expression: method call (matches by method name)
//   - Only creates edges when both caller and callee can be uniquely matched
//     to nodes in the same file.
func ExtractTreeSitterCallGraph(absPath, relPath, language string, graph *types.Graph) ([]types.Edge, error) {
	if !TreeSitterAvailable() {
		return nil, nil
	}

	cmd := exec.Command("tree-sitter", "parse", "--xml", absPath)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, nil // Silently skip on tree-sitter failure
	}

	return parseCallGraphFromXML(out.Bytes(), relPath, graph)
}

// parseCallGraphFromXML parses tree-sitter XML AST and extracts call edges.
func parseCallGraphFromXML(data []byte, relPath string, graph *types.Graph) ([]types.Edge, error) {
	var doc struct {
		Sources []tsSource `xml:"source"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if len(doc.Sources) == 0 {
		return nil, nil
	}

	var edges []types.Edge
	for _, node := range doc.Sources[0].Program.Nodes {
		findCallSites(node, relPath, "", &edges, graph)
	}
	return edges, nil
}

// findCallSites recursively traverses AST nodes to find call expressions.
// enclosingFunc is the name of the closest enclosing function/method.
func findCallSites(node tsNode, relPath, enclosingFunc string, edges *[]types.Edge, graph *types.Graph) {
	// Update enclosing function when entering a new function declaration.
	if name := extractFunctionName(node); name != "" {
		enclosingFunc = name
	}

	if node.XMLName.Local == "call_expression" {
		calleeName := extractCalleeName(node)
		if calleeName != "" && enclosingFunc != "" {
			callerID := findNodeIDByNameAndFile(graph, enclosingFunc, relPath)
			calleeID := findNodeIDByNameAndFile(graph, calleeName, relPath)
			if callerID != "" && calleeID != "" && callerID != calleeID {
				*edges = append(*edges, types.Edge{
					From: callerID,
					To:   calleeID,
					Kind: types.EdgeKindCalls,
					File: relPath,
					Line: node.SRow + 1, // tree-sitter uses 0-based
				})
			}
		}
	}

	for _, child := range node.Nodes {
		findCallSites(child, relPath, enclosingFunc, edges, graph)
	}
}

// extractFunctionName returns the name of a function/method declaration node.
func extractFunctionName(node tsNode) string {
	switch node.XMLName.Local {
	case "function_declaration", "function_definition":
		return findNamedChild(node, "identifier", "name")
	case "method_definition":
		return findNamedChild(node, "property_identifier", "identifier", "name")
	case "arrow_function":
		// Arrow functions often don't have a name at declaration site.
		// Check if parent is a variable_declarator to get the variable name.
		return ""
	}
	return ""
}

// extractCalleeName returns the name of the function/method being called.
func extractCalleeName(node tsNode) string {
	// node is call_expression; look for the function child.
	for _, child := range node.Nodes {
		switch child.XMLName.Local {
		case "identifier":
			if child.Name != "" {
				return child.Name
			}
			if text := strings.TrimSpace(child.Chardata); text != "" {
				return text
			}
		case "member_expression":
			// Return the method/property name being called
			return findNamedChild(child, "property_identifier", "identifier", "name")
		case "field_expression":
			// Rust: field_expression for method calls
			return findNamedChild(child, "field_identifier", "identifier", "name")
		case "scoped_identifier":
			// Rust: std::foo::bar()
			return findNamedChild(child, "identifier", "name")
		}
	}
	return ""
}

// findNodeIDByNameAndFile finds a node ID by name in a specific file.
// Returns empty string if no unique match is found.
func findNodeIDByNameAndFile(graph *types.Graph, name, file string) string {
	if graph == nil || name == "" {
		return ""
	}
	candidates := graph.FindNodeByName(name, "")
	var match string
	for _, node := range candidates {
		if node.File == file {
			if match != "" {
				// Multiple matches in same file — ambiguous, skip.
				return ""
			}
			match = node.ID
		}
	}
	return match
}
