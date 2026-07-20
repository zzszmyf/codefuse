package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	// Grep-compatible mode: when argv[0] is "grep", behave like grep.
	if isGrepMode() {
		if err := runGrepCompat(os.Args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// isGrepMode returns true if the binary is invoked as "grep".
func isGrepMode() bool {
	base := filepath.Base(os.Args[0])
	return base == "grep" || strings.HasPrefix(base, "grep")
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "codefuse",
		Short: "CodeFuse — grep replacement for AI agents",
		Long: `CodeFuse indexes your codebase into a thin symbol→location map,
then exposes it via a grep-compatible interface.

When invoked as "grep" (symlink or renamed binary), it behaves exactly
like grep — same flags, same output format — but returns symbol
definitions instead of raw text matches.

Index is built once, queries hit the index + actual source files
for 100% accurate results.`,
		Version: version,
	}

	root.AddCommand(newIndexCmd())
	root.AddCommand(newListCmd())
	root.AddCommand(newQueryCmd())
	root.AddCommand(newOutlineCmd())
	root.AddCommand(newGrepCmd())
	root.AddCommand(newWatchCmd())
	root.AddCommand(newServeCmd())
	root.AddCommand(newSetupCmd())

	return root
}

func newIndexCmd() *cobra.Command {
	var projectPath string

	cmd := &cobra.Command{
		Use:   "index [path]",
		Short: "Build the thin symbol index for a codebase",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				projectPath = args[0]
			}
			if projectPath == "" {
				projectPath = "."
			}
			return runIndex(projectPath)
		},
	}

	return cmd
}

func newListCmd() *cobra.Command {
	var project string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List indexed files and symbols",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(project)
		},
	}

	cmd.Flags().StringVarP(&project, "project", "p", ".", "Project path")

	return cmd
}

func newQueryCmd() *cobra.Command {
	var (
		project string
		callers bool
		callees bool
	)

	cmd := &cobra.Command{
		Use:   "query <symbol>",
		Short: "Query a symbol in the index",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runQuery(project, args[0], callers, callees)
		},
	}

	cmd.Flags().StringVarP(&project, "project", "p", ".", "Project path")
	cmd.Flags().BoolVar(&callers, "callers", false, "Show who calls this symbol")
	cmd.Flags().BoolVar(&callees, "callees", false, "Show who this symbol calls")

	return cmd
}

func newOutlineCmd() *cobra.Command {
	var project string

	cmd := &cobra.Command{
		Use:   "outline <file>",
		Short: "Show symbols in a file, sorted by line",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runOutline(project, args[0])
		},
	}

	cmd.Flags().StringVarP(&project, "project", "p", ".", "Project path")

	return cmd
}

func newGrepCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "grep [grep-flags] <pattern> [path...]",
		Short: "Grep-compatible symbol search (fallback to real grep for text)",
		Long: `Search symbols using grep-compatible syntax.

This command mimics grep: same flags (-rn, -l, -i, -c, -w, -A/-B/-C),
same output format (file:line:content). It queries the thin index for
symbol definitions and reads actual source files for current content.

Use -t/--text to skip the index and run real grep directly.`,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGrepCompat(args)
		},
	}

	return cmd
}

func newWatchCmd() *cobra.Command {
	var project string

	cmd := &cobra.Command{
		Use:   "watch [path]",
		Short: "Watch for file changes and update index in real time",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				project = args[0]
			}
			if project == "" {
				project = "."
			}
			return runWatch(project)
		},
	}

	return cmd
}

func newServeCmd() *cobra.Command {
	var project string

	cmd := &cobra.Command{
		Use:   "serve [path]",
		Short: "Start MCP server for AI agent integration (stdio JSON-RPC)",
		Long: `Start an MCP (Model Context Protocol) server over stdio.

Keeps the index loaded in memory for zero-latency queries.
Exposes tools: find_symbol, find_callers, find_callees, get_outline.

Claude Code / Cursor configuration:
  {
    "mcpServers": {
      "codefuse": {
        "command": "codefuse",
        "args": ["serve", "/path/to/project"]
      }
    }
  }`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				project = args[0]
			}
			if project == "" {
				project = "."
			}
			return runServe(project)
		},
	}

	return cmd
}

func newSetupCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "setup",
		Short: "Set up codefuse dependencies",
	}

	var (
		project string
		auto    bool
	)

	treesitter := &cobra.Command{
		Use:   "treesitter",
		Short: "Install tree-sitter grammars for detected languages",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetupTreeSitter(project, auto)
		},
	}

	treesitter.Flags().StringVarP(&project, "project", "p", ".", "Project path")
	treesitter.Flags().BoolVar(&auto, "auto", false, "Automatically install missing grammars")

	root.AddCommand(treesitter)
	return root
}
