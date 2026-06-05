package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yifanmeng/codefuse/internal/fusefs"
	"github.com/yifanmeng/codefuse/internal/index"
	"github.com/yifanmeng/codefuse/internal/parser"
	"github.com/yifanmeng/codefuse/internal/scanner"
	"github.com/yifanmeng/codefuse/internal/vfs"
	"github.com/yifanmeng/codefuse/pkg/types"
)

func runIndex(projectPath string, force, useTreeSitter bool) error {
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

	indexDir := filepath.Join(absPath, ".codefuse")
	if !force {
		if _, err := os.Stat(indexDir); err == nil {
			// Check if index is up to date
			// For now, just re-index
		}
	}

	// Remove old index
	os.RemoveAll(indexDir)
	if err := os.MkdirAll(indexDir, 0755); err != nil {
		return fmt.Errorf("cannot create index dir: %w", err)
	}

	// Phase 1: Scan files
	fmt.Fprintf(os.Stderr, "Scanning %s...\n", absPath)
	files, err := scanner.Scan(absPath)
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Found %d files\n", len(files))

	// Phase 2: Build graph index (v0.2+)
	fmt.Fprintf(os.Stderr, "Building index...\n")
	graph, err := index.BuildGraph(absPath, files, useTreeSitter)
	if err != nil {
		return fmt.Errorf("index build failed: %w", err)
	}

	// Phase 3: Save graph
	if err := graph.Save(indexDir); err != nil {
		return fmt.Errorf("save index failed: %w", err)
	}

	fmt.Printf("Indexed %d files, %d nodes, %d edges in %s\n",
		len(files), len(graph.Nodes), len(graph.Edges), absPath)
	return nil
}

func runList(project string) error {
	absPath, err := filepath.Abs(project)
	if err != nil {
		return err
	}

	indexDir := filepath.Join(absPath, ".codefuse")
	graph, err := index.LoadAny(indexDir)
	if err != nil {
		return err
	}

	fmt.Printf("Project: %s\n", absPath)
	fmt.Printf("Files:   %d\n", len(graph.Files))
	fmt.Printf("Nodes:   %d\n", len(graph.Nodes))
	fmt.Printf("Edges:   %d\n\n", len(graph.Edges))

	// Group nodes by kind
	byKind := make(map[string][]types.Node)
	for _, node := range graph.Nodes {
		byKind[node.Kind] = append(byKind[node.Kind], node)
	}

	for kind, nodes := range byKind {
		fmt.Printf("[%s] (%d)\n", strings.ToUpper(kind), len(nodes))
		for _, node := range nodes {
			fmt.Printf("  %s\t%s:%d\n", node.Name, node.File, node.Line)
		}
		fmt.Println()
	}

	return nil
}

func runQuery(project, symbolName, kind string, callers, callees bool) error {
	absPath, err := filepath.Abs(project)
	if err != nil {
		return err
	}

	indexDir := filepath.Join(absPath, ".codefuse")
	graph, err := index.LoadAny(indexDir)
	if err != nil {
		return err
	}

	// Find matching nodes (smart query: exact | prefix | glob)
	results := graph.Query(symbolName, kind)
	if len(results) == 0 {
		fmt.Printf("No symbols found matching '%s'\n", symbolName)
		return nil
	}

	for _, node := range results {
		fmt.Printf("%s %s\n", node.Kind, node.Name)
		fmt.Printf("  ID:   %s\n", node.ID)
		fmt.Printf("  File: %s:%d\n", node.File, node.Line)
		if node.Signature != "" {
			fmt.Printf("  Signature: %s\n", node.Signature)
		}
		if node.Docstring != "" {
			fmt.Printf("  Doc: %s\n", node.Docstring)
		}

		// Call graph analysis
		if callers {
			edges := graph.FindCallers(node.ID)
			if len(edges) > 0 {
				fmt.Printf("\n  Callers (%d):\n", len(edges))
				for _, edge := range edges {
					caller := graph.FindNodeByID(edge.From)
					if caller != nil {
						fmt.Printf("    → %s (%s) @ %s:%d\n", caller.Name, caller.Kind, edge.File, edge.Line)
					} else {
						fmt.Printf("    → %s @ %s:%d\n", edge.From, edge.File, edge.Line)
					}
				}
			} else {
				fmt.Printf("\n  Callers: none\n")
			}
		}

		if callees {
			edges := graph.FindCallees(node.ID)
			if len(edges) > 0 {
				fmt.Printf("\n  Callees (%d):\n", len(edges))
				for _, edge := range edges {
					callee := graph.FindNodeByID(edge.To)
					if callee != nil {
						fmt.Printf("    → %s (%s) @ %s:%d\n", callee.Name, callee.Kind, edge.File, edge.Line)
					} else {
						fmt.Printf("    → %s @ %s:%d\n", edge.To, edge.File, edge.Line)
					}
				}
			} else {
				fmt.Printf("\n  Callees: none\n")
			}
		}

		fmt.Println()
	}

	return nil
}

func runOutline(project, filePath string) error {
	absPath, err := filepath.Abs(project)
	if err != nil {
		return err
	}

	indexDir := filepath.Join(absPath, ".codefuse")
	graph, err := index.LoadAny(indexDir)
	if err != nil {
		return err
	}

	var outline []types.Node
	for _, node := range graph.Nodes {
		if node.File == filePath {
			outline = append(outline, node)
		}
	}

	if len(outline) == 0 {
		fmt.Printf("No symbols found in '%s'\n", filePath)
		return nil
	}

	fmt.Printf("Outline: %s\n\n", filePath)
	for _, node := range outline {
		fmt.Printf("L%03d\t%s\t%s\n", node.Line, node.Kind, node.Name)
	}

	return nil
}

func runVFSGenerate(project string) error {
	absPath, err := filepath.Abs(project)
	if err != nil {
		return err
	}

	indexDir := filepath.Join(absPath, ".codefuse")
	graph, err := index.LoadAny(indexDir)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Generating VFS views...\n")
	gen := vfs.NewGenerator(graph, absPath)
	if err := gen.GenerateAll(); err != nil {
		return fmt.Errorf("vfs generation failed: %w", err)
	}

	fmt.Printf("VFS views generated in %s\n", filepath.Join(absPath, ".codefuse", "vfs"))
	return nil
}

func runMount(project, mountpoint string) error {
	if !fusefs.IsSupported() {
		return fmt.Errorf("FUSE is not supported on this system. " +
			"On macOS, install macFUSE (https://macfuse.io/). " +
			"On Linux, ensure /dev/fuse exists.")
	}

	absPath, err := filepath.Abs(project)
	if err != nil {
		return err
	}

	indexDir := filepath.Join(absPath, ".codefuse")
	graph, err := index.LoadAny(indexDir)
	if err != nil {
		return err
	}

	return fusefs.Mount(graph, mountpoint)
}

func runSetupTreeSitter(project string, auto bool) error {
	absPath, err := filepath.Abs(project)
	if err != nil {
		return err
	}

	// Check tree-sitter CLI
	if !parser.TreeSitterAvailable() {
		return fmt.Errorf("tree-sitter CLI not found. Install it first:\n" +
			"  npm install -g tree-sitter-cli\n" +
			"Or visit https://tree-sitter.github.io/tree-sitter/")
	}

	// Scan project to detect languages
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

	// Check missing grammars
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
		fmt.Println("\nRun with --auto to install them automatically, or install manually:")
		for _, g := range missing {
			fmt.Printf("  git clone https://github.com/%s.git && cd %s && tree-sitter build --wasm\n",
				g.Repo, g.DirName)
		}
		return nil
	}

	// Auto-install
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	grammarsDir := filepath.Join(home, ".config", "codefuse", "grammars")
	if err := os.MkdirAll(grammarsDir, 0755); err != nil {
		return err
	}

	// Add to tree-sitter config
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

	fmt.Println("\nSetup complete. You can now use --treesitter for higher accuracy parsing.")
	return nil
}
