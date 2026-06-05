package vfs

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yifanmeng/codefuse/internal/index"
	"github.com/yifanmeng/codefuse/pkg/types"
)

// Generator creates virtual filesystem views from a graph index.
type Generator struct {
	graph     *index.Graph
	vfsRoot   string
}

// NewGenerator creates a VFS generator.
func NewGenerator(graph *index.Graph, projectPath string) *Generator {
	return &Generator{
		graph:   graph,
		vfsRoot: filepath.Join(projectPath, ".codefuse", "vfs"),
	}
}

// GenerateAll creates all VFS views.
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

	// Group nodes by name
	byName := make(map[string][]types.Node)
	for _, node := range g.graph.Nodes {
		byName[node.Name] = append(byName[node.Name], node)
	}

	for name, nodes := range byName {
		filename := sanitizeFilename(name)
		path := filepath.Join(symbolDir, filename)

		var content strings.Builder
		content.WriteString(fmt.Sprintf("# Symbol: %s\n\n", name))
		for _, node := range nodes {
			content.WriteString(fmt.Sprintf("## %s (%s)\n", node.Name, node.Kind))
			content.WriteString(fmt.Sprintf("- File: %s:%d\n", node.File, node.Line))
			if node.Parent != "" {
				content.WriteString(fmt.Sprintf("- Parent: %s\n", node.Parent))
			}
			if node.Signature != "" {
				content.WriteString(fmt.Sprintf("- Signature: %s\n", node.Signature))
			}
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

	// Group nodes by file
	byFile := make(map[string][]types.Node)
	for _, node := range g.graph.Nodes {
		byFile[node.File] = append(byFile[node.File], node)
	}

	for file, nodes := range byFile {
		// Sort by line number
		sort.Slice(nodes, func(i, j int) bool {
			return nodes[i].Line < nodes[j].Line
		})

		filename := sanitizeFilename(file)
		path := filepath.Join(outlineDir, filename)

		var content strings.Builder
		content.WriteString(fmt.Sprintf("# Outline: %s\n\n", file))
		for _, node := range nodes {
			content.WriteString(fmt.Sprintf("L%03d\t%s\t%s\n", node.Line, node.Kind, node.Name))
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

	// Ensure indexes are built.
	g.graph.BuildIndexes()

	// Generate a reference file for each unique symbol name.
	byName := make(map[string][]types.Node)
	for _, node := range g.graph.Nodes {
		byName[node.Name] = append(byName[node.Name], node)
	}

	for name, nodes := range byName {
		filename := sanitizeFilename(name)
		path := filepath.Join(refDir, filename)

		var content strings.Builder
		content.WriteString(fmt.Sprintf("# References: %s\n\n", name))

		for _, node := range nodes {
			content.WriteString(fmt.Sprintf("## %s (%s) @ %s:%d\n\n",
				node.Name, node.Kind, node.File, node.Line))

			// Callers (who calls this node)
			callers := g.graph.FindCallers(node.ID)
			if len(callers) > 0 {
				content.WriteString("### Callers\n\n")
				for _, edge := range callers {
					caller := g.graph.FindNodeByID(edge.From)
					if caller != nil {
						content.WriteString(fmt.Sprintf("- `%s` (%s) @ %s:%d\n",
							caller.Name, caller.Kind, edge.File, edge.Line))
					} else {
						content.WriteString(fmt.Sprintf("- `%s` @ %s:%d\n",
							edge.From, edge.File, edge.Line))
					}
				}
				content.WriteString("\n")
			} else {
				content.WriteString("### Callers\n\nNo callers found.\n\n")
			}

			// Callees (who this node calls)
			callees := g.graph.FindCallees(node.ID)
			if len(callees) > 0 {
				content.WriteString("### Callees\n\n")
				for _, edge := range callees {
					callee := g.graph.FindNodeByID(edge.To)
					if callee != nil {
						content.WriteString(fmt.Sprintf("- `%s` (%s) @ %s:%d\n",
							callee.Name, callee.Kind, edge.File, edge.Line))
					} else {
						content.WriteString(fmt.Sprintf("- `%s` @ %s:%d\n",
							edge.To, edge.File, edge.Line))
					}
				}
				content.WriteString("\n")
			} else {
				content.WriteString("### Callees\n\nNo callees found.\n\n")
			}
		}

		if err := os.WriteFile(path, []byte(content.String()), 0644); err != nil {
			return err
		}
	}

	return nil
}

func sanitizeFilename(name string) string {
	replacer := []struct{ old, new string }{
		{"/", "_"}, {"\\", "_"}, {":", "_"},
		{"*", "_"}, {"?", "_"}, {"\"", "_"},
		{"<", "_"}, {">", "_"}, {"|", "_"},
	}
	for _, r := range replacer {
		name = strings.ReplaceAll(name, r.old, r.new)
	}
	return name
}
