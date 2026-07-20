package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yifanmeng/codefuse/internal/index"
	"github.com/yifanmeng/codefuse/internal/parser"
	"github.com/yifanmeng/codefuse/internal/scanner"
)

// =============================================================================
// Index
// =============================================================================

func runIndex(projectPath string) error {
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("cannot access path: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", absPath)
	}

	// Check tree-sitter availability.
	if !parser.TreeSitterAvailable() {
		fmt.Fprintln(os.Stderr, "⚠ tree-sitter CLI not found. Install it first:")
		fmt.Fprintln(os.Stderr, "  npm install -g tree-sitter-cli")
		fmt.Fprintln(os.Stderr, "  codefuse setup treesitter --auto")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Continuing with limited support (some languages may be skipped)...")
	}

	// Scan files.
	fmt.Fprintf(os.Stderr, "Scanning %s...\n", absPath)
	files, err := scanner.Scan(absPath)
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Found %d source files\n", len(files))

	// Build thin index.
	fmt.Fprintf(os.Stderr, "Building index...\n")
	graph, err := index.BuildGraph(absPath, files)
	if err != nil {
		return fmt.Errorf("index build failed: %w", err)
	}

	// Save.
	indexDir := filepath.Join(absPath, ".codefuse")
	if err := graph.Save(indexDir); err != nil {
		return fmt.Errorf("save failed: %w", err)
	}

	fmt.Printf("Indexed %d files, %d symbols, %d edges in %s\n",
		len(files), len(graph.Nodes), len(graph.Edges), absPath)
	return nil
}

// =============================================================================
// List
// =============================================================================

func runList(project string) error {
	absPath, err := filepath.Abs(project)
	if err != nil {
		return err
	}

	indexDir := filepath.Join(absPath, ".codefuse")
	graph, err := index.LoadGraph(indexDir)
	if err != nil {
		return fmt.Errorf("no index found. Run 'codefuse index .' first: %w", err)
	}

	fmt.Printf("Project: %s\n", absPath)
	fmt.Printf("Files:   %d\n", len(graph.Files))
	fmt.Printf("Symbols: %d\n", len(graph.Nodes))
	fmt.Printf("Edges:   %d\n\n", len(graph.Edges))

	// Group by first letter for quick overview.
	byPrefix := make(map[string]int)
	for _, node := range graph.Nodes {
		if len(node.Name) > 0 {
			prefix := strings.ToUpper(node.Name[:1])
			byPrefix[prefix]++
		}
	}
	fmt.Println("Symbol distribution:")
	for _, prefix := range sortedKeys(byPrefix) {
		fmt.Printf("  %s: %d\n", prefix, byPrefix[prefix])
	}

	return nil
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Bubble sort (small map, readability > performance).
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}

// =============================================================================
// Query
// =============================================================================

func runQuery(project, symbolName string, showCallers, showCallees bool) error {
	absPath, err := filepath.Abs(project)
	if err != nil {
		return err
	}

	indexDir := filepath.Join(absPath, ".codefuse")
	// Load full graph (with edges) only when --callers/--callees is used.
	var graph *index.Graph
	if showCallers || showCallees {
		graph, err = index.LoadGraph(indexDir)
	} else {
		graph, err = index.LoadGraphNodes(indexDir)
	}
	if err != nil {
		return fmt.Errorf("no index found. Run 'codefuse index .' first: %w", err)
	}

	results := graph.Query(symbolName)
	if len(results) == 0 {
		fmt.Printf("No symbols found matching '%s'\n", symbolName)
		return nil
	}

	for _, node := range results {
		// Read actual source line for current definition.
		absFile := filepath.Join(absPath, node.File)
		line, _ := index.ReadLine(absFile, node.Line)

		fmt.Printf("%s:%d:%d  %s\n", node.File, node.Line, node.Column, node.Name)
		if line != "" {
			fmt.Printf("  %s\n", line)
		}

		if showCallers {
			edges := graph.FindCallers(node.ID)
			if len(edges) > 0 {
				fmt.Printf("  Callers (%d):\n", len(edges))
				for _, e := range edges {
					fmt.Printf("    → %s @ %s:%d\n", e.Node.Name, e.Edge.File, e.Edge.Line)
				}
			} else {
				fmt.Printf("  Callers: none\n")
			}
		}

		if showCallees {
			edges := graph.FindCallees(node.ID)
			if len(edges) > 0 {
				fmt.Printf("  Callees (%d):\n", len(edges))
				for _, e := range edges {
					fmt.Printf("    → %s @ %s:%d\n", e.Node.Name, e.Edge.File, e.Edge.Line)
				}
			} else {
				fmt.Printf("  Callees: none\n")
			}
		}

		fmt.Println()
	}

	return nil
}

// =============================================================================
// Outline
// =============================================================================

func runOutline(project, filePath string) error {
	absPath, err := filepath.Abs(project)
	if err != nil {
		return err
	}

	indexDir := filepath.Join(absPath, ".codefuse")
	graph, err := index.LoadGraph(indexDir)
	if err != nil {
		return fmt.Errorf("no index found. Run 'codefuse index .' first: %w", err)
	}

	var outline []struct {
		line int
		name string
	}
	for _, node := range graph.Nodes {
		if node.File == filePath || strings.HasSuffix(node.File, filePath) {
			outline = append(outline, struct {
				line int
				name string
			}{node.Line, node.Name})
		}
	}

	if len(outline) == 0 {
		fmt.Printf("No symbols found in '%s'\n", filePath)
		return nil
	}

	// Sort by line.
	for i := 0; i < len(outline); i++ {
		for j := i + 1; j < len(outline); j++ {
			if outline[i].line > outline[j].line {
				outline[i], outline[j] = outline[j], outline[i]
			}
		}
	}

	fmt.Printf("Outline: %s\n\n", filePath)
	for _, s := range outline {
		fmt.Printf("L%03d  %s\n", s.line, s.name)
	}

	return nil
}

// =============================================================================
// Watch
// =============================================================================

func runWatch(project string) error {
	absPath, err := filepath.Abs(project)
	if err != nil {
		return err
	}

	// Ensure index exists.
	indexDir := filepath.Join(absPath, ".codefuse")
	if _, err := os.Stat(filepath.Join(indexDir, "graph.json")); os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "No index found. Building initial index...")
		if err := runIndex(project); err != nil {
			return err
		}
	}

	// Start watching.
	w, err := newFileWatcher(absPath)
	if err != nil {
		return fmt.Errorf("watch failed: %w", err)
	}
	defer w.Close()

	fmt.Printf("Watching %s for changes...\n", absPath)
	fmt.Println("Press Ctrl+C to stop.")
	return w.Watch()
}

// =============================================================================
// Setup
// =============================================================================

func runSetupTreeSitter(project string, auto bool) error {
	absPath, err := filepath.Abs(project)
	if err != nil {
		return err
	}

	if !parser.TreeSitterAvailable() {
		return fmt.Errorf("tree-sitter CLI not found. Install it first:\n" +
			"  npm install -g tree-sitter-cli\n" +
			"Or visit https://tree-sitter.github.io/tree-sitter/")
	}

	fmt.Fprintf(os.Stderr, "Scanning %s for languages...\n", absPath)
	files, err := scanner.Scan(absPath)
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}

	langs := make(map[string]bool)
	for _, f := range files {
		langs[f.Language] = true
	}

	var langList []string
	for l := range langs {
		langList = append(langList, l)
	}

	if len(langList) == 0 {
		fmt.Println("No source files found.")
		return nil
	}

	fmt.Printf("Detected languages: %s\n", strings.Join(langList, ", "))

	missing, err := parser.DetectMissingGrammars(langList)
	if err != nil {
		return err
	}

	if len(missing) == 0 {
		fmt.Println("✓ All tree-sitter grammars are already installed.")
		return nil
	}

	fmt.Printf("\nMissing grammars (%d):\n", len(missing))
	for _, g := range missing {
		fmt.Printf("  • %s  (%s)\n", g.Lang, g.Repo)
	}

	if !auto {
		fmt.Println("\nRun with --auto to install them automatically.")
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	grammarsDir := filepath.Join(home, ".config", "codefuse", "grammars")
	if err := os.MkdirAll(grammarsDir, 0755); err != nil {
		return err
	}

	if err := parser.AddParserDirectory(grammarsDir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not update tree-sitter config: %v\n", err)
	}

	for _, g := range missing {
		if err := parser.InstallGrammar(g, grammarsDir); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to install %s: %v\n", g.Lang, err)
			continue
		}
		fmt.Printf("✓ Installed %s grammar\n", g.Lang)
	}

	fmt.Println("\nSetup complete. Run 'codefuse index .' to build the index.")
	return nil
}
