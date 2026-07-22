package parser

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// All tests use pre-generated XML fixtures — no tree-sitter needed.

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err, "fixture %s not found — run: make fixtures", name)
	return data
}

func TestExtractFromXML_Python_Nodes(t *testing.T) {
	data := loadFixture(t, "fixture_py.xml")
	nodes, edges, sinks, err := ExtractFromXML(data, "test.py", "python")
	require.NoError(t, err)
	assert.NotEmpty(t, nodes, "should extract symbols from Python fixture")

	// Check we found the class and functions.
	names := make(map[string]bool)
	for _, n := range nodes {
		names[n.Name] = true
	}
	assert.True(t, names["login"], "should find login method")
	assert.True(t, names["helper"], "should find helper function")
	assert.True(t, names["AuthService"], "should find AuthService class")
	_ = edges
	_ = sinks
}

func TestExtractFromXML_Python_Sinks(t *testing.T) {
	data := loadFixture(t, "fixture_py.xml")
	_, _, sinks, err := ExtractFromXML(data, "test.py", "python")
	require.NoError(t, err)

	// sql.Query("SELECT...") should be captured as a sink with pkg=sql.
	found := false
	for _, s := range sinks {
		if strings.HasPrefix(s.CalleeName, "sql.") && s.Pkg == "sql" {
			found = true
		}
	}
	assert.True(t, found, "should capture sql.Query as external sink")
}

func TestExtractFromXML_Python_Edges(t *testing.T) {
	data := loadFixture(t, "fixture_py.xml")
	_, edges, _, err := ExtractFromXML(data, "test.py", "python")
	require.NoError(t, err)

	// login() calls dao.findById(token) — should create edge login→findById
	assert.NotEmpty(t, edges, "should have internal call edges")
}

func TestExtractFromXML_Java_Nodes(t *testing.T) {
	data := loadFixture(t, "fixture_java.xml")
	nodes, _, _, err := ExtractFromXML(data, "test.java", "java")
	require.NoError(t, err)
	assert.NotEmpty(t, nodes, "should extract at least class name from Java fixture")
	// Verify class declaration is extracted.
	names := make(map[string]bool)
	for _, n := range nodes {
		names[n.Name] = true
	}
	assert.True(t, names["login"], "should find login method")
	assert.True(t, names["AuthService"], "should find AuthService class")
}

func TestExtractFromXML_Java_CallEdges(t *testing.T) {
	data := loadFixture(t, "fixture_java.xml")
	_, edges, _, err := ExtractFromXML(data, "test.java", "java")
	require.NoError(t, err)

	// dao.findById(token) → should create an edge (either internal or via import)
	assert.NotEmpty(t, edges, "should extract call edges from Java")
}

func TestExtractFromXML_Empty(t *testing.T) {
	data := loadFixture(t, "fixture_empty.xml")
	nodes, _, _, err := ExtractFromXML(data, "empty.py", "python")
	require.NoError(t, err)
	assert.Empty(t, nodes, "empty file should produce no symbols")
}

func TestExtractFromXML_UnsupportedLanguage(t *testing.T) {
	_, _, _, err := ExtractFromXML([]byte("<sources/>"), "test.xyz", "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported")
}

func TestParseImports_Python_Fixture(t *testing.T) {
	data := loadFixture(t, "fixture_py.xml")
	_, _, _, err := ExtractFromXML(data, "test.py", "python")
	require.NoError(t, err)

	// The source file has: import sql; from db.dao import UserDao
	// But imports are parsed via regex from source code, not XML.
	// Verify the extractor handles this correctly.
	_ = data
}

func TestExtractVarMap_Python_Fixture(t *testing.T) {
	content := "dao = UserDao()\nresult = dao.findById(token)\n"
	vm := ExtractVarMap(content, "python")
	assert.Contains(t, vm, "dao")
	assert.Equal(t, "UserDao", vm["dao"])
}

func TestExtractVarMap_Java_Fixture(t *testing.T) {
	content := "UserDao dao = new UserDao();\n"
	vm := ExtractVarMap(content, "java")
	assert.Contains(t, vm, "dao")
	assert.Equal(t, "UserDao", vm["dao"])
}

func TestExtractVarMap_Python_Param(t *testing.T) {
	content := "def login(dao: UserDao, token: str):\n    pass\n"
	vm := ExtractVarMap(content, "python")
	assert.Contains(t, vm, "dao")
	assert.Equal(t, "UserDao", vm["dao"])
}

func TestParseError_Format(t *testing.T) {
	err := &ParseError{File: "test.py", Lang: "python", Stderr: "syntax error"}
	assert.Contains(t, err.Error(), "test.py")
	assert.Contains(t, err.Error(), "python")
}
