package index

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yifanmeng/codefuse/pkg/types"
)

func TestFindSymbolGlob(t *testing.T) {
	idx := &Index{
		Symbols: []types.Symbol{
			{Name: "useAuth", Kind: types.KindFunction, File: "a.ts", Line: 1},
			{Name: "useDebouncedCallback", Kind: types.KindFunction, File: "b.ts", Line: 2},
			{Name: "userConfirm", Kind: types.KindVariable, File: "c.ts", Line: 3},
			{Name: "ArtifactCard", Kind: types.KindClass, File: "d.tsx", Line: 4},
			{Name: "QuickReplyCard", Kind: types.KindClass, File: "e.tsx", Line: 5},
		},
	}
	idx.buildSymbolMap()

	results := idx.FindSymbol("use*", "")
	assert.Len(t, results, 3)

	results = idx.FindSymbol("*Card", "")
	assert.Len(t, results, 2)

	results = idx.FindSymbol("use*", types.KindFunction)
	assert.Len(t, results, 2)
}
