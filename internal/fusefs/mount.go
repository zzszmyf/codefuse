package fusefs

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/yifanmeng/codefuse/internal/index"
)

// Mount mounts the codefuse VFS at the given mountpoint.
// Blocks until unmounted.
func Mount(idx *index.Index, mountpoint string) error {
	if err := os.MkdirAll(mountpoint, 0755); err != nil {
		return fmt.Errorf("cannot create mountpoint: %w", err)
	}

	opts := &fs.Options{
		MountOptions: fuse.MountOptions{
			Name:          "codefuse",
			FsName:        "codefuse",
			Debug:         os.Getenv("CODEFUSE_DEBUG") != "",
			AllowOther:    false,
			DisableXAttrs: true,
		},
		UID: uint32(os.Getuid()),
		GID: uint32(os.Getgid()),
	}

	server, err := fs.Mount(mountpoint, NewVFSRoot(idx), opts)
	if err != nil {
		return fmt.Errorf("fuse mount failed: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Mounted codefuse at %s\n", mountpoint)
	fmt.Fprintf(os.Stderr, "Press Ctrl+C or run 'umount %s' to unmount\n", mountpoint)

	// Handle unmount signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Fprintf(os.Stderr, "\nUnmounting...\n")
		server.Unmount()
	}()

	server.Wait()
	return nil
}

// IsSupported checks if FUSE is available on this system.
func IsSupported() bool {
	// Try a quick check: on macOS, /dev/macfuse* or /dev/fuse should exist
	// On Linux, /dev/fuse should exist
	for _, path := range []string{"/dev/fuse", "/dev/macfuse0", "/dev/osxfuse0"} {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}
