package parser

import (
	"bytes"
	"encoding/xml"
	"os/exec"
	"strings"

	"github.com/yifanmeng/codefuse/pkg/types"
)

// TreeSitterAvailable reports whether tree-sitter CLI is installed and usable.
func TreeSitterAvailable() bool {
	_, err := exec.LookPath("tree-sitter")
	return err == nil
}

// ExtractWithTreeSitter uses tree-sitter CLI to parse a source file and extract symbols.
// It falls back to nil if tree-sitter is unavailable or parsing fails.
// Requires tree-sitter CLI to be installed and grammar repos to be discoverable
// (via tree-sitter's config.json parser-directories).
func ExtractWithTreeSitter(absPath, relPath, language string) ([]types.Symbol, error) {
	if !TreeSitterAvailable() {
		return nil, nil
	}

	cmd := exec.Command("tree-sitter", "parse", "--xml", absPath)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// tree-sitter may fail if grammar is not installed/configured
		return nil, nil
	}

	syms, err := parseTreeSitterXML(out.Bytes(), relPath, language)
	if err != nil {
		return nil, nil
	}
	return syms, nil
}

// =============================================================================
// XML AST Parsing
// =============================================================================

type tsSource struct {
	XMLName xml.Name    `xml:"source"`
	Name    string      `xml:"name,attr"`
	Program tsProgram   `xml:"program"`
}

type tsProgram struct {
	XMLName xml.Name   `xml:"program"`
	Nodes   []tsNode   `xml:",any"`
}

type tsNode struct {
	XMLName xml.Name `xml:""`
	SRow    int      `xml:"srow,attr"`
	SCol    int      `xml:"scol,attr"`
	ERow    int      `xml:"erow,attr"`
	ECol    int      `xml:"ecol,attr"`
	Name    string   `xml:"name,attr"`
	Value   string   `xml:"value,attr"`
	Pattern string   `xml:"pattern,attr"`
	Nodes   []tsNode `xml:",any"`
	Chardata string `xml:",chardata"`
}

func parseTreeSitterXML(data []byte, relPath, language string) ([]types.Symbol, error) {
	// tree-sitter outputs a <?xml ...?> declaration; xml.Unmarshal handles it
	var doc struct {
		Sources []tsSource `xml:"source"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if len(doc.Sources) == 0 {
		return nil, nil
	}

	var syms []types.Symbol
	var exports map[string]bool

	// First pass: collect exports
	exports = make(map[string]bool)
	for _, node := range doc.Sources[0].Program.Nodes {
		collectExports(node, exports)
	}

	// Second pass: collect symbols
	for _, node := range doc.Sources[0].Program.Nodes {
		syms = append(syms, extractFromNode(node, relPath, language, exports)...)
	}

	return syms, nil
}

func collectExports(n tsNode, exports map[string]bool) {
	switch n.XMLName.Local {
	case "export_statement":
		// Look for identifier child with name
		for _, child := range n.Nodes {
			if child.XMLName.Local == "identifier" && child.Name != "" {
				exports[child.Name] = true
			}
			// Also recurse for nested exports
			collectExports(child, exports)
		}
	default:
		for _, child := range n.Nodes {
			collectExports(child, exports)
		}
	}
}

func extractFromNode(n tsNode, relPath, language string, exports map[string]bool) []types.Symbol {
	var syms []types.Symbol

	switch n.XMLName.Local {
	// --- TypeScript / JavaScript ---
	case "function_declaration":
		if name := findNamedChild(n, "identifier", "name"); name != "" {
			syms = append(syms, types.Symbol{
				Name:     name,
				Kind:     types.KindFunction,
				File:     relPath,
				Line:     n.SRow + 1, // tree-sitter uses 0-based

			})
		}
	case "class_declaration":
		if name := findNamedChild(n, "identifier", "type_identifier", "name"); name != "" {
			syms = append(syms, types.Symbol{
				Name:     name,
				Kind:     types.KindClass,
				File:     relPath,
				Line:     n.SRow + 1,

			})
		}
		// Also extract methods
		for _, child := range n.Nodes {
			if child.XMLName.Local == "class_body" || child.XMLName.Local == "statement_block" {
				for _, member := range child.Nodes {
					if member.XMLName.Local == "method_definition" {
						if mname := findNamedChild(member, "property_identifier", "identifier", "name"); mname != "" {
							syms = append(syms, types.Symbol{
								Name:     mname,
								Kind:     types.KindMethod,
								File:     relPath,
								Line:     member.SRow + 1,
								Parent:   findNamedChild(n, "identifier", "type_identifier", "name"),
	
							})
						}
					}
				}
			}
		}
	case "interface_declaration":
		if name := findNamedChild(n, "type_identifier", "identifier", "name"); name != "" {
			syms = append(syms, types.Symbol{
				Name:     name,
				Kind:     types.KindInterface,
				File:     relPath,
				Line:     n.SRow + 1,

			})
		}
	case "type_alias_declaration":
		if name := findNamedChild(n, "type_identifier", "identifier", "name"); name != "" {
			syms = append(syms, types.Symbol{
				Name:     name,
				Kind:     types.KindType,
				File:     relPath,
				Line:     n.SRow + 1,

			})
		}
	case "enum_declaration":
		if name := findNamedChild(n, "identifier", "name"); name != "" {
			syms = append(syms, types.Symbol{
				Name:     name,
				Kind:     types.KindEnum,
				File:     relPath,
				Line:     n.SRow + 1,

			})
		}
	case "lexical_declaration", "variable_declaration":
		for _, child := range n.Nodes {
			if child.XMLName.Local == "variable_declarator" {
				if name := findNamedChild(child, "identifier", "name"); name != "" {
					kind := types.KindVariable
					// Check if value is an arrow_function or function
					for _, vchild := range child.Nodes {
						if vchild.XMLName.Local == "arrow_function" || vchild.XMLName.Local == "function" {
							kind = types.KindFunction
							break
						}
					}
					syms = append(syms, types.Symbol{
						Name:     name,
						Kind:     kind,
						File:     relPath,
						Line:     child.SRow + 1,
		
					})
				}
			}
		}

	// --- Python ---
	case "function_definition":
		if name := findNamedChild(n, "identifier", "name"); name != "" {
			syms = append(syms, types.Symbol{
				Name: name,
				Kind: types.KindFunction,
				File: relPath,
				Line: n.SRow + 1,
			})
		}
	case "class_definition":
		if name := findNamedChild(n, "identifier", "name"); name != "" {
			syms = append(syms, types.Symbol{
				Name: name,
				Kind: types.KindClass,
				File: relPath,
				Line: n.SRow + 1,
			})
		}

	// --- Go (tree-sitter-go) ---
	// function_declaration is already handled above (same logic as TS/JS)
	case "method_declaration":
		if name := findNamedChild(n, "field_identifier", "identifier", "name"); name != "" {
			parent := ""
			// receiver is in method's parameter_list or receiver node
			for _, child := range n.Nodes {
				if child.XMLName.Local == "parameter_list" || child.XMLName.Local == "receiver" {
					for _, pchild := range child.Nodes {
						if pchild.XMLName.Local == "pointer_type" || pchild.XMLName.Local == "type_identifier" {
							if pname := findNamedChild(pchild, "type_identifier", "identifier", "name"); pname != "" {
								parent = pname
								break
							}
						}
					}
				}
			}
			syms = append(syms, types.Symbol{
				Name:   name,
				Kind:   types.KindMethod,
				File:   relPath,
				Line:   n.SRow + 1,
				Parent: parent,
			})
		}
	case "type_declaration":
		for _, child := range n.Nodes {
			if child.XMLName.Local == "type_spec" {
				if name := findNamedChild(child, "type_identifier", "identifier", "name"); name != "" {
					kind := types.KindStruct
					// Check if type is interface_type
					for _, tchild := range child.Nodes {
						if tchild.XMLName.Local == "interface_type" {
							kind = types.KindInterface
							break
						}
					}
					syms = append(syms, types.Symbol{
						Name: name,
						Kind: kind,
						File: relPath,
						Line: child.SRow + 1,
					})
				}
			}
		}
	}

	// Recurse into children for nested declarations
	for _, child := range n.Nodes {
		syms = append(syms, extractFromNode(child, relPath, language, exports)...)
	}

	return syms
}

// findNamedChild searches children for an identifier-like node and returns its text content.
func findNamedChild(n tsNode, tagNames ...string) string {
	for _, child := range n.Nodes {
		for _, tag := range tagNames {
			if child.XMLName.Local == tag {
				if child.Name != "" {
					return child.Name
				}
				// Sometimes the text is in chardata
				if text := strings.TrimSpace(child.Chardata); text != "" {
					return text
				}
				// Recurse one level for nested identifier
				if v := findNamedChild(child, tagNames...); v != "" {
					return v
				}
			}
		}
	}
	return ""
}

