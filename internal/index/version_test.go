package index

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yifanmeng/codefuse/pkg/types"
)

// TestManifest_VersionRoundTrip verifies manifest saves/loads with version.
func TestManifest_VersionRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()

	m := &Manifest{
		Version: types.IndexVersion,
		Files:   map[string]int64{"main.go": 1234567890},
	}
	err := saveManifest(tmpDir, m)
	require.NoError(t, err)

	loaded, err := loadManifest(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, types.IndexVersion, loaded.Version)
	assert.Equal(t, int64(1234567890), loaded.Files["main.go"])
}

// TestManifest_BackwardCompat_LoadsV1Manifest verifies v0.1 manifest
// (without version field) loads correctly with empty version.
func TestManifest_BackwardCompat_LoadsV1Manifest(t *testing.T) {
	tmpDir := t.TempDir()
	// Write v0.1-style manifest (no version field)
	require.NoError(t, os.WriteFile(
		filepath.Join(tmpDir, "manifest.json"),
		[]byte(`{"files":{"main.go":1234567890}}`),
		0644,
	))

	loaded, err := loadManifest(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, "", loaded.Version)
	assert.Equal(t, int64(1234567890), loaded.Files["main.go"])
}

// TestBuildGraph_SavesManifestWithVersion verifies BuildGraph writes versioned manifest.
func TestBuildGraph_SavesManifestWithVersion(t *testing.T) {
	tmpDir := t.TempDir()
	goFile := filepath.Join(tmpDir, "main.go")
	require.NoError(t, os.WriteFile(goFile, []byte("package main\nfunc main(){}"), 0644))

	files := []types.FileEntry{{
		Path:     "main.go",
		AbsPath:  goFile,
		Language: types.LangGo,
		Mtime:    1234567890,
	}}

	_, err := BuildGraph(tmpDir, files, false)
	require.NoError(t, err)

	manifest, err := loadManifest(filepath.Join(tmpDir, ".codefuse"))
	require.NoError(t, err)
	assert.Equal(t, types.IndexVersion, manifest.Version)
}

// TestLoadGraph_VersionMismatch_ReturnsError verifies LoadGraph rejects
// manifest with incompatible version.
func TestLoadGraph_VersionMismatch_ReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	idxDir := filepath.Join(tmpDir, ".codefuse")
	require.NoError(t, os.MkdirAll(idxDir, 0755))

	// Write a v2 graph
	graph := NewGraph(tmpDir)
	graph.Nodes = []types.Node{{ID: "main.main", Name: "main", Kind: types.KindFunction, File: "main.go"}}
	require.NoError(t, graph.Save(idxDir))

	// Write a future-version manifest (v99)
	require.NoError(t, saveManifest(idxDir, &Manifest{Version: "99", Files: map[string]int64{}}))

	_, err := LoadGraph(idxDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "version")
}

// TestLoad_VersionField_AcceptsV1 rejects v2 index.json.
func TestLoad_VersionField_RejectsV2(t *testing.T) {
	tmpDir := t.TempDir()
	idxDir := filepath.Join(tmpDir, ".codefuse")
	require.NoError(t, os.MkdirAll(idxDir, 0755))

	// Write a v2-style index.json (has Version field = "2")
	require.NoError(t, os.WriteFile(
		filepath.Join(idxDir, "index.json"),
		[]byte(`{"version":"2","project_path":"/tmp","files":[],"symbols":[]}`),
		0644,
	))

	_, err := Load(idxDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "re-index")
}

// TestLoad_NoVersionField_AcceptsV1 verifies v0.1 index (no version) still loads.
func TestLoad_NoVersionField_AcceptsV1(t *testing.T) {
	tmpDir := t.TempDir()
	idxDir := filepath.Join(tmpDir, ".codefuse")
	require.NoError(t, os.MkdirAll(idxDir, 0755))

	// Write a v0.1-style index (no version field)
	require.NoError(t, os.WriteFile(
		filepath.Join(idxDir, "index.json"),
		[]byte(`{"project_path":"/tmp","files":[],"symbols":[]}`),
		0644,
	))

	idx, err := Load(idxDir)
	require.NoError(t, err)
	assert.NotNil(t, idx)
}

// TestLoad_VersionField_V1AcceptsV1 verifies v1 index with version "1" loads fine.
func TestLoad_VersionField_V1AcceptsV1(t *testing.T) {
	tmpDir := t.TempDir()
	idxDir := filepath.Join(tmpDir, ".codefuse")
	require.NoError(t, os.MkdirAll(idxDir, 0755))

	require.NoError(t, os.WriteFile(
		filepath.Join(idxDir, "index.json"),
		[]byte(`{"version":"1","project_path":"/tmp","files":[],"symbols":[]}`),
		0644,
	))

	idx, err := Load(idxDir)
	require.NoError(t, err)
	assert.NotNil(t, idx)
}

// TestBuild_SavesManifestWithVersion verifies old Build() also writes versioned manifest.
func TestBuild_SavesManifestWithVersion(t *testing.T) {
	tmpDir := t.TempDir()
	goFile := filepath.Join(tmpDir, "main.go")
	require.NoError(t, os.WriteFile(goFile, []byte("package main\nfunc main(){}"), 0644))

	files := []types.FileEntry{{
		Path:     "main.go",
		AbsPath:  goFile,
		Language: types.LangGo,
		Mtime:    1234567890,
	}}

	_, err := Build(tmpDir, files, false)
	require.NoError(t, err)

	manifest, err := loadManifest(filepath.Join(tmpDir, ".codefuse"))
	require.NoError(t, err)
	assert.Equal(t, "1", manifest.Version)
}
