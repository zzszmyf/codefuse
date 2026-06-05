package index

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/yifanmeng/codefuse/internal/parser"
	"github.com/yifanmeng/codefuse/pkg/types"
)

// Manifest tracks file modification times for incremental indexing
type Manifest struct {
	Files map[string]int64 `json:"files"` // path -> mtime
}

// BuildIncremental performs incremental indexing, only re-parsing changed files.
// Returns the new index and the count of files that were re-parsed.
func BuildIncremental(projectPath string, files []types.FileEntry, useTreeSitter bool) (*Index, int, error) {
	indexDir := filepath.Join(projectPath, ".codefuse")

	// Try to load existing index
	existingIdx, _ := Load(indexDir)
	if existingIdx == nil {
		// No existing index, do full build
		idx, err := Build(projectPath, files, useTreeSitter)
		return idx, len(files), err
	}

	// Load manifest
	manifest, _ := loadManifest(indexDir)
	if manifest == nil {
		manifest = &Manifest{Files: make(map[string]int64)}
	}

	// Determine which files changed
	changedFiles := make([]types.FileEntry, 0)
	unchangedPaths := make(map[string]bool)
	newPaths := make(map[string]bool)

	for _, f := range files {
		newPaths[f.Path] = true
		oldMtime, existed := manifest.Files[f.Path]
		if !existed || f.Mtime != oldMtime {
			changedFiles = append(changedFiles, f)
		} else {
			unchangedPaths[f.Path] = true
		}
	}

	// Find deleted files
	deletedPaths := make(map[string]bool)
	for path := range manifest.Files {
		if !newPaths[path] {
			deletedPaths[path] = true
		}
	}

	// Build new index
	newIdx := &Index{
		ProjectPath: projectPath,
		Files:       files,
		Symbols:     make([]types.Symbol, 0),
	}

	// Retain symbols from unchanged files
	for _, sym := range existingIdx.Symbols {
		if unchangedPaths[sym.File] {
			newIdx.Symbols = append(newIdx.Symbols, sym)
		}
	}

	// Parse changed files
	var remaining []types.FileEntry
	if useTreeSitter && len(changedFiles) > 0 {
		tsResults, failed := parser.BatchExtractWithTreeSitter(changedFiles)
		for _, syms := range tsResults {
			newIdx.Symbols = append(newIdx.Symbols, syms...)
		}
		remaining = failed
	} else {
		remaining = changedFiles
	}

	for _, file := range remaining {
		syms, err := extractSymbols(file)
		if err != nil {
			continue
		}
		newIdx.Symbols = append(newIdx.Symbols, syms...)
	}

	// Update manifest
	newManifest := &Manifest{Files: make(map[string]int64)}
	for _, f := range files {
		newManifest.Files[f.Path] = f.Mtime
	}

	// Save everything
	if err := newIdx.Save(indexDir); err != nil {
		return nil, 0, err
	}
	if err := saveManifest(indexDir, newManifest); err != nil {
		return nil, 0, err
	}

	newIdx.buildSymbolMap()
	return newIdx, len(changedFiles), nil
}

func loadManifest(indexDir string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(indexDir, "manifest.json"))
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func saveManifest(indexDir string, m *Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(indexDir, "manifest.json"), data, 0644)
}
