// Package scanner walks a project directory and collects source files.
// Language detection is driven by the config package — no hardcoded language lists.
package scanner

import (
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/yifanmeng/codefuse/pkg/config"
	"github.com/yifanmeng/codefuse/pkg/types"
)

var skipDirs = map[string]bool{
	".git":         true,
	".svn":         true,
	".hg":          true,
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	"build":        true,
	"target":       true,
	".idea":        true,
	".vscode":      true,
	"__pycache__":  true,
	".pytest_cache": true,
	".mypy_cache":  true,
	".tox":         true,
	".egg-info":    true,
	".venv":        true,
	"venv":         true,
}

// Scan walks the project directory and returns source files with language info.
func Scan(root string) ([]types.FileEntry, error) {
	var files []types.FileEntry

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		rel, _ := filepath.Rel(root, path)
		if rel == "." {
			return nil
		}

		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") && name != "." {
				if skipDirs[name] || name == ".git" {
					return filepath.SkipDir
				}
			}
			if skipDirs[name] {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		lang, ok := config.ExtToLang[ext]
		if !ok {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		cfg := config.Builtin[lang]
		files = append(files, types.FileEntry{
			Path:     rel,
			AbsPath:  path,
			Language: lang,
			Size:     info.Size(),
			Mtime:    info.ModTime().UnixNano(),
			IsTest:   isTestFile(rel, cfg.TestPatterns),
		})

		return nil
	})

	return files, err
}

func isTestFile(path string, patterns []string) bool {
	base := filepath.Base(path)
	for _, p := range patterns {
		if strings.Contains(base, p) {
			return true
		}
	}
	return false
}
