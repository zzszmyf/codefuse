package index

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSymbolTrie_InsertAndSearch(t *testing.T) {
	trie := newSymbolTrie()
	trie.Insert("Hello", "id1")
	trie.Insert("World", "id2")
	trie.Insert("HelloWorld", "id3")

	// Exact match
	ids := trie.FindPrefix("Hello")
	assert.ElementsMatch(t, []string{"id1", "id3"}, ids)

	// Another prefix
	ids = trie.FindPrefix("World")
	assert.ElementsMatch(t, []string{"id2"}, ids)

	// No match
	ids = trie.FindPrefix("Foo")
	assert.Empty(t, ids)

	// Empty prefix matches all
	ids = trie.FindPrefix("")
	assert.Len(t, ids, 3)
}

func TestSymbolTrie_CaseSensitive(t *testing.T) {
	trie := newSymbolTrie()
	trie.Insert("foo", "id1")
	trie.Insert("Foo", "id2")

	ids := trie.FindPrefix("foo")
	assert.ElementsMatch(t, []string{"id1"}, ids)

	ids = trie.FindPrefix("Foo")
	assert.ElementsMatch(t, []string{"id2"}, ids)
}

func TestSymbolTrie_MultipleIDsPerName(t *testing.T) {
	trie := newSymbolTrie()
	trie.Insert("main", "id1")
	trie.Insert("main", "id2")

	ids := trie.FindPrefix("main")
	assert.Len(t, ids, 2)
}

func BenchmarkTrieInsert(b *testing.B) {
	names := []string{
		"Authenticate", "Auth", "Authorize", "Add", "Append",
		"Build", "BuildIndex", "BuildGraph", "BuildTrie",
		"Create", "CreateUser", "CreateSession",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		t := newSymbolTrie()
		for _, name := range names {
			t.Insert(name, "id")
		}
	}
}

func BenchmarkTrieSearch(b *testing.B) {
	t := newSymbolTrie()
	for _, name := range []string{"Authenticate", "Auth", "Authorize", "Add", "Append"} {
		t.Insert(name, "id")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		t.FindPrefix("Auth")
	}
}
