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
	// -C overwrites -A and -B (real grep behavior: last flag wins).
	opts := parseGrepFlags([]string{"-A", "3", "-B", "2", "-C", "5", "pattern"})
	assert.Equal(t, 5, opts.contextAfter)  // -C overwrites -A
	assert.Equal(t, 5, opts.contextBefore) // -C overwrites -B
	assert.Equal(t, 5, opts.contextAround)
}

func TestParseGrepFlags_CombinedNum(t *testing.T) {
	opts := parseGrepFlags([]string{"-A3", "-B2", "-C5", "pattern"})
	assert.Equal(t, 5, opts.contextAfter)
	assert.Equal(t, 5, opts.contextBefore)
	assert.Equal(t, 5, opts.contextAround)
}

func TestParseGrepFlags_MaxCount(t *testing.T) {
	opts := parseGrepFlags([]string{"-m", "10", "pattern"})
	assert.Equal(t, 10, opts.maxCount)

	opts = parseGrepFlags([]string{"-m5", "pattern"})
	assert.Equal(t, 5, opts.maxCount)
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

func TestParseGrepFlags_InvertQuiet(t *testing.T) {
	opts := parseGrepFlags([]string{"-v", "-q", "pattern"})
	assert.True(t, opts.invertMatch)
	assert.True(t, opts.quiet)
}

func TestParseGrepFlags_OnlyMatching(t *testing.T) {
	opts := parseGrepFlags([]string{"-o", "pattern"})
	assert.True(t, opts.onlyMatching)
}

func TestParseGrepFlags_IncludeExclude(t *testing.T) {
	opts := parseGrepFlags([]string{"--include=*.py", "--exclude=test_*", "pattern"})
	assert.Contains(t, opts.include, "*.py")
	assert.Contains(t, opts.exclude, "test_*")
}

func TestParseGrepFlags_MultiplePatterns(t *testing.T) {
	opts := parseGrepFlags([]string{"-e", "pattern1", "-e", "pattern2"})
	// Second -e overwrites pattern (grep behavior)
	assert.Equal(t, "pattern2", opts.pattern)
}

func TestParseGrepFlags_PatternAndPath(t *testing.T) {
	opts := parseGrepFlags([]string{"pattern", "src/", "tests/"})
	assert.Equal(t, "pattern", opts.pattern)
	assert.Equal(t, []string{"src/", "tests/"}, opts.paths)
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
