package vfs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yifanmeng/codefuse/internal/index"
)

// Generator creates virtual filesystem views from an index
type Generator struct {
	idx     *index.Index
	vfsRoot string
}

// NewGenerator creates a VFS generator
func NewGenerator(idx *index.Index, projectPath string) *Generator {
	return &Generator{
		idx:     idx,
		vfsRoot: filepath.Join(projectPath, ".codefuse", "vfs"),
	}
}

// GenerateAll creates all VFS views
func (g *Generator) GenerateAll() error {
	if err := os.RemoveAll(g.vfsRoot); err != nil {
		return err
	}
	if err := os.MkdirAll(g.vfsRoot, 0755); err != nil {
		return err
	}

	if err := g.generateSymbolViews(); err != nil {
		return fmt.Errorf("symbol views: %w", err)
	}
	if err := g.generateOutlineViews(); err != nil {
		return fmt.Errorf("outline views: %w", err)
	}
	if err := g.generateReferenceViews(); err != nil {
		return fmt.Errorf("reference views: %w", err)
	}

	return nil
}

func (g *Generator) generateSymbolViews() error {
	symbolDir := filepath.Join(g.vfsRoot, "symbols")
	if err := os.MkdirAll(symbolDir, 0755); err != nil {
		return err
	}

	// Group symbols by name
	byName := make(map[string][]index.SymbolDisplay)
	for _, sym := range g.idx.Symbols {
		byName[sym.Name] = append(byName[sym.Name], index.SymbolDisplay{
			Name: sym.Name,
			File: sym.File,
			Line: sym.Line,
			Kind: sym.Kind,
		})
	}

	for name, syms := range byName {
		// Sanitize filename
		filename := sanitizeFilename(name)
		path := filepath.Join(symbolDir, filename)

		var content strings.Builder
		content.WriteString(fmt.Sprintf("# Symbol: %s\n\n", name))
		for _, sym := range syms {
			content.WriteString(fmt.Sprintf("## %s (%s)\n", sym.Name, sym.Kind))
			content.WriteString(fmt.Sprintf("- File: %s:%d\n", sym.File, sym.Line))
			content.WriteString("\n")
		}

		if err := os.WriteFile(path, []byte(content.String()), 0644); err != nil {
			return err
		}
	}

	return nil
}

func (g *Generator) generateOutlineViews() error {
	outlineDir := filepath.Join(g.vfsRoot, "outline")
	if err := os.MkdirAll(outlineDir, 0755); err != nil {
		return err
	}

	// Group symbols by file
	byFile := make(map[string][]index.SymbolDisplay)
	for _, sym := range g.idx.Symbols {
		byFile[sym.File] = append(byFile[sym.File], index.SymbolDisplay{
			Name: sym.Name,
			File: sym.File,
			Line: sym.Line,
			Kind: sym.Kind,
		})
	}

	for file, syms := range byFile {
		// Sort by line number (bubble sort for simplicity)
		for i := 0; i < len(syms); i++ {
			for j := i + 1; j < len(syms); j++ {
				if syms[j].Line < syms[i].Line {
					syms[i], syms[j] = syms[j], syms[i]
				}
			}
		}

		filename := sanitizeFilename(file)
		path := filepath.Join(outlineDir, filename)

		var content strings.Builder
		content.WriteString(fmt.Sprintf("# Outline: %s\n\n", file))
		for _, sym := range syms {
			content.WriteString(fmt.Sprintf("L%03d\t%s\t%s\n", sym.Line, sym.Kind, sym.Name))
		}

		if err := os.WriteFile(path, []byte(content.String()), 0644); err != nil {
			return err
		}
	}

	return nil
}

func (g *Generator) generateReferenceViews() error {
	refDir := filepath.Join(g.vfsRoot, "references")
	if err := os.MkdirAll(refDir, 0755); err != nil {
		return err
	}

	// For now, create a simple call graph summary
	// TODO: implement actual reference analysis with tree-sitter
	var content strings.Builder
	content.WriteString("# References\n\n")
	content.WriteString("Reference analysis requires tree-sitter parsing.\n")
	content.WriteString(fmt.Sprintf("Total symbols indexed: %d\n", len(g.idx.Symbols)))

	return os.WriteFile(filepath.Join(refDir, "README.md"), []byte(content.String()), 0644)
}

func sanitizeFilename(name string) string {
	// Replace path separators and other problematic characters
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	return replacer.Replace(name)
}
