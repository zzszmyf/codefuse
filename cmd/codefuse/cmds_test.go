package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Tests for cmds.go functions using mock index.

func TestRunQuery_WithMockIndex(t *testing.T) {
	_, projDir := buildMockIndex(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projDir)

	err := runQuery(".", "Authenticate", false, false)
	assert.NoError(t, err)

	err = runQuery(".", "NonExistent", false, false)
	assert.NoError(t, err)
}

func TestRunQuery_WithCallers(t *testing.T) {
	_, projDir := buildMockIndex(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projDir)

	// Query with --callers loads the full graph (including edges).
	err := runQuery(".", "Authenticate", true, false)
	assert.NoError(t, err)
}

func TestRunQuery_WithCallees(t *testing.T) {
	_, projDir := buildMockIndex(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projDir)

	err := runQuery(".", "Login", false, true)
	assert.NoError(t, err)
}

func TestRunList_WithMockIndex(t *testing.T) {
	_, projDir := buildMockIndex(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projDir)

	err := runList(".")
	assert.NoError(t, err)
}

func TestRunOutline_WithMockIndex(t *testing.T) {
	_, projDir := buildMockIndex(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projDir)

	err := runOutline(".", "auth.go")
	assert.NoError(t, err)

	err = runOutline(".", "nonexistent.go")
	assert.NoError(t, err)
}

func TestRunSinks_WithMockIndex(t *testing.T) {
	_, projDir := buildMockIndex(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projDir)

	err := runSinks(".", "", "")
	assert.NoError(t, err)

	err = runSinks(".", "Authenticate", "")
	assert.NoError(t, err)

	err = runSinks(".", "", "sql")
	assert.NoError(t, err)
}

func TestRunReachable_WithMockIndex(t *testing.T) {
	_, projDir := buildMockIndex(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(projDir)

	err := runReachable(".", "Login", "sql")
	assert.NoError(t, err)

	err = runReachable(".", "NonExistent", "sql")
	assert.NoError(t, err)
}

func TestRunQuery_NoIndex(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(tmpDir)

	err := runQuery(".", "Anything", false, false)
	assert.Error(t, err, "no index should return error")
}

func TestRunList_NoIndex(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(tmpDir)

	err := runList(".")
	assert.Error(t, err)
}

func TestSortedKeys(t *testing.T) {
	m := map[string]int{"c": 1, "a": 2, "b": 3}
	keys := sortedKeys(m)
	assert.Equal(t, []string{"a", "b", "c"}, keys)
}
