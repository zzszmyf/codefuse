package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/yifanmeng/codefuse/internal/index"
)

// =============================================================================
// Grep-compatible mode
//
// When invoked as "grep" (argv[0] contains "grep"), codefuse mimics the grep
// interface: same flags, same output format. Under the hood, it queries the
// thin index and reads actual source files for line content — so results are
// always current.
//
// If no index exists or --text is passed, it falls through to the real grep.
// =============================================================================

// grepOptions holds parsed grep-style flags.
type grepOptions struct {
	pattern      string
	paths        []string
	recursive    bool
	lineNumber   bool   // -n
	filesOnly    bool   // -l
	ignoreCase   bool   // -i
	countOnly    bool   // -c
	wordRegexp   bool   // -w
	forceText    bool   // -t / --text (codefuse ext: skip index, use real grep)
	contextAfter int    // -A N
	contextBefore int   // -B N
	contextAround int   // -C N
	showFile     bool   // -H (default with multiple files)
	noFile       bool   // -h
}

// runGrepCompat is the grep-compatible entry point.
// Invoked when argv[0] is "grep" or when the "grep" subcommand is used.
func runGrepCompat(args []string) error {
	opts := parseGrepFlags(args)

	// --text / -t: force text search, bypass index.
	if opts.forceText {
		return execRealGrep(opts)
	}

	// Find project root and load index.
	indexDir, projectPath := findIndexDir()
	if indexDir == "" {
		// No index found → fall back to real grep.
		return execRealGrep(opts)
	}

	// Load nodes only — grep queries don't need edges.
	graph, err := index.LoadGraphNodes(indexDir)
	if err != nil {
		return execRealGrep(opts)
	}

	// Query the thin index.
	results := graph.Query(opts.pattern)
	if len(results) == 0 {
		// Symbol not found. In count-only mode, just output 0.
		if opts.countOnly {
			fmt.Println("0")
			return nil
		}
		// Fall back to real grep for text search (comments, strings, etc.).
		return execRealGrep(opts)
	}

	// Resolve file paths to absolute for reading source.
	basePath := projectPath

	// Collect results, deduplicate by (file, line).
	var hits []hit
	seen := make(map[string]bool)

	for _, node := range results {
		key := fmt.Sprintf("%s:%d", node.File, node.Line)
		if seen[key] {
			continue
		}
		seen[key] = true

		absPath := filepath.Join(basePath, node.File)

		// Read context lines from actual source (truth source).
		contextLines := 1
		if opts.contextAround > 0 {
			contextLines = 1 + opts.contextAround
		} else if opts.contextAfter > 0 || opts.contextBefore > 0 {
			contextLines = 1 + opts.contextAfter + opts.contextBefore
		}

		lines, err := index.ReadLines(absPath,
			node.Line-opts.contextBefore,
			node.Line+opts.contextAfter)
		if err != nil {
			continue
		}
		content := ""
		if len(lines) > 0 {
			content = lines[len(lines)-1-opts.contextAfter] // The matched line
		}
		// For -A/-B/-C, include all context lines.
		if contextLines > 1 && len(lines) > 1 {
			content = strings.Join(lines, "\n-")
		}

		hits = append(hits, hit{
			file:    node.File,
			line:    node.Line,
			column:  node.Column,
			name:    node.Name,
			content: content,
		})
	}

	// Filter by paths if specified.
	if len(opts.paths) > 0 {
		hits = filterByPaths(hits, opts.paths)
	}

	// Apply word-regexp filter (-w): match whole word only.
	if opts.wordRegexp {
		hits = filterWordMatch(hits, opts.pattern)
	}

	// Output in grep format.
	if opts.countOnly {
		fmt.Println(len(hits))
		return nil
	}

	for _, h := range hits {
		if opts.filesOnly {
			fmt.Println(h.file)
			continue
		}

		linePrefix := ""
		if opts.showFile && !opts.noFile {
			linePrefix = h.file + ":"
		} else if opts.noFile {
			linePrefix = ""
		} else {
			linePrefix = h.file + ":"
		}
		if opts.lineNumber {
			linePrefix += fmt.Sprintf("%d:", h.line)
		}

		// Output: file:line:content
		if h.content != "" {
			fmt.Printf("%s%s\n", linePrefix, h.content)
		} else {
			fmt.Printf("%s%s\n", linePrefix, h.name)
		}
	}

	return nil
}

// parseGrepFlags parses grep-style command line arguments.
func parseGrepFlags(args []string) grepOptions {
	opts := grepOptions{
		recursive:  true, // codefuse defaults to recursive
		lineNumber: true, // codefuse defaults to line numbers
		showFile:   true,
	}

	i := 0
	for i < len(args) {
		arg := args[i]

		// Handle flags.
		if strings.HasPrefix(arg, "-") && arg != "-" {
			switch arg {
			case "-r", "-R":
				opts.recursive = true
			case "-n":
				opts.lineNumber = true
			case "-l":
				opts.filesOnly = true
			case "-i":
				opts.ignoreCase = true
			case "-c":
				opts.countOnly = true
			case "-w":
				opts.wordRegexp = true
			case "-H":
				opts.showFile = true
			case "-h":
				opts.noFile = true
			case "-t", "--text":
				opts.forceText = true
			case "-A":
				i++
				if i < len(args) {
					fmt.Sscanf(args[i], "%d", &opts.contextAfter)
				}
			case "-B":
				i++
				if i < len(args) {
					fmt.Sscanf(args[i], "%d", &opts.contextBefore)
				}
			case "-C":
				i++
				if i < len(args) {
					fmt.Sscanf(args[i], "%d", &opts.contextAround)
				}
			case "-e":
				i++
				if i < len(args) {
					opts.pattern = args[i]
				}
			default:
				// Combined short flags: -rn, -il, etc.
				if strings.HasPrefix(arg, "-") && len(arg) > 2 && arg[1] != '-' {
					for _, c := range arg[1:] {
						switch c {
						case 'r', 'R':
							opts.recursive = true
						case 'n':
							opts.lineNumber = true
						case 'l':
							opts.filesOnly = true
						case 'i':
							opts.ignoreCase = true
						case 'c':
							opts.countOnly = true
						case 'w':
							opts.wordRegexp = true
						case 'H':
							opts.showFile = true
						case 'h':
							opts.noFile = true
						}
					}
				} else {
					// Unknown flag or non-flag argument.
					if opts.pattern == "" {
						opts.pattern = arg
					} else {
						opts.paths = append(opts.paths, arg)
					}
				}
			}
		} else {
			// Non-flag argument: first is pattern, rest are paths.
			if opts.pattern == "" {
				opts.pattern = arg
			} else {
				opts.paths = append(opts.paths, arg)
			}
		}
		i++
	}

	return opts
}

// execRealGrep falls back to the actual grep binary.
func execRealGrep(opts grepOptions) error {
	var grepArgs []string

	if opts.recursive {
		grepArgs = append(grepArgs, "-r")
	}
	if opts.lineNumber {
		grepArgs = append(grepArgs, "-n")
	}
	if opts.filesOnly {
		grepArgs = append(grepArgs, "-l")
	}
	if opts.ignoreCase {
		grepArgs = append(grepArgs, "-i")
	}
	if opts.countOnly {
		grepArgs = append(grepArgs, "-c")
	}
	if opts.wordRegexp {
		grepArgs = append(grepArgs, "-w")
	}
	if opts.contextAfter > 0 {
		grepArgs = append(grepArgs, fmt.Sprintf("-A%d", opts.contextAfter))
	}
	if opts.contextBefore > 0 {
		grepArgs = append(grepArgs, fmt.Sprintf("-B%d", opts.contextBefore))
	}
	if opts.contextAround > 0 {
		grepArgs = append(grepArgs, fmt.Sprintf("-C%d", opts.contextAround))
	}

	grepArgs = append(grepArgs, opts.pattern)
	grepArgs = append(grepArgs, opts.paths...)

	if len(opts.paths) == 0 {
		grepArgs = append(grepArgs, ".")
	}

	cmd := exec.Command("grep", grepArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// findIndexDir walks up from the current directory looking for .codefuse/.
// Returns (indexDir, projectPath).
func findIndexDir() (string, string) {
	dir, _ := os.Getwd()
	for {
		indexDir := filepath.Join(dir, ".codefuse")
		if info, err := os.Stat(indexDir); err == nil && info.IsDir() {
			return indexDir, dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", ""
}

// filterByPaths filters hits to only those within the given path prefixes.
// "." matches everything. Otherwise, matches by path prefix or exact basename.
func filterByPaths(hits []hit, paths []string) []hit {
	// If only "." or empty, no filtering needed.
	allDot := true
	for _, p := range paths {
		if p != "." {
			allDot = false
			break
		}
	}
	if allDot || len(paths) == 0 {
		return hits
	}

	var filtered []hit
	for _, h := range hits {
		for _, p := range paths {
			p = strings.TrimPrefix(p, "./")
			if strings.HasPrefix(h.file, p) ||
				strings.HasPrefix(h.file, "./"+p) ||
				filepath.Base(h.file) == p {
				filtered = append(filtered, h)
				break
			}
		}
	}
	return filtered
}

// filterWordMatch filters hits where the pattern matches as a whole word.
func filterWordMatch(hits []hit, pattern string) []hit {
	var filtered []hit
	for _, h := range hits {
		if isWordMatch(h.content, pattern) || isWordMatch(h.name, pattern) {
			filtered = append(filtered, h)
		}
	}
	return filtered
}

// isWordMatch checks if pattern appears as a whole word in s.
func isWordMatch(s, pattern string) bool {
	lower := strings.ToLower(s)
	pat := strings.ToLower(pattern)
	idx := 0
	for idx < len(lower) {
		pos := strings.Index(lower[idx:], pat)
		if pos < 0 {
			return false
		}
		abs := idx + pos
		before := abs == 0 || !isWordChar(lower[abs-1])
		after := abs+len(pat) >= len(lower) || !isWordChar(lower[abs+len(pat)])
		if before && after {
			return true
		}
		idx = abs + 1
	}
	return false
}

func isWordChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

// hit is a query result ready for grep-format output.
type hit struct {
	file    string
	line    int
	column  int
	name    string
	content string
}
