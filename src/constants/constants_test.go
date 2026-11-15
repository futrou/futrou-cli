package constants

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestFolderNames(t *testing.T) {
	for got, want := range map[string]string{BuildDir: ".futrou", CacheDir: ".futrou/cache", StaticoDir: ".futrou/statico", TmpDir: ".futrou/tmp", DebugDir: ".futrou/debug"} {
		if got != want {
			t.Errorf("folder = %q, want %q", got, want)
		}
	}
	// the folders are plain names directly under BuildDir, so they cannot escape it
	for _, dir := range []string{CacheDir, StaticoDir, TmpDir, DebugDir} {
		name := strings.TrimPrefix(dir, BuildDir+"/")
		if name == dir || name == "" || name != filepath.Base(name) || strings.HasPrefix(name, ".") {
			t.Errorf("%q is not a plain folder directly under %q", dir, BuildDir)
		}
	}
	seen := map[string]bool{}
	for _, dir := range []string{CacheDir, StaticoDir, TmpDir, DebugDir} {
		if seen[dir] {
			t.Errorf("%q is used twice: the folders under BuildDir must differ", dir)
		}
		seen[dir] = true
	}
}

// The base folder name lives only here: every other file must use BuildDir,
// so the name cannot drift between the home folder and the work folder.
func TestBuildDirIsNotHardCodedElsewhere(t *testing.T) {
	root := filepath.Join("..")
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		if filepath.Base(path) == "constants.go" || strings.Contains(filepath.ToSlash(path), "/api/") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if value, _ := strconv.Unquote(lit.Value); value == BuildDir {
					t.Errorf("%s hard-codes %q: use constants.BuildDir", path, value)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The default log format is one of the values --log-format documents ("text or json").
func TestDefaultLogFormatIsADocumentedValue(t *testing.T) {
	if DefaultLogFormat != "text" && DefaultLogFormat != "json" {
		t.Errorf("DefaultLogFormat = %q, want text or json", DefaultLogFormat)
	}
}
