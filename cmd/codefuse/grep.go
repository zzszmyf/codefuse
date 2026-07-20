package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/yifanmeng/codefuse/internal/index"
	"github.com/yifanmeng/codefuse/pkg/types"
)

// grepOptions holds parsed grep-style flags.
type grepOptions struct {
	pattern       string
	paths         []string
	recursive     bool
	lineNumber    bool // -n
	filesOnly     bool // -l
	ignoreCase    bool // -i
	countOnly     bool // -c
	wordRegexp    bool // -w
	invertMatch   bool // -v
	onlyMatching  bool // -o
	maxCount      int  // -m N (0 = unlimited)
	quiet         bool // -q
	forceText     bool // -t / --text (codefuse ext: skip index, use real grep)
	contextAfter  int  // -A N
	contextBefore int  // -B N
	contextAround int  // -C N
	showFile      bool // -H (default with multiple files)
	noFile        bool // -h
	include       []string // --include=GLOB
	exclude       []string // --exclude=GLOB
}

// runGrepCompat is the grep-compatible entry point.
func runGrepCompat(args []string) error {
	opts := parseGrepFlags(args)

	// -t / --text: force text search, bypass index.
	if opts.forceText {
		return execRealGrep(opts)
	}

	// Find project root and load index.
	indexDir, projectPath := findIndexDir()
	if indexDir == "" {
		return execRealGrep(opts)
	}

	// Load nodes only — grep queries don't need edges.
	graph, err := index.LoadGraphNodes(indexDir)
	if err != nil {
		return execRealGrep(opts)
	}

	// Query the thin index (pass ignoreCase for trie prefix search).
	results := graph.Query(opts.pattern, opts.ignoreCase)
	if len(results) == 0 {
		if opts.countOnly || opts.quiet {
			if opts.countOnly {
				fmt.Println("0")
			}
			return nil
		}
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
		lineStart := node.Line - opts.contextBefore
		lineEnd := node.Line + opts.contextAfter
		lines, err := index.ReadLines(absPath, lineStart, lineEnd)
		if err != nil {
			continue
		}

		content := ""
		if len(lines) > 0 {
			// The matched line is at index opts.contextBefore.
			matchedIdx := opts.contextBefore
			if matchedIdx < len(lines) {
				content = lines[matchedIdx]
			}
		}

		// For -A/-B/-C, include all context lines with -- separator between groups.
		if (opts.contextAfter > 0 || opts.contextBefore > 0 || opts.contextAround > 0) && len(lines) > 1 {
			content = strings.Join(lines, "\n")
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

	// Apply word-regexp filter (-w).
	if opts.wordRegexp {
		hits = filterWordMatch(hits, opts.pattern)
	}

	// Apply invert-match (-v).
	if opts.invertMatch {
		// Invert: return all nodes NOT in hits (limited to same files).
		hits = invertHits(hits, results)
	}

	// Apply max-count (-m).
	if opts.maxCount > 0 && len(hits) > opts.maxCount {
		hits = hits[:opts.maxCount]
	}

	// Quiet mode (-q): just return exit code.
	if opts.quiet {
		if len(hits) > 0 {
			return nil
		}
		os.Exit(1)
	}

	// Count-only (-c).
	if opts.countOnly {
		fmt.Println(len(hits))
		return nil
	}

	// Output in grep format.
	prevFile := ""
	for i, h := range hits {
		if opts.filesOnly {
			fmt.Println(h.file)
			continue
		}

		// Context groups: print -- separator between non-adjacent matches.
		if (opts.contextAfter > 0 || opts.contextBefore > 0 || opts.contextAround > 0) && i > 0 {
			if prevFile != h.file || hits[i-1].line+opts.contextAfter+1 < h.line-opts.contextBefore {
				fmt.Println("--")
			}
		}
		prevFile = h.file

		linePrefix := ""
		if !opts.noFile {
			linePrefix = h.file + ":"
		}
		if opts.lineNumber {
			linePrefix += fmt.Sprintf("%d:", h.line)
		}

		// -o (only-matching): print just the pattern match.
		if opts.onlyMatching {
			fmt.Println(opts.pattern)
			continue
		}

		// Normal output: file:line:content
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
		recursive:  true,
		lineNumber: true,
		showFile:   true,
	}

	i := 0
	for i < len(args) {
		arg := args[i]

		if !strings.HasPrefix(arg, "-") || arg == "-" {
			// Non-flag: first is pattern, rest are paths.
			if opts.pattern == "" {
				opts.pattern = arg
			} else {
				opts.paths = append(opts.paths, arg)
			}
			i++
			continue
		}

		// Long flags: --text, --include=..., --exclude=...
		if strings.HasPrefix(arg, "--") {
			switch {
			case arg == "--text":
				opts.forceText = true
			case strings.HasPrefix(arg, "--include="):
				opts.include = append(opts.include, strings.TrimPrefix(arg, "--include="))
			case strings.HasPrefix(arg, "--exclude="):
				opts.exclude = append(opts.exclude, strings.TrimPrefix(arg, "--exclude="))
			default:
				// Unknown long flag, treat as pattern if none set.
				if opts.pattern == "" {
					opts.pattern = arg
				}
			}
			i++
			continue
		}

		// Short flags: -r, -n, -A3, -B2, -rn, etc.
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
		case "-v":
			opts.invertMatch = true
		case "-o":
			opts.onlyMatching = true
		case "-H":
			opts.showFile = true
		case "-h":
			opts.noFile = true
		case "-q", "--quiet":
			opts.quiet = true
		case "-t", "--text":
			opts.forceText = true
		case "-A", "-B", "-C":
			// Space-separated: -A 3
			flag := arg
			i++
			if i < len(args) {
				n, err := strconv.Atoi(args[i])
				if err == nil {
					switch flag {
					case "-A":
						opts.contextAfter = n
					case "-B":
						opts.contextBefore = n
					case "-C":
						opts.contextAround = n
						opts.contextBefore = n
						opts.contextAfter = n
					}
				}
			}
		case "-e":
			// Pattern follows: -e pattern
			i++
			if i < len(args) {
				opts.pattern = args[i]
			}
		case "-m":
			// Max count: -m 10
			i++
			if i < len(args) {
				n, err := strconv.Atoi(args[i])
				if err == nil {
					opts.maxCount = n
				}
			}
		default:
			// Check for combined flags with numbers: -A3, -B2, -C5, -m10
			if handled := parseCombinedNumFlag(arg, &opts); handled {
				i++
				continue
			}

			// Combined short flags: -rn, -il, -vn, etc.
			if len(arg) > 2 && arg[1] != '-' {
				if handleCombinedShort(arg, &opts) {
					i++
					continue
				}
			}

			// Unknown flag or non-flag argument.
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

// parseCombinedNumFlag handles flags like -A3, -B2, -C5, -m10.
func parseCombinedNumFlag(arg string, opts *grepOptions) bool {
	if len(arg) < 3 || arg[0] != '-' || arg[1] == '-' {
		return false
	}
	flag := arg[0:2]
	numStr := arg[2:]
	n, err := strconv.Atoi(numStr)
	if err != nil {
		return false
	}
	switch flag {
	case "-A":
		opts.contextAfter = n
	case "-B":
		opts.contextBefore = n
	case "-C":
		opts.contextAround = n
		opts.contextBefore = n
		opts.contextAfter = n
	case "-m":
		opts.maxCount = n
	default:
		return false
	}
	return true
}

// handleCombinedShort handles combined short flags like -rn, -il, -vnc.
func handleCombinedShort(arg string, opts *grepOptions) bool {
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
		case 'v':
			opts.invertMatch = true
		case 'o':
			opts.onlyMatching = true
		case 'H':
			opts.showFile = true
		case 'h':
			opts.noFile = true
		case 'q':
			opts.quiet = true
		default:
			return false // unrecognized char, not a combined flag
		}
	}
	return true
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
	if opts.invertMatch {
		grepArgs = append(grepArgs, "-v")
	}
	if opts.onlyMatching {
		grepArgs = append(grepArgs, "-o")
	}
	if opts.quiet {
		grepArgs = append(grepArgs, "-q")
	}
	if opts.maxCount > 0 {
		grepArgs = append(grepArgs, fmt.Sprintf("-m%d", opts.maxCount))
	}
	if opts.noFile {
		grepArgs = append(grepArgs, "-h")
	}
	if opts.showFile {
		grepArgs = append(grepArgs, "-H")
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
	for _, inc := range opts.include {
		grepArgs = append(grepArgs, "--include="+inc)
	}
	for _, exc := range opts.exclude {
		grepArgs = append(grepArgs, "--exclude="+exc)
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
func filterByPaths(hits []hit, paths []string) []hit {
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

// invertHits returns nodes that are NOT in hits (for -v flag).
func invertHits(hits []hit, allResults []types.Node) []hit {
	// Build set of matched (file, line) pairs.
	matched := make(map[string]bool)
	for _, h := range hits {
		key := fmt.Sprintf("%s:%d", h.file, h.line)
		matched[key] = true
	}

	// Return unmatched nodes.
	var inverted []hit
	seen := make(map[string]bool)
	for _, node := range allResults {
		key := fmt.Sprintf("%s:%d", node.File, node.Line)
		if matched[key] || seen[key] {
			continue
		}
		seen[key] = true
		inverted = append(inverted, hit{
			file:   node.File,
			line:   node.Line,
			column: node.Column,
			name:   node.Name,
		})
	}
	return inverted
}

type hit struct {
	file    string
	line    int
	column  int
	name    string
	content string
}
