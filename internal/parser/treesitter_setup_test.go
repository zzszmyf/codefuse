package parser

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectMissingGrammars_KnownLanguages(t *testing.T) {
	// Create a fake tree-sitter config with one parser directory
	tmpDir := t.TempDir()
	setupFakeConfig(t, tmpDir, []string{
		filepath.Join(tmpDir, "grammars"),
	})

	// Mark "go" as installed by creating the directory
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "grammars", "tree-sitter-go"), 0755))

	missing, err := DetectMissingGrammars([]string{"go", "python", "rust"})
	require.NoError(t, err)

	names := make([]string, len(missing))
	for i, g := range missing {
		names[i] = g.Lang
	}
	assert.ElementsMatch(t, []string{"python", "rust"}, names)
}

func TestDetectMissingGrammars_UnknownLanguage(t *testing.T) {
	tmpDir := t.TempDir()
	setupFakeConfig(t, tmpDir, []string{})

	missing, err := DetectMissingGrammars([]string{"unknown-lang"})
	require.NoError(t, err)
	assert.Empty(t, missing)
}

func TestLoadSaveTreeSitterConfig(t *testing.T) {
	tmpDir := t.TempDir()

	// Override home dir for test
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	config := &TreeSitterConfig{
		ParserDirectories: []string{"/path/to/grammars", "/another/path"},
	}
	err := SaveTreeSitterConfig(config)
	require.NoError(t, err)

	loaded, err := LoadTreeSitterConfig()
	require.NoError(t, err)
	assert.Equal(t, config.ParserDirectories, loaded.ParserDirectories)
}

func TestAddParserDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	config := &TreeSitterConfig{
		ParserDirectories: []string{"/existing"},
	}
	require.NoError(t, SaveTreeSitterConfig(config))

	// Add new directory
	err := AddParserDirectory("/new/dir")
	require.NoError(t, err)

	loaded, err := LoadTreeSitterConfig()
	require.NoError(t, err)
	assert.Equal(t, []string{"/existing", "/new/dir"}, loaded.ParserDirectories)

	// Adding again should not duplicate
	err = AddParserDirectory("/new/dir")
	require.NoError(t, err)

	loaded, err = LoadTreeSitterConfig()
	require.NoError(t, err)
	assert.Len(t, loaded.ParserDirectories, 2)
}

// setupFakeConfig creates a fake tree-sitter config.json in the temp home.
func setupFakeConfig(t *testing.T, tmpDir string, dirs []string) {
	t.Helper()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	configDir := filepath.Join(tmpDir, ".config", "tree-sitter")
	require.NoError(t, os.MkdirAll(configDir, 0755))
	data := []byte(`{"parser-directories":[]}`)
	if len(dirs) > 0 {
		cfg := TreeSitterConfig{ParserDirectories: dirs}
		var err error
		data, err = json.Marshal(cfg)
		require.NoError(t, err)
	}
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.json"), data, 0644))
}
