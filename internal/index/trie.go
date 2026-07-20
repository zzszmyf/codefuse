package index

// symbolTrie provides fast prefix lookup for symbol names.
// Each node maps runes to child nodes; leaf paths store node IDs.
type symbolTrie struct {
	root *trieNode
	size int
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
	t.size++
}

// RemoveID removes a specific id from the trie node for the given name.
// If the node has no more ids and no children after removal, it's kept
// (pruning empty paths is possible but rarely worth the complexity).
func (t *symbolTrie) RemoveID(name, id string) {
	node := t.root
	for _, r := range name {
		child, ok := node.children[r]
		if !ok {
			return // name not in trie
		}
		node = child
	}
	// Found the node; filter out the id.
	filtered := node.ids[:0]
	for _, existing := range node.ids {
		if existing != id {
			filtered = append(filtered, existing)
		}
	}
	if len(filtered) < len(node.ids) {
		t.size--
	}
	node.ids = filtered
}

// Delete removes ALL ids for a given name from the trie.
func (t *symbolTrie) Delete(name string) {
	node := t.root
	for _, r := range name {
		child, ok := node.children[r]
		if !ok {
			return
		}
		node = child
	}
	t.size -= len(node.ids)
	node.ids = nil
}

// HasPrefix returns true if any name in the trie starts with the given prefix.
func (t *symbolTrie) HasPrefix(prefix string) bool {
	node := t.root
	for _, r := range prefix {
		child, ok := node.children[r]
		if !ok {
			return false
		}
		node = child
	}
	return len(node.ids) > 0 || len(node.children) > 0
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

// Size returns the number of entries in the trie.
func (t *symbolTrie) Size() int {
	return t.size
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
