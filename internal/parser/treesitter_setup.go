package parser

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// TreeSitterConfig represents tree-sitter's config.json
type TreeSitterConfig struct {
	ParserDirectories []string `json:"parser-directories"`
}

// GrammarInfo maps a language to its tree-sitter grammar repository.
type GrammarInfo struct {
	Lang      string
	Repo      string // GitHub repo, e.g. "tree-sitter/tree-sitter-go"
	DirName   string // Local directory name, e.g. "tree-sitter-go"
	Installed bool
}

// KnownGrammars is the built-in registry of tree-sitter grammars.
var KnownGrammars = map[string]GrammarInfo{
	"go":         {Lang: "go", Repo: "tree-sitter/tree-sitter-go", DirName: "tree-sitter-go"},
	"javascript": {Lang: "javascript", Repo: "tree-sitter/tree-sitter-javascript", DirName: "tree-sitter-javascript"},
	"typescript": {Lang: "typescript", Repo: "tree-sitter/tree-sitter-typescript", DirName: "tree-sitter-typescript"},
	"python":     {Lang: "python", Repo: "tree-sitter/tree-sitter-python", DirName: "tree-sitter-python"},
	"rust":       {Lang: "rust", Repo: "tree-sitter/tree-sitter-rust", DirName: "tree-sitter-rust"},
	"java":       {Lang: "java", Repo: "tree-sitter/tree-sitter-java", DirName: "tree-sitter-java"},
	"c":          {Lang: "c", Repo: "tree-sitter/tree-sitter-c", DirName: "tree-sitter-c"},
	"cpp":        {Lang: "cpp", Repo: "tree-sitter/tree-sitter-cpp", DirName: "tree-sitter-cpp"},
}

// DetectMissingGrammars returns grammars needed for the given languages
// that are not already installed.
func DetectMissingGrammars(languages []string) ([]GrammarInfo, error) {
	if !TreeSitterAvailable() {
		return nil, fmt.Errorf("tree-sitter CLI not found. Install it first: npm install -g tree-sitter-cli")
	}

	config, err := LoadTreeSitterConfig()
	if err != nil {
		config = &TreeSitterConfig{}
	}

	installed := findInstalledGrammars(config)

	var missing []GrammarInfo
	seen := make(map[string]bool)
	for _, lang := range languages {
		if seen[lang] {
			continue
		}
		seen[lang] = true
		g, ok := KnownGrammars[lang]
		if !ok {
			continue // Unknown language, skip
		}
		if !installed[g.DirName] {
			g.Installed = false
			missing = append(missing, g)
		}
	}
	return missing, nil
}

// LoadTreeSitterConfig reads tree-sitter's config.json from the default location.
func LoadTreeSitterConfig() (*TreeSitterConfig, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	configPath := filepath.Join(home, ".config", "tree-sitter", "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	var config TreeSitterConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

// SaveTreeSitterConfig writes tree-sitter's config.json.
func SaveTreeSitterConfig(config *TreeSitterConfig) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	configDir := filepath.Join(home, ".config", "tree-sitter")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(configDir, "config.json"), data, 0644)
}

// findInstalledGrammars scans parser-directories for existing grammar repos.
func findInstalledGrammars(config *TreeSitterConfig) map[string]bool {
	installed := make(map[string]bool)
	for _, dir := range config.ParserDirectories {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() && strings.HasPrefix(entry.Name(), "tree-sitter-") {
				installed[entry.Name()] = true
			}
		}
	}
	return installed
}

// InstallGrammar clones a grammar repo and builds the WASM parser.
func InstallGrammar(g GrammarInfo, targetDir string) error {
	grammarsDir := filepath.Join(targetDir, g.DirName)

	fmt.Fprintf(os.Stderr, "Installing %s grammar...\n", g.Lang)

	// Clone the repo
	url := fmt.Sprintf("https://github.com/%s.git", g.Repo)
	cmd := exec.Command("git", "clone", "--depth", "1", url, grammarsDir)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone failed for %s: %w", g.Repo, err)
	}

	// Build WASM
	cmd = exec.Command("tree-sitter", "build", "--wasm")
	cmd.Dir = grammarsDir
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tree-sitter build failed for %s: %w", g.Lang, err)
	}

	return nil
}

// AddParserDirectory adds a directory to tree-sitter's parser-directories if not present.
func AddParserDirectory(dir string) error {
	config, err := LoadTreeSitterConfig()
	if err != nil {
		config = &TreeSitterConfig{}
	}
	for _, existing := range config.ParserDirectories {
		if existing == dir {
			return nil // Already present
		}
	}
	config.ParserDirectories = append(config.ParserDirectories, dir)
	return SaveTreeSitterConfig(config)
}
