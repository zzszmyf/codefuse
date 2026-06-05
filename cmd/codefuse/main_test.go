package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "codefuse")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}

	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = filepath.Dir(getMainPath(t))
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "build failed: %s", out)
	return bin
}

func getMainPath(t *testing.T) string {
	_, f, _, _ := runtime.Caller(0)
	return f
}

func TestCLI_RootHelp(t *testing.T) {
	bin := buildBinary(t)
	cmd := exec.Command(bin, "--help")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err)

	output := string(out)
	assert.Contains(t, output, "codefuse")
	assert.Contains(t, output, "index")
	assert.Contains(t, output, "query")
	assert.Contains(t, output, "list")
	assert.Contains(t, output, "outline")
	assert.Contains(t, output, "mount")
}

func TestCLI_Version(t *testing.T) {
	bin := buildBinary(t)
	cmd := exec.Command(bin, "--version")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err)
	assert.Contains(t, string(out), "codefuse")
}

func TestCLI_IndexCommand_RequiresPath(t *testing.T) {
	bin := buildBinary(t)
	cmd := exec.Command(bin, "index")
	out, err := cmd.CombinedOutput()
	// Should fail because path is required
	require.Error(t, err)
	assert.Contains(t, string(out), "path")
}

func TestCLI_IndexCommand_CreatesIndex(t *testing.T) {
	bin := buildBinary(t)
	tmpDir := t.TempDir()

	// Create a dummy Go file
	dummyFile := filepath.Join(tmpDir, "main.go")
	err := os.WriteFile(dummyFile, []byte(`package main

func main() {}
`), 0644)
	require.NoError(t, err)

	cmd := exec.Command(bin, "index", tmpDir)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "index failed: %s", out)

	// Should report success
	output := string(out)
	assert.Contains(t, output, "Indexed")

	// Should create .codefuse index dir
	indexDir := filepath.Join(tmpDir, ".codefuse")
	_, err = os.Stat(indexDir)
	assert.NoError(t, err, "index directory should be created")
}

func TestCLI_ListCommand_ShowsFiles(t *testing.T) {
	bin := buildBinary(t)
	tmpDir := t.TempDir()

	// Create dummy files
	err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\n"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, "utils.go"), []byte("package main\n"), 0644)
	require.NoError(t, err)

	// First index
	cmd := exec.Command(bin, "index", tmpDir)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "index failed: %s", out)

	// Then list
	cmd = exec.Command(bin, "list", "--project", tmpDir)
	out, err = cmd.CombinedOutput()
	require.NoError(t, err, "list failed: %s", out)

	output := string(out)
	assert.Contains(t, output, "main.go")
	assert.Contains(t, output, "utils.go")
}

func TestCLI_QueryCommand_FindsSymbol(t *testing.T) {
	bin := buildBinary(t)
	tmpDir := t.TempDir()

	// Create a file with a function
	goFile := filepath.Join(tmpDir, "hello.go")
	err := os.WriteFile(goFile, []byte(`package main

func Hello(name string) string {
	return "Hello, " + name
}
`), 0644)
	require.NoError(t, err)

	// Index
	cmd := exec.Command(bin, "index", tmpDir)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "index failed: %s", out)

	// Query for symbol
	cmd = exec.Command(bin, "query", "Hello", "--project", tmpDir)
	out, err = cmd.CombinedOutput()
	require.NoError(t, err, "query failed: %s", out)

	output := string(out)
	assert.Contains(t, output, "Hello")
	assert.Contains(t, output, "hello.go")
}

func TestCLI_OutlineCommand_ShowsStructure(t *testing.T) {
	bin := buildBinary(t)
	tmpDir := t.TempDir()

	goFile := filepath.Join(tmpDir, "hello.go")
	err := os.WriteFile(goFile, []byte(`package main

func Hello(name string) string {
	return "Hello, " + name
}
`), 0644)
	require.NoError(t, err)

	// Index
	cmd := exec.Command(bin, "index", tmpDir)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "index failed: %s", out)

	// Outline
	cmd = exec.Command(bin, "outline", "hello.go", "--project", tmpDir)
	out, err = cmd.CombinedOutput()
	require.NoError(t, err, "outline failed: %s", out)

	output := string(out)
	assert.Contains(t, output, "package")
	assert.Contains(t, output, "Hello")
	assert.Contains(t, output, "function")
}

func TestCLI_VFSCommand_GeneratesViews(t *testing.T) {
	bin := buildBinary(t)
	tmpDir := t.TempDir()

	goFile := filepath.Join(tmpDir, "hello.go")
	err := os.WriteFile(goFile, []byte(`package main

type Greeter struct{}

func (g *Greeter) Hello() string {
	return "Hello"
}
`), 0644)
	require.NoError(t, err)

	// Index
	cmd := exec.Command(bin, "index", tmpDir)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "index failed: %s", out)

	// Generate VFS views
	cmd = exec.Command(bin, "vfs", "generate", "--project", tmpDir)
	out, err = cmd.CombinedOutput()
	require.NoError(t, err, "vfs generate failed: %s", out)

	// Check symbol view
	symbolDir := filepath.Join(tmpDir, ".codefuse", "vfs", "symbols")
	_, err = os.Stat(symbolDir)
	assert.NoError(t, err, "symbol view dir should exist")

	// Check that Greeter symbol exists
	greeterFile := filepath.Join(symbolDir, "Greeter")
	content, err := os.ReadFile(greeterFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "Greeter")

	// Check outline view
	outlineDir := filepath.Join(tmpDir, ".codefuse", "vfs", "outline")
	_, err = os.Stat(outlineDir)
	assert.NoError(t, err, "outline view dir should exist")
}
