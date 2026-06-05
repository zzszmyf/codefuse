package parser

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/yifanmeng/codefuse/pkg/types"
)

// BatchExtractWithTreeSitter parses multiple files in batches using tree-sitter CLI.
// It groups files by language and invokes tree-sitter once per language group.
// Returns a map of file path -> symbols, and a slice of files that failed to parse.
func BatchExtractWithTreeSitter(files []types.FileEntry) (map[string][]types.Symbol, []types.FileEntry) {
	if !TreeSitterAvailable() {
		return nil, files
	}

	result := make(map[string][]types.Symbol)
	var failed []types.FileEntry

	// Group files by language, skipping Go (handled by go/ast)
	byLang := make(map[string][]types.FileEntry)
	for _, f := range files {
		if f.Language == types.LangGo {
			failed = append(failed, f) // Go handled separately by caller
			continue
		}
		byLang[f.Language] = append(byLang[f.Language], f)
	}

	for lang, group := range byLang {
		// tree-sitter CLI auto-detects language by file extension,
		// so we can mix JS/TS in one call as long as extensions match grammar.
		// Process in chunks to avoid command-line length limits.
		const chunkSize = 500
		for i := 0; i < len(group); i += chunkSize {
			end := i + chunkSize
			if end > len(group) {
				end = len(group)
			}
			chunk := group[i:end]
			syms, err := parseChunk(chunk, lang)
			if err != nil {
				// All files in chunk failed, fallback to individual regex
				failed = append(failed, chunk...)
				continue
			}
			for path, syms := range syms {
				result[path] = syms
			}
		}
	}

	return result, failed
}

func parseChunk(files []types.FileEntry, language string) (map[string][]types.Symbol, error) {
	// Write file paths to temp file for --paths
	tmpDir, err := os.MkdirTemp("", "codefuse-treesitter-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	pathsFile := filepath.Join(tmpDir, "paths.txt")
	var pathContent strings.Builder
	for _, f := range files {
		pathContent.WriteString(f.AbsPath)
		pathContent.WriteByte('\n')
	}
	if err := os.WriteFile(pathsFile, []byte(pathContent.String()), 0644); err != nil {
		return nil, err
	}

	cmd := exec.Command("tree-sitter", "parse", "--paths", pathsFile, "--xml")
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("tree-sitter failed: %w (stderr: %s)", err, stderr.String())
	}

	return parseTreeSitterXMLMulti(out.Bytes(), files)
}

// parseTreeSitterXMLMulti parses XML output containing multiple <source> elements
// and maps symbols back to their original FileEntry.
func parseTreeSitterXMLMulti(data []byte, files []types.FileEntry) (map[string][]types.Symbol, error) {
	var doc struct {
		Sources []tsSource `xml:"source"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}

	// Build map from absolute path to FileEntry for quick lookup
	fileByAbs := make(map[string]types.FileEntry)
	for _, f := range files {
		fileByAbs[f.AbsPath] = f
	}

	result := make(map[string][]types.Symbol)
	for _, src := range doc.Sources {
		f, ok := fileByAbs[src.Name]
		if !ok {
			continue
		}
		var syms []types.Symbol
		exports := make(map[string]bool)
		for _, node := range src.Program.Nodes {
			collectExports(node, exports)
		}
		for _, node := range src.Program.Nodes {
			syms = append(syms, extractFromNode(node, f.Path, f.Language, exports)...)
		}
		result[f.Path] = syms
	}

	return result, nil
}
