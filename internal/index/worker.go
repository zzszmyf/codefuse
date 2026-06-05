package index

import (
	"runtime"
	"sync"

	"github.com/yifanmeng/codefuse/pkg/types"
)

// nodeResult holds the output of parsing a single file for nodes.
type nodeResult struct {
	filePath string
	nodes    []types.Node
	pkgName  string
	err      error
}

// edgeResult holds the output of parsing a single file for edges.
type edgeResult struct {
	filePath string
	edges    []types.Edge
	err      error
}

// workerCount returns the number of workers to use.
// Defaults to runtime.NumCPU(), minimum 1.
func workerCount() int {
	n := runtime.NumCPU()
	if n < 1 {
		return 1
	}
	return n
}

// buildNodesParallel extracts nodes from all files in parallel.
func buildNodesParallel(files []types.FileEntry) ([]types.Node, map[string]string) {
	workers := workerCount()
	if len(files) < workers {
		workers = len(files)
	}
	if workers == 1 {
		// Fallback to serial for small projects.
		return buildNodesSerial(files)
	}

	jobs := make(chan types.FileEntry, len(files))
	results := make(chan nodeResult, len(files))

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for file := range jobs {
				nodes, pkgName, err := extractNodes(file)
				results <- nodeResult{
					filePath: file.Path,
					nodes:    nodes,
					pkgName:  pkgName,
					err:      err,
				}
			}
		}()
	}

	// Distribute work.
	go func() {
		for _, file := range files {
			jobs <- file
		}
		close(jobs)
	}()

	// Close results when all workers are done.
	go func() {
		wg.Wait()
		close(results)
	}()

	var allNodes []types.Node
	pkgNames := make(map[string]string)
	for r := range results {
		if r.err == nil {
			allNodes = append(allNodes, r.nodes...)
			if r.pkgName != "" {
				pkgNames[r.filePath] = r.pkgName
			}
		}
	}
	return allNodes, pkgNames
}

// buildNodesSerial is the single-threaded fallback.
func buildNodesSerial(files []types.FileEntry) ([]types.Node, map[string]string) {
	var allNodes []types.Node
	pkgNames := make(map[string]string)
	for _, file := range files {
		nodes, pkgName, err := extractNodes(file)
		if err == nil {
			allNodes = append(allNodes, nodes...)
			if pkgName != "" {
				pkgNames[file.Path] = pkgName
			}
		}
	}
	return allNodes, pkgNames
}

// buildEdgesParallel extracts call graph edges from all files in parallel.
func buildEdgesParallel(files []types.FileEntry, pkgNames map[string]string, graph *types.Graph) []types.Edge {
	workers := workerCount()
	if len(files) < workers {
		workers = len(files)
	}
	if workers == 1 {
		return buildEdgesSerial(files, pkgNames, graph)
	}

	jobs := make(chan types.FileEntry, len(files))
	results := make(chan edgeResult, len(files))

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for file := range jobs {
				edges, err := extractEdges(file, pkgNames, graph)
				results <- edgeResult{
					filePath: file.Path,
					edges:    edges,
					err:      err,
				}
			}
		}()
	}

	go func() {
		for _, file := range files {
			jobs <- file
		}
		close(jobs)
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	var allEdges []types.Edge
	for r := range results {
		if r.err == nil {
			allEdges = append(allEdges, r.edges...)
		}
	}
	return allEdges
}

// buildEdgesSerial is the single-threaded fallback.
func buildEdgesSerial(files []types.FileEntry, pkgNames map[string]string, graph *types.Graph) []types.Edge {
	var allEdges []types.Edge
	for _, file := range files {
		edges, err := extractEdges(file, pkgNames, graph)
		if err == nil {
			allEdges = append(allEdges, edges...)
		}
	}
	return allEdges
}
