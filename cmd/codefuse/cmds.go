package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yifanmeng/codefuse/internal/fusefs"
	"github.com/yifanmeng/codefuse/internal/index"
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

	// Find matching nodes
	results := graph.FindNodeByName(symbolName, kind)
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
