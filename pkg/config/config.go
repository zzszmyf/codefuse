// Package config defines language-specific configuration for symbol extraction.
// Each language maps file extensions to tree-sitter node types that represent
// declarations (name extraction points) and call sites (edge extraction points).
//
// Adding a new language requires only adding an entry to the Builtin map —
// no new Go code is needed.
package config

// LangConfig defines how to extract symbols and call edges for a language.
type LangConfig struct {
	Name       string   `json:"name"`
	Extensions []string `json:"extensions"`

	// DeclNodes are tree-sitter node type names that represent symbol declarations.
	// The extractor finds these nodes, extracts the first identifier child as the
	// symbol name, and records the file + line + column position.
	//
	// e.g., Go: ["function_declaration", "method_declaration", "type_spec"]
	//       Python: ["function_definition", "class_definition"]
	//       Java: ["method_declaration", "class_declaration", "interface_declaration"]
	DeclNodes []string `json:"decl_nodes"`

	// CallNodes are tree-sitter node type names that represent function/method calls.
	// The extractor finds these nodes within function bodies and extracts
	// the callee name to build call graph edges.
	//
	// e.g., Go: ["call_expression"]
	//       Python: ["call"]
	//       Java: ["method_invocation"]
	CallNodes []string `json:"call_nodes,omitempty"`

	// NameTags are the tree-sitter child node tags/names that contain the
	// identifier for a declaration. Default: ["identifier", "name", "type_identifier",
	// "property_identifier", "field_identifier"].
	NameTags []string `json:"name_tags,omitempty"`

	// CalleeTags are the tree-sitter child node tags/names that contain the
	// callee name in a call expression. Default: ["identifier", "property_identifier",
	// "field_identifier", "name"].
	CalleeTags []string `json:"callee_tags,omitempty"`

	// TestPatterns are file name patterns that indicate test files.
	// e.g., Go: ["_test.go"], Python: ["test_", "_test.py"]
	TestPatterns []string `json:"test_patterns,omitempty"`
}

// DefaultNameTags are the tree-sitter node tags tried when extracting a symbol name.
var DefaultNameTags = []string{
	"identifier", "name", "type_identifier",
	"property_identifier", "field_identifier",
}

// DefaultCalleeTags are the tree-sitter node tags tried when extracting a callee name.
var DefaultCalleeTags = []string{
	"identifier", "property_identifier", "field_identifier", "name",
}

// Builtin is the built-in language configuration registry.
// To add a new language, add an entry here. No Go code changes needed elsewhere.
var Builtin = map[string]LangConfig{
	"go": {
		Name:     "go",
		Extensions: []string{".go"},
		DeclNodes:  []string{"function_declaration", "method_declaration", "type_spec"},
		CallNodes:  []string{"call_expression"},
		NameTags:   DefaultNameTags,
		CalleeTags: DefaultCalleeTags,
		TestPatterns: []string{"_test.go"},
	},
	"python": {
		Name:     "python",
		Extensions: []string{".py"},
		DeclNodes:  []string{"function_definition", "class_definition"},
		CallNodes:  []string{"call"},
		NameTags:   DefaultNameTags,
		CalleeTags: DefaultCalleeTags,
		TestPatterns: []string{"test_", "_test.py"},
	},
	"rust": {
		Name:     "rust",
		Extensions: []string{".rs"},
		DeclNodes:  []string{"function_item", "struct_item", "enum_item", "trait_item", "impl_item"},
		CallNodes:  []string{"call_expression"},
		NameTags:   DefaultNameTags,
		CalleeTags: append(DefaultCalleeTags, "field_identifier", "scoped_identifier"),
		TestPatterns: []string{"test"},
	},
	"javascript": {
		Name:     "javascript",
		Extensions: []string{".js", ".jsx", ".mjs", ".cjs"},
		DeclNodes:  []string{
			"function_declaration", "class_declaration",
			"method_definition", "lexical_declaration", "variable_declaration",
		},
		CallNodes: []string{"call_expression"},
		NameTags:  DefaultNameTags,
		CalleeTags: append(DefaultCalleeTags, "member_expression"),
		TestPatterns: []string{".test.", ".spec."},
	},
	"typescript": {
		Name:     "typescript",
		Extensions: []string{".ts", ".tsx", ".mts", ".cts"},
		DeclNodes: []string{
			"function_declaration", "class_declaration",
			"method_definition", "lexical_declaration", "variable_declaration",
			"interface_declaration", "enum_declaration", "type_alias_declaration",
		},
		CallNodes: []string{"call_expression"},
		NameTags:  DefaultNameTags,
		CalleeTags: append(DefaultCalleeTags, "member_expression"),
		TestPatterns: []string{".test.", ".spec."},
	},
	"java": {
		Name:     "java",
		Extensions: []string{".java"},
		DeclNodes: []string{
			"method_declaration", "class_declaration",
			"interface_declaration", "enum_declaration",
			"constructor_declaration", "field_declaration",
		},
		CallNodes: []string{"method_invocation", "object_creation_expression"},
		NameTags:  DefaultNameTags,
		CalleeTags: DefaultCalleeTags,
		TestPatterns: []string{"Test.java", "Tests.java"},
	},
	"c": {
		Name:     "c",
		Extensions: []string{".c", ".h"},
		DeclNodes:  []string{"function_definition", "function_declaration"},
		CallNodes:  []string{"call_expression"},
		NameTags:   DefaultNameTags,
		CalleeTags: DefaultCalleeTags,
	},
	"cpp": {
		Name:     "cpp",
		Extensions: []string{".cpp", ".cc", ".cxx", ".hpp", ".hh", ".hxx"},
		DeclNodes:  []string{
			"function_definition", "function_declaration",
			"class_specifier", "struct_specifier",
		},
		CallNodes:  []string{"call_expression"},
		NameTags:   DefaultNameTags,
		CalleeTags: DefaultCalleeTags,
	},
}

// Lookup returns the LangConfig for a file extension, or nil if not found.
func Lookup(ext string) *LangConfig {
	for _, cfg := range Builtin {
		for _, e := range cfg.Extensions {
			if e == ext {
				return &cfg
			}
		}
	}
	return nil
}

// ExtToLang maps file extensions to language names (built from Builtin at init).
var ExtToLang = buildExtToLang()

func buildExtToLang() map[string]string {
	m := make(map[string]string)
	for name, cfg := range Builtin {
		for _, ext := range cfg.Extensions {
			m[ext] = name
		}
	}
	return m
}
