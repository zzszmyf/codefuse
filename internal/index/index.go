package index

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yifanmeng/codefuse/internal/parser"
	"github.com/yifanmeng/codefuse/pkg/types"
)

// SymbolDisplay is a flattened symbol for CLI output
type SymbolDisplay struct {
	Name string
	File string
	Line int
	Kind string
}

// Index holds the complete code index (v0.1 format).
type Index struct {
	Version     string              `json:"version,omitempty"`
	ProjectPath string              `json:"project_path"`
	Files       []types.FileEntry   `json:"files"`
	Symbols     []types.Symbol      `json:"symbols"`
	symbolMap   map[string][]types.Symbol // runtime index by name
}

// Build creates an index from scanned files.
// If useTreeSitter is true, non-Go files are parsed via tree-sitter CLI in batches.
func Build(projectPath string, files []types.FileEntry, useTreeSitter bool) (*Index, error) {
	idx := &Index{
		ProjectPath: projectPath,
		Files:       files,
		Symbols:     make([]types.Symbol, 0),
	}

	var remaining []types.FileEntry
	if useTreeSitter {
		tsResults, failed := parser.BatchExtractWithTreeSitter(files)
		for path, syms := range tsResults {
			idx.Symbols = append(idx.Symbols, syms...)
			_ = path
		}
		remaining = failed
	} else {
		remaining = files
	}

	// Fallback: extract remaining files individually (Go via go/ast, others via regex)
	for _, file := range remaining {
		syms, err := extractSymbols(file)
		if err != nil {
			continue // skip unparseable files
		}
		idx.Symbols = append(idx.Symbols, syms...)
	}

	idx.buildSymbolMap()

	// Save manifest for future incremental indexing
	manifest := &Manifest{
		Version: "1",
		Files:   make(map[string]int64),
	}
	for _, f := range files {
		manifest.Files[f.Path] = f.Mtime
	}
	indexDir := filepath.Join(projectPath, ".codefuse")
	_ = os.MkdirAll(indexDir, 0755)
	_ = saveManifest(indexDir, manifest)

	return idx, nil
}

// Save writes the index to disk
func (idx *Index) Save(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, "index.json"), data, 0644)
}

// Load reads an index from disk.
// Rejects v2+ graph.json indexes with a clear re-index message.
func Load(dir string) (*Index, error) {
	data, err := os.ReadFile(filepath.Join(dir, "index.json"))
	if err != nil {
		return nil, err
	}

	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, err
	}

	// Version check: v0.2+ uses graph.json (not index.json).
	// If index.json has version != "1", it's incompatible.
	if idx.Version != "" && idx.Version != "1" {
		return nil, fmt.Errorf("index format version %s is incompatible (expected v1). Run 'codefuse index .' to re-index", idx.Version)
	}

	idx.buildSymbolMap()
	return &idx, nil
}

// FindSymbol searches for symbols by name. Supports exact match, prefix match,
// and glob patterns (*, ?, [abc]).
func (idx *Index) FindSymbol(name string, kind string) []types.Symbol {
	// Auto-detect glob patterns
	if strings.ContainsAny(name, "*?[") {
		return idx.FindSymbolGlob(name, kind)
	}

	var results []types.Symbol
	for _, sym := range idx.Symbols {
		match := sym.Name == name || strings.HasPrefix(sym.Name, name+".") || strings.HasPrefix(name, sym.Name+".")
		if kind != "" && sym.Kind != kind {
			match = false
		}
		if match {
			results = append(results, sym)
		}
	}
	return results
}

// FindSymbolGlob searches symbols using glob patterns.
func (idx *Index) FindSymbolGlob(pattern string, kind string) []types.Symbol {
	var results []types.Symbol
	for _, sym := range idx.Symbols {
		match, _ := path.Match(pattern, sym.Name)
		if !match {
			continue
		}
		if kind != "" && sym.Kind != kind {
			continue
		}
		results = append(results, sym)
	}
	return results
}

func (idx *Index) buildSymbolMap() {
	idx.symbolMap = make(map[string][]types.Symbol)
	for _, sym := range idx.Symbols {
		idx.symbolMap[sym.Name] = append(idx.symbolMap[sym.Name], sym)
	}
}

// GetOutline returns all symbols in a specific file, sorted by line number
func (idx *Index) GetOutline(filePath string) []SymbolDisplay {
	var result []SymbolDisplay
	for _, sym := range idx.Symbols {
		if sym.File == filePath {
			result = append(result, SymbolDisplay{
				Name: sym.Name,
				File: sym.File,
				Line: sym.Line,
				Kind: sym.Kind,
			})
		}
	}
	// Sort by line number
	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].Line < result[i].Line {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result
}

// extractSymbols uses language-specific parsers.
// Go files use go/ast (100% accurate). Others prefer tree-sitter CLI if available,
// falling back to regex-based extraction.
func extractSymbols(file types.FileEntry) ([]types.Symbol, error) {
	switch file.Language {
	case types.LangGo:
		content, err := os.ReadFile(file.AbsPath)
		if err != nil {
			return nil, err
		}
		return parser.ExtractGoSymbols(file.Path, content)
	}

	// Try tree-sitter CLI first for non-Go languages
	if syms, err := parser.ExtractWithTreeSitter(file.AbsPath, file.Path, file.Language); err == nil && len(syms) > 0 {
		return syms, nil
	}

	// Fallback to regex
	content, err := os.ReadFile(file.AbsPath)
	if err != nil {
		return nil, err
	}
	switch file.Language {
	case types.LangPython:
		return extractPythonSymbols(file.Path, string(content))
	case types.LangRust:
		return extractRustSymbols(file.Path, string(content))
	case types.LangJS, types.LangTS:
		return extractJSSymbols(file.Path, string(content))
	default:
		return nil, nil
	}
}

var (
	// Go patterns
	goPackagePat = regexp.MustCompile(`^package\s+(\w+)`)
	goImportPat  = regexp.MustCompile(`^import\s+(?:\(\s*"([^"]+)"|'([^']+)'|\s*"([^"]+)"\s*\)|"([^"]+)"|'([^']+)')`)
	goFuncPat    = regexp.MustCompile(`^func\s+(?:\([^)]*\)\s+)?(\w+)\s*\(`)
	goTypePat    = regexp.MustCompile(`^type\s+(\w+)\s+(?:struct|interface)`)
	goMethodPat  = regexp.MustCompile(`^func\s+\([^)]*?\*?\s*(\w+)\s*\)\s+(\w+)\s*\(`)

	// Python patterns
	pyDefPat    = regexp.MustCompile(`^def\s+(\w+)\s*\(`)
	pyClassPat  = regexp.MustCompile(`^class\s+(\w+)`)
	pyImportPat = regexp.MustCompile(`^(?:import|from)\s+(\w+)`)

	// Rust patterns
	rustFnPat   = regexp.MustCompile(`^(?:pub\s+)?fn\s+(\w+)`)
	rustStructPat = regexp.MustCompile(`^(?:pub\s+)?struct\s+(\w+)`)
	rustTraitPat  = regexp.MustCompile(`^(?:pub\s+)?trait\s+(\w+)`)
	rustImplPat   = regexp.MustCompile(`^impl\s+(?:<[^>]+>\s+)?(\w+)`)

	// JS/TS patterns
	jsFuncPat  = regexp.MustCompile(`^(?:export\s+)?(?:async\s+)?function\s+(\w+)`)
	jsClassPat = regexp.MustCompile(`^(?:export\s+)?class\s+(\w+)`)
	jsConstPat = regexp.MustCompile(`^(?:export\s+)?const\s+(\w+)\s*=`)
	jsMethodPat = regexp.MustCompile(`^(?:async\s+)?(\w+)\s*\([^)]*\)\s*\{`)
)

func extractGoSymbols(file string, content string) ([]types.Symbol, error) {
	var syms []types.Symbol
	var currentStruct string

	scanner := bufio.NewScanner(strings.NewReader(content))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		rawLine := scanner.Text()
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}

		// Top-level declarations have no leading whitespace
		isTopLevel := len(rawLine) > 0 && rawLine[0] != ' ' && rawLine[0] != '\t'

		if m := goPackagePat.FindStringSubmatch(line); m != nil {
			syms = append(syms, types.Symbol{Name: m[1], Kind: types.KindPackage, File: file, Line: lineNo})
		} else if m := goTypePat.FindStringSubmatch(line); m != nil {
			syms = append(syms, types.Symbol{Name: m[1], Kind: types.KindStruct, File: file, Line: lineNo, Signature: line})
			currentStruct = m[1]
		} else if m := goFuncPat.FindStringSubmatch(line); m != nil {
			// Check if this is a receiver method: func (T) Method()
			isReceiverMethod := strings.HasPrefix(line, "func (")
			if isTopLevel && !isReceiverMethod {
				currentStruct = ""
			}
			if currentStruct != "" || isReceiverMethod {
				parent := currentStruct
				if parent == "" && isReceiverMethod {
					// Try to extract receiver type from func (t *Type) Method()
					if rm := goMethodPat.FindStringSubmatch(line); rm != nil {
						parent = rm[1]
					}
				}
				syms = append(syms, types.Symbol{Name: m[1], Kind: types.KindMethod, File: file, Line: lineNo, Parent: parent, Signature: line})
			} else {
				syms = append(syms, types.Symbol{Name: m[1], Kind: types.KindFunction, File: file, Line: lineNo, Signature: line})
			}
		} else if m := goMethodPat.FindStringSubmatch(line); m != nil {
			syms = append(syms, types.Symbol{Name: m[2], Kind: types.KindMethod, File: file, Line: lineNo, Parent: m[1], Signature: line})
		}
	}
	return syms, scanner.Err()
}

func extractPythonSymbols(file string, content string) ([]types.Symbol, error) {
	var syms []types.Symbol
	var currentClass string
	var classIndent int

	scanner := bufio.NewScanner(strings.NewReader(content))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := len(line) - len(trimmed)

		if m := pyClassPat.FindStringSubmatch(trimmed); m != nil {
			syms = append(syms, types.Symbol{Name: m[1], Kind: types.KindClass, File: file, Line: lineNo})
			currentClass = m[1]
			classIndent = indent
		} else if m := pyDefPat.FindStringSubmatch(trimmed); m != nil {
			if currentClass != "" && indent > classIndent {
				syms = append(syms, types.Symbol{Name: m[1], Kind: types.KindMethod, File: file, Line: lineNo, Parent: currentClass})
			} else {
				// Def at same or lower indentation than class — it's a standalone function.
				// Reset currentClass since we've exited the class body.
				currentClass = ""
				syms = append(syms, types.Symbol{Name: m[1], Kind: types.KindFunction, File: file, Line: lineNo})
			}
		} else if currentClass != "" && indent <= classIndent {
			// Any non-empty, non-comment line at or before class indentation
			// means we've exited the class body.
			currentClass = ""
		}
	}
	return syms, scanner.Err()
}

func extractRustSymbols(file string, content string) ([]types.Symbol, error) {
	var syms []types.Symbol

	scanner := bufio.NewScanner(strings.NewReader(content))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}

		if m := rustFnPat.FindStringSubmatch(line); m != nil {
			syms = append(syms, types.Symbol{Name: m[1], Kind: types.KindFunction, File: file, Line: lineNo, Signature: line})
		} else if m := rustStructPat.FindStringSubmatch(line); m != nil {
			syms = append(syms, types.Symbol{Name: m[1], Kind: types.KindStruct, File: file, Line: lineNo, Signature: line})
		} else if m := rustTraitPat.FindStringSubmatch(line); m != nil {
			syms = append(syms, types.Symbol{Name: m[1], Kind: types.KindInterface, File: file, Line: lineNo, Signature: line})
		} else if m := rustImplPat.FindStringSubmatch(line); m != nil {
			syms = append(syms, types.Symbol{Name: m[1], Kind: types.KindClass, File: file, Line: lineNo, Signature: line})
		}
	}
	return syms, scanner.Err()
}

func extractJSSymbols(file string, content string) ([]types.Symbol, error) {
	var syms []types.Symbol
	var currentClass string

	scanner := bufio.NewScanner(strings.NewReader(content))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") {
			continue
		}

		if m := jsClassPat.FindStringSubmatch(line); m != nil {
			syms = append(syms, types.Symbol{Name: m[1], Kind: types.KindClass, File: file, Line: lineNo})
			currentClass = m[1]
		} else if m := jsFuncPat.FindStringSubmatch(line); m != nil {
			syms = append(syms, types.Symbol{Name: m[1], Kind: types.KindFunction, File: file, Line: lineNo})
		} else if m := jsConstPat.FindStringSubmatch(line); m != nil {
			syms = append(syms, types.Symbol{Name: m[1], Kind: types.KindVariable, File: file, Line: lineNo})
		} else if m := jsMethodPat.FindStringSubmatch(line); m != nil {
			if currentClass != "" {
				syms = append(syms, types.Symbol{Name: m[1], Kind: types.KindMethod, File: file, Line: lineNo, Parent: currentClass})
			}
		}
	}
	return syms, scanner.Err()
}
