package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:     "codefuse",
		Short:   "CodeFuse - Code Virtual File System for AI Agents",
		Long:    `CodeFuse indexes your codebase and exposes it as queryable views.`,
		Version: version,
	}

	root.AddCommand(newIndexCmd())
	root.AddCommand(newListCmd())
	root.AddCommand(newQueryCmd())
	root.AddCommand(newOutlineCmd())
	root.AddCommand(newVFSCmd())
	root.AddCommand(newMountCmd())
	root.AddCommand(newSetupCmd())

	return root
}

func newIndexCmd() *cobra.Command {
	var (
		projectPath    string
		force          bool
		useTreeSitter  bool
	)

	cmd := &cobra.Command{
		Use:   "index <path>",
		Short: "Index a codebase",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath = args[0]
			return runIndex(projectPath, force, useTreeSitter)
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force re-index even if up to date")
	cmd.Flags().BoolVar(&useTreeSitter, "treesitter", false, "Use tree-sitter CLI for parsing (higher accuracy, slower first run)")

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

	cmd.Flags().StringVarP(&project, "project", "p", ".", "Project path (defaults to current directory)")

	return cmd
}

func newQueryCmd() *cobra.Command {
	var (
		project string
		kind    string
		callers bool
		callees bool
	)

	cmd := &cobra.Command{
		Use:   "query <symbol>",
		Short: "Query a symbol in the index",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runQuery(project, args[0], kind, callers, callees)
		},
	}

	cmd.Flags().StringVarP(&project, "project", "p", ".", "Project path")
	cmd.Flags().StringVarP(&kind, "kind", "k", "", "Filter by symbol kind (func, class, var, etc.)")
	cmd.Flags().BoolVar(&callers, "callers", false, "Show who calls this symbol (requires call graph)")
	cmd.Flags().BoolVar(&callees, "callees", false, "Show who this symbol calls (requires call graph)")

	return cmd
}

func newOutlineCmd() *cobra.Command {
	var project string

	cmd := &cobra.Command{
		Use:   "outline <file>",
		Short: "Show the structured outline of a file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runOutline(project, args[0])
		},
	}

	cmd.Flags().StringVarP(&project, "project", "p", ".", "Project path")

	return cmd
}

func newVFSCmd() *cobra.Command {
	var project string

	cmd := &cobra.Command{
		Use:   "vfs",
		Short: "Virtual filesystem views for code exploration",
	}

	generate := &cobra.Command{
		Use:   "generate",
		Short: "Generate VFS views (symbols, outline, references) in .codefuse/vfs/",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVFSGenerate(project)
		},
	}
	generate.Flags().StringVarP(&project, "project", "p", ".", "Project path")

	cmd.AddCommand(generate)
	return cmd
}

func newMountCmd() *cobra.Command {
	var project string

	cmd := &cobra.Command{
		Use:   "mount <mountpoint>",
		Short: "Mount the codebase as a virtual filesystem (FUSE)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMount(project, args[0])
		},
	}

	cmd.Flags().StringVarP(&project, "project", "p", ".", "Project path")

	return cmd
}

func newSetupCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "setup",
		Short: "Set up codefuse dependencies and configuration",
	}

	var (
		project string
		auto    bool
	)

	treesitter := &cobra.Command{
		Use:   "treesitter",
		Short: "Set up tree-sitter grammars for your project's languages",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetupTreeSitter(project, auto)
		},
	}

	treesitter.Flags().StringVarP(&project, "project", "p", ".", "Project path")
	treesitter.Flags().BoolVar(&auto, "auto", false, "Automatically clone and build missing grammars")

	root.AddCommand(treesitter)
	return root
}
