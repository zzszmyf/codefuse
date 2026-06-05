package scanner

import (
	"io/fs"
	"path/filepath"
	"strings"

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

var sourceExts = map[string]string{
	".go":    types.LangGo,
	".py":    types.LangPython,
	".rs":    types.LangRust,
	".js":    types.LangJS,
	".jsx":   types.LangJS,
	".ts":    types.LangTS,
	".tsx":   types.LangTS,
	".java":  types.LangJava,
	".c":     types.LangC,
	".h":     types.LangC,
	".cpp":   types.LangCPP,
	".cc":    types.LangCPP,
	".hpp":   types.LangCPP,
}

// Scan walks the project directory and returns source files
func Scan(root string) ([]types.FileEntry, error) {
	var files []types.FileEntry

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}

		rel, _ := filepath.Rel(root, path)
		if rel == "." {
			return nil
		}

		// Skip hidden dirs and known non-source dirs
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

		// Only source files
		ext := strings.ToLower(filepath.Ext(path))
		lang, ok := sourceExts[ext]
		if !ok {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		files = append(files, types.FileEntry{
			Path:     rel,
			AbsPath:  path,
			Language: lang,
			Size:     info.Size(),
			Mtime:    info.ModTime().UnixNano(),
			IsTest:   isTestFile(rel, lang),
		})

		return nil
	})

	return files, err
}

func isTestFile(path string, lang string) bool {
	base := filepath.Base(path)
	switch lang {
	case types.LangGo:
		return strings.HasSuffix(base, "_test.go")
	case types.LangPython:
		return strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py")
	case types.LangRust:
		return strings.HasSuffix(base, ".rs") && strings.Contains(base, "test")
	case types.LangJS, types.LangTS:
		return strings.Contains(base, ".test.") || strings.Contains(base, ".spec.")
	}
	return false
}
