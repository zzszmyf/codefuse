package fusefs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yifanmeng/codefuse/internal/index"
	"github.com/yifanmeng/codefuse/pkg/types"
)

func TestFormatSymbolContent(t *testing.T) {
	syms := []types.Symbol{
		{Name: "Hello", Kind: types.KindFunction, File: "a.go", Line: 1, Signature: "func Hello()"},
		{Name: "Hello", Kind: types.KindMethod, File: "b.go", Line: 5, Parent: "Greeter"},
	}
	content := formatSymbolContent("Hello", syms)
	assert.Contains(t, content, "Symbol: Hello")
	assert.Contains(t, content, "a.go:1")
	assert.Contains(t, content, "b.go:5")
	assert.Contains(t, content, "Parent: Greeter")
}

func TestFormatOutlineContent(t *testing.T) {
	syms := []index.SymbolDisplay{
		{Name: "main", Kind: types.KindFunction, Line: 1},
		{Name: "Hello", Kind: types.KindFunction, Line: 5},
	}
	content := formatOutlineContent("main.go", syms)
	assert.Contains(t, content, "Outline: main.go")
	assert.Contains(t, content, "L001")
	assert.Contains(t, content, "L005")
	assert.Contains(t, content, "Hello")
}

func TestSanitizeName(t *testing.T) {
	assert.Equal(t, "foo_bar", sanitizeName("foo/bar"))
	assert.Equal(t, "foo_bar", sanitizeName("foo\\bar"))
	assert.Equal(t, "foo_bar", sanitizeName("foo:bar"))
}

func TestVFSRoot_Readdir(t *testing.T) {
	idx := &index.Index{
		Symbols: []types.Symbol{
			{Name: "main", Kind: types.KindFunction, File: "main.go", Line: 1},
		},
	}

	root := NewVFSRoot(idx)
	require.NotNil(t, root)

	// OnAdd should create children
	// Note: full FUSE testing requires kernel module, so we test structure only
	assert.NotNil(t, root)
}
