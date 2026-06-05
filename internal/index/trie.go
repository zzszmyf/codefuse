package index

// symbolTrie provides fast prefix lookup for symbol names.
// Each node maps runes to child nodes; leaf paths store node IDs.
type symbolTrie struct {
	root *trieNode
}

type trieNode struct {
	children map[rune]*trieNode
	ids      []string
}

func newSymbolTrie() *symbolTrie {
	return &symbolTrie{
		root: &trieNode{children: make(map[rune]*trieNode)},
	}
}

// Insert adds a name->id mapping into the trie.
func (t *symbolTrie) Insert(name, id string) {
	node := t.root
	for _, r := range name {
		child, ok := node.children[r]
		if !ok {
			child = &trieNode{children: make(map[rune]*trieNode)}
			node.children[r] = child
		}
		node = child
	}
	node.ids = append(node.ids, id)
}

// FindPrefix returns all node IDs whose names start with the given prefix.
func (t *symbolTrie) FindPrefix(prefix string) []string {
	node := t.root
	for _, r := range prefix {
		child, ok := node.children[r]
		if !ok {
			return nil
		}
		node = child
	}
	return collectIDs(node)
}

// collectIDs returns all IDs in the subtree rooted at node.
func collectIDs(node *trieNode) []string {
	var result []string
	var walk func(n *trieNode)
	walk = func(n *trieNode) {
		result = append(result, n.ids...)
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(node)
	return result
}
