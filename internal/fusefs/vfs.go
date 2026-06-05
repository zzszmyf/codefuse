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
	idx *index.Index
}

// NewVFSRoot creates a new root node for the FUSE filesystem.
func NewVFSRoot(idx *index.Index) *VFSRoot {
	return &VFSRoot{idx: idx}
}

// Ensure VFSRoot implements the required interfaces.
var _ fs.NodeReaddirer = (*VFSRoot)(nil)
var _ fs.NodeLookuper = (*VFSRoot)(nil)

func (r *VFSRoot) OnAdd(ctx context.Context) {
	// Pre-populate fixed children
	ch := r.NewPersistentInode(ctx, &SymbolsDir{idx: r.idx}, fs.StableAttr{Mode: fuse.S_IFDIR, Ino: 1})
	r.AddChild("symbols", ch, true)

	ch2 := r.NewPersistentInode(ctx, &OutlineDir{idx: r.idx}, fs.StableAttr{Mode: fuse.S_IFDIR, Ino: 2})
	r.AddChild("outline", ch2, true)
}

func (r *VFSRoot) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	entries := []fuse.DirEntry{
		{Name: "symbols", Mode: fuse.S_IFDIR},
		{Name: "outline", Mode: fuse.S_IFDIR},
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
	idx *index.Index
}

var _ fs.NodeReaddirer = (*SymbolsDir)(nil)
var _ fs.NodeLookuper = (*SymbolsDir)(nil)

func (s *SymbolsDir) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	symbolMap := buildSymbolNameMap(s.idx.Symbols)
	entries := make([]fuse.DirEntry, 0, len(symbolMap))
	for name := range symbolMap {
		entries = append(entries, fuse.DirEntry{Name: sanitizeName(name), Mode: fuse.S_IFREG})
	}
	return fs.NewListDirStream(entries), fs.OK
}

func (s *SymbolsDir) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	// Find the original symbol name from sanitized name
	var foundName string
	symbolMap := buildSymbolNameMap(s.idx.Symbols)
	for symName := range symbolMap {
		if sanitizeName(symName) == name {
			foundName = symName
			break
		}
	}
	if foundName == "" {
		return nil, syscall.ENOENT
	}

	syms := symbolMap[foundName]
	content := formatSymbolContent(foundName, syms)
	node := &FileNode{content: content}
	inode := s.NewPersistentInode(ctx, node, fs.StableAttr{Mode: fuse.S_IFREG})
	return inode, fs.OK
}

// =============================================================================
// Outline Directory
// =============================================================================

type OutlineDir struct {
	fs.Inode
	idx *index.Index
}

var _ fs.NodeReaddirer = (*OutlineDir)(nil)
var _ fs.NodeLookuper = (*OutlineDir)(nil)

func (o *OutlineDir) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	fileMap := buildFileOutlineMap(o.idx.Symbols)
	entries := make([]fuse.DirEntry, 0, len(fileMap))
	for filename := range fileMap {
		entries = append(entries, fuse.DirEntry{Name: sanitizeName(filename), Mode: fuse.S_IFREG})
	}
	return fs.NewListDirStream(entries), fs.OK
}

func (o *OutlineDir) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	var foundFile string
	fileMap := buildFileOutlineMap(o.idx.Symbols)
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

func buildSymbolNameMap(symbols []types.Symbol) map[string][]types.Symbol {
	m := make(map[string][]types.Symbol)
	for _, sym := range symbols {
		m[sym.Name] = append(m[sym.Name], sym)
	}
	return m
}

func buildFileOutlineMap(symbols []types.Symbol) map[string][]index.SymbolDisplay {
	m := make(map[string][]index.SymbolDisplay)
	for _, sym := range symbols {
		m[sym.File] = append(m[sym.File], index.SymbolDisplay{
			Name: sym.Name,
			Kind: sym.Kind,
			File: sym.File,
			Line: sym.Line,
		})
	}
	return m
}

func formatSymbolContent(name string, syms []types.Symbol) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Symbol: %s\n", name)
	fmt.Fprintf(&b, "Occurrences: %d\n\n", len(syms))
	for i, sym := range syms {
		fmt.Fprintf(&b, "--- Occurrence %d ---\n", i+1)
		fmt.Fprintf(&b, "Kind: %s\n", sym.Kind)
		fmt.Fprintf(&b, "File: %s:%d\n", sym.File, sym.Line)
		if sym.Parent != "" {
			fmt.Fprintf(&b, "Parent: %s\n", sym.Parent)
		}
		if sym.Signature != "" {
			fmt.Fprintf(&b, "Signature: %s\n", sym.Signature)
		}
		fmt.Fprintln(&b)
	}
	return b.String()
}

func formatOutlineContent(filename string, syms []index.SymbolDisplay) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Outline: %s\n\n", filename)
	for _, sym := range syms {
		fmt.Fprintf(&b, "  L%03d  %-12s  %s\n", sym.Line, sym.Kind, sym.Name)
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
