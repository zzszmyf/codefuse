package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsGrepMode_True(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()

	os.Args = []string{"/usr/local/bin/grep", "-rn", "pattern", "."}
	assert.True(t, isGrepMode())

	os.Args = []string{"grep", "pattern"}
	assert.True(t, isGrepMode())

	os.Args = []string{"codefuse", "index", "."}
	assert.False(t, isGrepMode())
}

func TestParseGrepFlags_Basic(t *testing.T) {
	opts := parseGrepFlags([]string{"-rn", "AuthService", "."})
	assert.Equal(t, "AuthService", opts.pattern)
	assert.True(t, opts.recursive)
	assert.True(t, opts.lineNumber)
}

func TestParseGrepFlags_FilesOnly(t *testing.T) {
	opts := parseGrepFlags([]string{"-l", "pattern"})
	assert.True(t, opts.filesOnly)
}

func TestParseGrepFlags_CountOnly(t *testing.T) {
	opts := parseGrepFlags([]string{"-c", "pattern"})
	assert.True(t, opts.countOnly)
}

func TestParseGrepFlags_CaseInsensitive(t *testing.T) {
	opts := parseGrepFlags([]string{"-i", "Pattern"})
	assert.True(t, opts.ignoreCase)
}

func TestParseGrepFlags_Context(t *testing.T) {
	opts := parseGrepFlags([]string{"-A", "3", "-B", "2", "-C", "5", "pattern"})
	assert.Equal(t, 3, opts.contextAfter)
	assert.Equal(t, 2, opts.contextBefore)
	assert.Equal(t, 5, opts.contextAround)
}

func TestParseGrepFlags_TextMode(t *testing.T) {
	opts := parseGrepFlags([]string{"-t", "pattern"})
	assert.True(t, opts.forceText)

	opts = parseGrepFlags([]string{"--text", "pattern"})
	assert.True(t, opts.forceText)
}

func TestParseGrepFlags_CombinedShort(t *testing.T) {
	opts := parseGrepFlags([]string{"-rnil", "pattern"})
	assert.True(t, opts.recursive)
	assert.True(t, opts.lineNumber)
	assert.True(t, opts.ignoreCase)
	assert.True(t, opts.filesOnly)
}

func TestFindIndexDir(t *testing.T) {
	tmpDir := t.TempDir()
	indexDir := filepath.Join(tmpDir, ".codefuse")
	require.NoError(t, os.MkdirAll(indexDir, 0755))

	// Change to the temp dir.
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(tmpDir)

	found, projPath := findIndexDir()
	// Resolve symlinks (macOS /var → /private/var).
	resolvedFound, _ := filepath.EvalSymlinks(found)
	resolvedExpected, _ := filepath.EvalSymlinks(indexDir)
	resolvedProj, _ := filepath.EvalSymlinks(projPath)
	resolvedTmp, _ := filepath.EvalSymlinks(tmpDir)
	assert.Equal(t, resolvedExpected, resolvedFound)
	assert.Equal(t, resolvedTmp, resolvedProj)
}

func TestFindIndexDir_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(tmpDir)

	found, _ := findIndexDir()
	assert.Empty(t, found)
}
