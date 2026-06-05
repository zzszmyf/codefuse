package fusefs

import (
	"context"
	"fmt"
	"strings"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/yifanmeng/codefuse/internal/index"
	"github.com/yifanmeng/codefuse/pkg/types"
)

// =============================================================================
// VFS Root
// =============================================================================

// VFSRoot implements the root directory of the codefuse FUSE mount.
type VFSRoot struct {
	fs.Inode
	graph *index.Graph
}

// NewVFSRoot creates a new root node for the FUSE filesystem.
func NewVFSRoot(graph *index.Graph) *VFSRoot {
	return &VFSRoot{graph: graph}
}

// Ensure VFSRoot implements the required interfaces.
var _ fs.NodeReaddirer = (*VFSRoot)(nil)
var _ fs.NodeLookuper = (*VFSRoot)(nil)

func (r *VFSRoot) OnAdd(ctx context.Context) {
	// Pre-populate fixed children
	ch := r.NewPersistentInode(ctx, &SymbolsDir{graph: r.graph}, fs.StableAttr{Mode: fuse.S_IFDIR, Ino: 1})
	r.AddChild("symbols", ch, true)

	ch2 := r.NewPersistentInode(ctx, &OutlineDir{graph: r.graph}, fs.StableAttr{Mode: fuse.S_IFDIR, Ino: 2})
	r.AddChild("outline", ch2, true)

	ch3 := r.NewPersistentInode(ctx, &ReferencesDir{graph: r.graph}, fs.StableAttr{Mode: fuse.S_IFDIR, Ino: 3})
	r.AddChild("references", ch3, true)
}

func (r *VFSRoot) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	entries := []fuse.DirEntry{
		{Name: "symbols", Mode: fuse.S_IFDIR},
		{Name: "outline", Mode: fuse.S_IFDIR},
		{Name: "references", Mode: fuse.S_IFDIR},
	}
	return fs.NewListDirStream(entries), fs.OK
}

func (r *VFSRoot) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	// Children are already added in OnAdd; this shouldn't normally be called.
	return nil, syscall.ENOENT
}

// =============================================================================
// Symbols Directory
// =============================================================================

type SymbolsDir struct {
	fs.Inode
	graph *index.Graph
}

var _ fs.NodeReaddirer = (*SymbolsDir)(nil)
var _ fs.NodeLookuper = (*SymbolsDir)(nil)

func (s *SymbolsDir) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	symbolMap := buildSymbolNameMap(s.graph.Nodes)
	entries := make([]fuse.DirEntry, 0, len(symbolMap))
	for name := range symbolMap {
		entries = append(entries, fuse.DirEntry{Name: sanitizeName(name), Mode: fuse.S_IFREG})
	}
	return fs.NewListDirStream(entries), fs.OK
}

func (s *SymbolsDir) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	var foundName string
	symbolMap := buildSymbolNameMap(s.graph.Nodes)
	for symName := range symbolMap {
		if sanitizeName(symName) == name {
			foundName = symName
			break
		}
	}
	if foundName == "" {
		return nil, syscall.ENOENT
	}

	nodes := symbolMap[foundName]
	content := formatSymbolContent(foundName, nodes)
	node := &FileNode{content: content}
	inode := s.NewPersistentInode(ctx, node, fs.StableAttr{Mode: fuse.S_IFREG})
	return inode, fs.OK
}

// =============================================================================
// Outline Directory
// =============================================================================

type OutlineDir struct {
	fs.Inode
	graph *index.Graph
}

var _ fs.NodeReaddirer = (*OutlineDir)(nil)
var _ fs.NodeLookuper = (*OutlineDir)(nil)

func (o *OutlineDir) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	fileMap := buildFileOutlineMap(o.graph.Nodes)
	entries := make([]fuse.DirEntry, 0, len(fileMap))
	for filename := range fileMap {
		entries = append(entries, fuse.DirEntry{Name: sanitizeName(filename), Mode: fuse.S_IFREG})
	}
	return fs.NewListDirStream(entries), fs.OK
}

func (o *OutlineDir) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	var foundFile string
	fileMap := buildFileOutlineMap(o.graph.Nodes)
	for filename := range fileMap {
		if sanitizeName(filename) == name {
			foundFile = filename
			break
		}
	}
	if foundFile == "" {
		return nil, syscall.ENOENT
	}

	content := formatOutlineContent(foundFile, fileMap[foundFile])
	node := &FileNode{content: content}
	inode := o.NewPersistentInode(ctx, node, fs.StableAttr{Mode: fuse.S_IFREG})
	return inode, fs.OK
}

// =============================================================================
// References Directory
// =============================================================================

type ReferencesDir struct {
	fs.Inode
	graph *index.Graph
}

var _ fs.NodeReaddirer = (*ReferencesDir)(nil)
var _ fs.NodeLookuper = (*ReferencesDir)(nil)

func (r *ReferencesDir) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	symbolMap := buildSymbolNameMap(r.graph.Nodes)
	entries := make([]fuse.DirEntry, 0, len(symbolMap))
	for name := range symbolMap {
		entries = append(entries, fuse.DirEntry{Name: sanitizeName(name), Mode: fuse.S_IFREG})
	}
	return fs.NewListDirStream(entries), fs.OK
}

func (r *ReferencesDir) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	var foundName string
	symbolMap := buildSymbolNameMap(r.graph.Nodes)
	for symName := range symbolMap {
		if sanitizeName(symName) == name {
			foundName = symName
			break
		}
	}
	if foundName == "" {
		return nil, syscall.ENOENT
	}

	nodes := symbolMap[foundName]
	content := formatReferencesContent(foundName, nodes, r.graph)
	node := &FileNode{content: content}
	inode := r.NewPersistentInode(ctx, node, fs.StableAttr{Mode: fuse.S_IFREG})
	return inode, fs.OK
}

// =============================================================================
// File Node (content is generated on-demand)
// =============================================================================

type FileNode struct {
	fs.Inode
	content string
}

var _ fs.NodeOpener = (*FileNode)(nil)
var _ fs.NodeReader = (*FileNode)(nil)
var _ fs.NodeGetattrer = (*FileNode)(nil)

func (f *FileNode) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	return nil, fuse.FOPEN_KEEP_CACHE, fs.OK
}

func (f *FileNode) Read(ctx context.Context, fh fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	content := f.content
	if off >= int64(len(content)) {
		return fuse.ReadResultData(nil), fs.OK
	}
	end := off + int64(len(dest))
	if end > int64(len(content)) {
		end = int64(len(content))
	}
	return fuse.ReadResultData([]byte(content)[off:end]), fs.OK
}

func (f *FileNode) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = 0444
	out.Size = uint64(len(f.content))
	return fs.OK
}

// =============================================================================
// Helpers
// =============================================================================

func buildSymbolNameMap(nodes []types.Node) map[string][]types.Node {
	m := make(map[string][]types.Node)
	for _, node := range nodes {
		m[node.Name] = append(m[node.Name], node)
	}
	return m
}

func buildFileOutlineMap(nodes []types.Node) map[string][]types.Node {
	m := make(map[string][]types.Node)
	for _, node := range nodes {
		m[node.File] = append(m[node.File], node)
	}
	return m
}

func formatSymbolContent(name string, nodes []types.Node) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Symbol: %s\n", name)
	fmt.Fprintf(&b, "Occurrences: %d\n\n", len(nodes))
	for i, node := range nodes {
		fmt.Fprintf(&b, "--- Occurrence %d ---\n", i+1)
		fmt.Fprintf(&b, "Kind: %s\n", node.Kind)
		fmt.Fprintf(&b, "File: %s:%d\n", node.File, node.Line)
		fmt.Fprintf(&b, "ID: %s\n", node.ID)
		if node.Parent != "" {
			fmt.Fprintf(&b, "Parent: %s\n", node.Parent)
		}
		if node.Signature != "" {
			fmt.Fprintf(&b, "Signature: %s\n", node.Signature)
		}
		fmt.Fprintln(&b)
	}
	return b.String()
}

func formatOutlineContent(filename string, nodes []types.Node) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Outline: %s\n\n", filename)
	for _, node := range nodes {
		fmt.Fprintf(&b, "  L%03d  %-12s  %s\n", node.Line, node.Kind, node.Name)
	}
	return b.String()
}

func formatReferencesContent(name string, nodes []types.Node, graph *index.Graph) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# References: %s\n\n", name)

	for _, node := range nodes {
		fmt.Fprintf(&b, "## %s (%s) @ %s:%d\n\n", node.Name, node.Kind, node.File, node.Line)

		callers := graph.FindCallers(node.ID)
		if len(callers) > 0 {
			fmt.Fprintf(&b, "### Callers (%d)\n", len(callers))
			for _, edge := range callers {
				caller := graph.FindNodeByID(edge.From)
				if caller != nil {
					fmt.Fprintf(&b, "  → %s (%s) @ %s:%d\n", caller.Name, caller.Kind, edge.File, edge.Line)
				} else {
					fmt.Fprintf(&b, "  → %s @ %s:%d\n", edge.From, edge.File, edge.Line)
				}
			}
			fmt.Fprintln(&b)
		} else {
			fmt.Fprintf(&b, "### Callers\n  No callers found.\n\n")
		}

		callees := graph.FindCallees(node.ID)
		if len(callees) > 0 {
			fmt.Fprintf(&b, "### Callees (%d)\n", len(callees))
			for _, edge := range callees {
				callee := graph.FindNodeByID(edge.To)
				if callee != nil {
					fmt.Fprintf(&b, "  → %s (%s) @ %s:%d\n", callee.Name, callee.Kind, edge.File, edge.Line)
				} else {
					fmt.Fprintf(&b, "  → %s @ %s:%d\n", edge.To, edge.File, edge.Line)
				}
			}
			fmt.Fprintln(&b)
		} else {
			fmt.Fprintf(&b, "### Callees\n  No callees found.\n\n")
		}
	}

	return b.String()
}

// sanitizeName replaces path separator characters with underscores.
func sanitizeName(name string) string {
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, ":", "_")
	return name
}
