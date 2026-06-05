package types

// FileEntry represents a scanned source file
type FileEntry struct {
	Path        string `json:"path"`
	AbsPath     string `json:"abs_path"`
	Language    string `json:"language"`
	Size        int64  `json:"size"`
	Mtime       int64  `json:"mtime"` // nanoseconds since epoch
	IsGitignore bool   `json:"is_gitignored"`
	IsTest      bool   `json:"is_test"`
}

// Symbol represents a code symbol (function, class, variable, etc.)
type Symbol struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	File       string `json:"file"`
	Line       int    `json:"line"`
	Column     int    `json:"column"`
	EndLine    int    `json:"end_line"`
	Parent     string `json:"parent,omitempty"`
	Signature  string `json:"signature,omitempty"`
	Docstring  string `json:"docstring,omitempty"`
}

// Index represents the complete code index for a project
type Index struct {
	ProjectPath string            `json:"project_path"`
	Files       []FileEntry       `json:"files"`
	Symbols     []Symbol          `json:"symbols"`
	FileMap     map[string]Symbol `json:"-"` // runtime lookup by file
}

// SymbolKind constants
const (
	KindFunction   = "function"
	KindMethod     = "method"
	KindClass      = "class"
	KindStruct     = "struct"
	KindInterface  = "interface"
	KindVariable   = "variable"
	KindConstant   = "constant"
	KindImport     = "import"
	KindPackage    = "package"
	KindModule     = "module"
	KindField      = "field"
	KindEnum       = "enum"
	KindType       = "type"
	KindUnknown    = "unknown"
)

// Language constants
const (
	LangGo       = "go"
	LangPython   = "python"
	LangRust     = "rust"
	LangJS       = "javascript"
	LangTS       = "typescript"
	LangJava     = "java"
	LangC        = "c"
	LangCPP      = "cpp"
	LangUnknown  = "unknown"
)
