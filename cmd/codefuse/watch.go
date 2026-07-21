package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/yifanmeng/codefuse/internal/index"
	"github.com/yifanmeng/codefuse/internal/parser"
	"github.com/yifanmeng/codefuse/internal/scanner"
	"github.com/yifanmeng/codefuse/pkg/types"
)

// watcherSkipDirs lists directory names to skip during file watching.
var watcherSkipDirs = map[string]bool{
	".git": true, ".svn": true, ".hg": true,
	"node_modules": true, "vendor": true, "dist": true,
	"build": true, "target": true, ".idea": true, ".vscode": true,
	"__pycache__": true, ".pytest_cache": true, ".mypy_cache": true,
	".tox": true, ".egg-info": true, ".venv": true, "venv": true,
	".codefuse": true,
}

// fileWatcher watches a project directory and updates the thin index on changes.
type fileWatcher struct {
	projectPath string
	indexDir    string
	watcher     *fsnotify.Watcher
	graph       *index.Graph
	mu          sync.Mutex
	pending     map[string]*time.Timer
	debounce    time.Duration
}

func newFileWatcher(projectPath string) (*fileWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("fsnotify: %w", err)
	}

	if err := addRecursive(w, projectPath); err != nil {
		w.Close()
		return nil, err
	}

	indexDir := filepath.Join(projectPath, ".codefuse")
	graph, err := index.LoadGraph(indexDir)
	if err != nil {
		graph = index.NewGraph(projectPath)
	}

	return &fileWatcher{
		projectPath: projectPath,
		indexDir:    indexDir,
		watcher:     w,
		graph:       graph,
		pending:     make(map[string]*time.Timer),
		debounce:    200 * time.Millisecond,
	}, nil
}

func (fw *fileWatcher) Close() error {
	return fw.watcher.Close()
}

func (fw *fileWatcher) Watch() error {
	rebuildTicker := time.NewTicker(5 * time.Minute)
	defer rebuildTicker.Stop()

	for {
		select {
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return nil
			}
			fw.handleEvent(event)

		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return nil
			}
			fmt.Fprintf(os.Stderr, "watch error: %v\n", err)

		case <-rebuildTicker.C:
			fw.periodicRebuild()
		}
	}
}

func (fw *fileWatcher) handleEvent(event fsnotify.Event) {
	// Handle Delete/Rename: remove nodes for the deleted file.
	if event.Op&(fsnotify.Remove|fsnotify.Rename) != 0 {
		fw.mu.Lock()
		relPath, _ := filepath.Rel(fw.projectPath, event.Name)
		fw.removeFileNodes(relPath)
		fw.graph.BuildIndexes()
		fw.graph.BuildTrie()
		fw.graph.Save(fw.indexDir)
		fw.mu.Unlock()
		fmt.Fprintf(os.Stderr, "  ✗ removed %s\n", relPath)
		return
	}

	// Only Write and Create events on source files.
	if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
		return
	}
	if isSkippedPath(event.Name) {
		return
	}
	ext := filepath.Ext(event.Name)
	if ext == "" || detectLang(ext) == "" {
		return
	}

	fw.mu.Lock()
	if timer, ok := fw.pending[event.Name]; ok {
		timer.Reset(fw.debounce)
		fw.mu.Unlock()
		return
	}

	timer := time.AfterFunc(fw.debounce, func() {
		fw.reindexFile(event.Name)
		fw.mu.Lock()
		delete(fw.pending, event.Name)
		fw.mu.Unlock()
	})
	fw.pending[event.Name] = timer
	fw.mu.Unlock()
}

func (fw *fileWatcher) reindexFile(absPath string) {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	relPath, _ := filepath.Rel(fw.projectPath, absPath)
	ext := filepath.Ext(absPath)
	lang := detectLang(ext)
	if lang == "" {
		return
	}

	nodes, edges, sinks, err := parser.ExtractFile(absPath, relPath, lang)
	if err != nil || len(nodes) == 0 {
		return
	}

	// Remove old nodes, edges, and sinks for this file.
	fw.removeFileNodes(relPath)

	// Add new nodes, edges, and sinks.
	fw.graph.Nodes = append(fw.graph.Nodes, nodes...)
	for _, edge := range edges {
		resolved := resolveEdgeToGraph(edge, &fw.graph.Graph)
		fw.graph.Edges = append(fw.graph.Edges, resolved...)
	}
	fw.graph.Sinks = append(fw.graph.Sinks, sinks...)

	// Rebuild indexes.
	fw.graph.BuildIndexes()
	fw.graph.BuildTrie()

	// Save.
	_ = fw.graph.Save(fw.indexDir)

	fmt.Fprintf(os.Stderr, "  updated %s (%d symbols, %d sinks)\n", relPath, len(nodes), len(sinks))
}

func (fw *fileWatcher) removeFileNodes(relPath string) {
	var keptNodes []types.Node
	for _, n := range fw.graph.Nodes {
		if n.File != relPath {
			keptNodes = append(keptNodes, n)
		}
	}
	fw.graph.Nodes = keptNodes

	var keptEdges []types.Edge
	for _, e := range fw.graph.Edges {
		if e.File != relPath {
			keptEdges = append(keptEdges, e)
		}
	}
	fw.graph.Edges = keptEdges

	var keptSinks []types.Sink
	for _, s := range fw.graph.Sinks {
		if s.File != relPath {
			keptSinks = append(keptSinks, s)
		}
	}
	fw.graph.Sinks = keptSinks
}

func (fw *fileWatcher) periodicRebuild() {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	files, err := scanner.Scan(fw.projectPath)
	if err != nil {
		return
	}

	// Rebuild if file count changed by more than 10.
	if absDiff(len(files), len(fw.graph.Files)) > 10 {
		fmt.Fprintf(os.Stderr, "  full rebuild (%d files changed)\n",
			absDiff(len(files), len(fw.graph.Files)))
		graph, err := index.BuildGraph(fw.projectPath, files)
		if err == nil {
			fw.graph = graph
		}
	}
}

// =============================================================================
// Helpers
// =============================================================================

func addRecursive(w *fsnotify.Watcher, dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			return nil
		}
		name := info.Name()
		if strings.HasPrefix(name, ".") && name != "." {
			return filepath.SkipDir
		}
		if watcherSkipDirs[name] {
			return filepath.SkipDir
		}
		return w.Add(path)
	})
}

func isSkippedPath(path string) bool {
	parts := strings.Split(filepath.ToSlash(path), "/")
	for _, p := range parts {
		if watcherSkipDirs[p] {
			return true
		}
	}
	return false
}

func detectLang(ext string) string {
	for name, cfg := range parser.BuiltinConfig() {
		for _, e := range cfg.Extensions {
			if e == ext {
				return name
			}
		}
	}
	return ""
}

func resolveEdgeToGraph(edge types.Edge, g *types.Graph) []types.Edge {
	calleeName := edge.To
	candidates := g.FindNodeByName(calleeName)
	if len(candidates) == 0 {
		return nil
	}
	var edges []types.Edge
	for _, callee := range candidates {
		edges = append(edges, types.Edge{
			From: edge.From,
			To:   callee.ID,
			Kind: edge.Kind,
			File: edge.File,
			Line: edge.Line,
		})
	}
	return edges
}

func absDiff(a, b int) int {
	if a > b {
		return a - b
	}
	return b - a
}
