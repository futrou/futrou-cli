// Package statico uploads, downloads and syncs the files of Futrou statico
// (static website) hosting. A website's files live in a storage, one folder
// per version (StaticoVersion), and are moved with the storage files API.
package statico

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"futrou-cli/src/constants"
	"futrou-cli/src/utils"
)

// Selection says which local files make a website. utils.GlobRules run in order (see
// Rule); without any, the whole base folder lands at the root.
type Selection struct {
	Rules utils.GlobRules
	// NoHTMLToFolders stops the copies of page.html as page/index.html.
	NoHTMLToFolders bool
	// Ignore lists absolute paths that are never uploaded (the config file).
	Ignore []string
}

// LocalFile is one selected file: where it is and where it lands in the website.
type LocalFile struct {
	// Abs is the path on disk.
	Abs string
	// Path is the slash separated path inside the website, without a leading slash.
	Path    string
	Size    int64
	ModTime time.Time
	// Source is the rule source that selected it, for messages.
	Source string
}

// Resolved is what a Selection makes: the files to upload and the redirect
// rules a _redirects file among them declared.
type Resolved struct {
	Files []LocalFile
	// Redirects come from a _redirects file at the root of the website. The file itself is not uploaded.
	Redirects []map[string]interface{}
	// Warnings are the _redirects lines that cannot be converted.
	Warnings []string
	// Skipped counts the hidden files and .map files the includes found and left out.
	Skipped int
}

// safeSegment mirrors the API's rule for file names: no control characters,
// path separators, glob characters or characters reserved on common filesystems.
var safeSegment = regexp.MustCompile(`^[^\x00-\x1f/\\*?"<>|:]+$`)

// ValidSitePath reports why p cannot be a file path inside a website ("" when it can).
func ValidSitePath(p string) string {
	if p == "" {
		return "is empty"
	}
	for _, segment := range strings.Split(p, "/") {
		switch {
		case segment == "" || segment == "." || segment == "..":
			return "has an empty, \".\" or \"..\" segment"
		case strings.TrimSpace(segment) != segment:
			return "has a segment with leading or trailing spaces"
		case !safeSegment.MatchString(segment):
			return fmt.Errorf("has the segment %q with characters the API does not accept (control characters and * ? \" < > | : \\)", segment).Error()
		}
	}
	return ""
}

// skippedDirs are never walked into: they are not part of a website.
var skippedDirs = map[string]bool{".git": true, constants.BuildDir: true}

// match is one file a rule source matched.
type match struct {
	Abs     string
	Size    int64
	ModTime time.Time
	// RelRoot is the path below the fixed part of the source (the folder, or the
	// part of a glob before its first wildcard); for a file its name.
	RelRoot string
	// RelBase is the path relative to the base folder, empty outside it.
	RelBase string
	// File is true when the source named one file, not a folder or a glob.
	File bool
	// Folder is true when the source named a folder (not a glob).
	Folder bool
}

type resolver struct {
	base   string // absolute, slash separated
	ignore map[string]bool
	// skipped counts the hidden and .map files left out of what the include rules selected.
	skipped  int
	counting bool
}

// defaultExcluded reports whether a file found below an include is left out
// without being asked: a hidden file or folder (a name starting with a dot, such as
// .env) or a source map. The names are looked at below the include, so an include
// that names a hidden folder itself (./public/.well-known) takes what is in it, and
// naming one file takes that file. allowMap is true when the include names .map files.
func defaultExcluded(relRoot string, allowMap bool) bool {
	for _, segment := range strings.Split(relRoot, "/") {
		if strings.HasPrefix(segment, ".") {
			return true
		}
	}
	return !allowMap && strings.HasSuffix(strings.ToLower(relRoot), ".map")
}

func slash(p string) string { return filepath.ToSlash(p) }

func (r resolver) absolute(p string) string {
	p = slash(p)
	if !filepath.IsAbs(filepath.FromSlash(p)) && !strings.HasPrefix(p, "/") {
		p = path.Join(r.base, p)
	}
	return path.Clean(p)
}

func (r resolver) relBase(abs string) string {
	rel, err := filepath.Rel(filepath.FromSlash(r.base), filepath.FromSlash(abs))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	return slash(rel)
}

func hasMeta(p string) bool { return utils.HasGlobMeta(p) }

// walk lists the regular files below root. root may itself be a link to a
// folder (an explicit choice of the user); links found below it are never followed.
// accept gets the path of a file as the pattern sees it (below root) and says
// whether the file is wanted.
func (r *resolver) walk(root string, allowMap bool, accept func(patternPath string) bool) ([]match, error) {
	real := root
	if resolved, err := filepath.EvalSymlinks(filepath.FromSlash(root)); err == nil {
		real = slash(resolved)
	}
	var out []match
	err := filepath.WalkDir(filepath.FromSlash(real), func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("cannot read %s: %w", p, unwrapPathError(err))
		}
		abs := slash(p)
		if entry.IsDir() {
			if abs != real && (skippedDirs[entry.Name()] || strings.HasPrefix(entry.Name(), ".")) {
				return filepath.SkipDir // .git, .futrou and every other hidden folder
			}
			return nil
		}
		relRoot := strings.TrimPrefix(abs, real+"/")
		if !entry.Type().IsRegular() || r.ignore[abs] || r.ignore[root+"/"+relRoot] || !accept(root+"/"+relRoot) {
			return nil // links, sockets and devices are skipped, never followed
		}
		if defaultExcluded(relRoot, allowMap) {
			if r.counting {
				r.skipped++
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("cannot read %s: %w", p, unwrapPathError(err))
		}
		out = append(out, match{Abs: abs, Size: info.Size(), ModTime: info.ModTime(), RelRoot: relRoot, RelBase: r.relBase(root + "/" + relRoot)})
		return nil
	})
	return out, err
}

// matches lists the files a rule source selects. A source that is not a glob
// must exist when mustExist is set.
func (r *resolver) matches(source string, mustExist bool) ([]match, error) {
	var all []match
	seen := map[string]bool{}
	for _, pattern := range utils.ExpandBraces(slash(source)) {
		abs := r.absolute(pattern)
		var found []match
		if !hasMeta(abs) {
			info, err := os.Stat(filepath.FromSlash(abs)) // an explicit link to a folder is followed
			switch {
			case err != nil && os.IsNotExist(err):
				if mustExist {
					return nil, fmt.Errorf("include %q does not exist", source)
				}
				continue
			case err != nil:
				return nil, fmt.Errorf("include %q: %w", source, unwrapPathError(err))
			case info.IsDir():
				if found, err = r.walk(abs, strings.Contains(source, ".map"), func(string) bool { return true }); err != nil {
					return nil, err
				}
				for i := range found {
					found[i].Folder = true
				}
			case info.Mode().IsRegular():
				if !r.ignore[abs] {
					found = []match{{Abs: abs, Size: info.Size(), ModTime: info.ModTime(), RelRoot: path.Base(abs), RelBase: r.relBase(abs), File: true}}
				}
			default:
				if mustExist {
					return nil, fmt.Errorf("include %q is not a regular file or folder", source)
				}
			}
		} else {
			root := utils.GlobRoot(abs)
			if info, err := os.Stat(filepath.FromSlash(root)); err != nil || !info.IsDir() {
				if err != nil && !os.IsNotExist(err) {
					return nil, fmt.Errorf("include %q: %w", source, unwrapPathError(err))
				}
				continue
			}
			var err error
			found, err = r.walk(root, strings.Contains(source, ".map"), func(p string) bool {
				// a pattern that names a folder takes everything below it
				return utils.GlobMatch(abs, p) || utils.GlobMatch(abs+"/**", p)
			})
			if err != nil {
				return nil, err
			}
		}
		for _, m := range found {
			if !seen[m.Abs] {
				seen[m.Abs] = true
				all = append(all, m)
			}
		}
	}
	return all, nil
}

// destPath is where a matched file lands in the website.
func destPath(rule utils.GlobRule, m match) (string, error) {
	if rule.Dest == "" {
		if m.Folder {
			return m.RelRoot, nil // a folder is its files at the root, unless it is sent somewhere
		}
		if m.RelBase != "" {
			return m.RelBase, nil // true: the path relative to the config is kept
		}
		return m.RelRoot, nil // outside the config folder there is no such path
	}
	dest := slash(strings.TrimSpace(rule.Dest))
	dirLike := dest == "." || strings.HasSuffix(dest, "/")
	clean := strings.Trim(path.Clean(strings.TrimPrefix(dest, "./")), "/")
	if clean == "." {
		clean = ""
	}
	if clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return "", fmt.Errorf("the destination %q of %q leaves the website", rule.Dest, rule.Source)
	}
	if m.File && !dirLike {
		return clean, nil // a file with a name as destination is renamed
	}
	return strings.TrimPrefix(path.Join(clean, m.RelRoot), "/"), nil
}

// Resolve applies the rules in order against base (the folder of the config, or
// the current folder) and returns the files of the website. An include that
// matches nothing fails, and so does a file the API would not accept or that
// cannot be read, so a bad selection is found before anything is sent.
func Resolve(base string, sel Selection) (*Resolved, error) {
	absBase, err := filepath.Abs(base)
	if err != nil {
		return nil, err
	}
	r := &resolver{base: slash(absBase), ignore: map[string]bool{}}
	for _, p := range sel.Ignore {
		if abs, err := filepath.Abs(p); err == nil {
			r.ignore[slash(abs)] = true
		}
	}
	rules := sel.Rules
	if len(rules) == 0 {
		rules = utils.GlobRules{{Source: ".", Dest: "./"}}
	}

	selected := map[string]LocalFile{}
	for _, rule := range rules {
		if rule.Exclude {
			found, err := r.matches(rule.Source, false)
			if err != nil {
				return nil, err
			}
			gone := make(map[string]bool, len(found))
			for _, m := range found {
				gone[m.Abs] = true
			}
			for p, f := range selected {
				if gone[slash(f.Abs)] {
					delete(selected, p)
				}
			}
			continue
		}
		r.counting = true
		found, err := r.matches(rule.Source, true)
		r.counting = false
		if err != nil {
			return nil, err
		}
		if len(found) == 0 {
			return nil, fmt.Errorf("include %q matches no files", rule.Source)
		}
		for _, m := range found {
			p, err := destPath(rule, m)
			if err != nil {
				return nil, err
			}
			selected[p] = LocalFile{Abs: filepath.FromSlash(m.Abs), Path: p, Size: m.Size, ModTime: m.ModTime, Source: rule.Source}
		}
	}

	out := &Resolved{Skipped: r.skipped}
	if f, ok := selected["_redirects"]; ok {
		delete(selected, "_redirects") // it configures the version; it is not served
		data, err := os.ReadFile(f.Abs)
		if err != nil {
			return nil, fmt.Errorf("cannot read %s: %w", f.Abs, unwrapPathError(err))
		}
		out.Redirects, out.Warnings = ParseRedirects(string(data))
	}
	if !sel.NoHTMLToFolders {
		addHTMLToFolders(selected)
	}
	for _, f := range selected {
		if reason := ValidSitePath(f.Path); reason != "" {
			return nil, fmt.Errorf("%s: the path %q %s", f.Abs, f.Path, reason)
		}
		if err := readable(f.Abs); err != nil {
			return nil, err
		}
		out.Files = append(out.Files, f)
	}
	sort.Slice(out.Files, func(i, j int) bool { return out.Files[i].Path < out.Files[j].Path })
	if len(out.Files) == 0 {
		return nil, fmt.Errorf("no files to upload: everything selected was excluded")
	}
	return out, nil
}

// addHTMLToFolders gives every page its folder form, so a proxy serving the
// website finds it: /about.html is also /about/index.html. A copy is made only
// when that path is free, and index.html files are left as they are.
func addHTMLToFolders(files map[string]LocalFile) {
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		lower := strings.ToLower(p)
		var stem string
		switch {
		case strings.HasSuffix(lower, ".html"):
			stem = p[:len(p)-len(".html")]
		case strings.HasSuffix(lower, ".htm"):
			stem = p[:len(p)-len(".htm")]
		default:
			continue
		}
		target := stem + "/index.html"
		if strings.EqualFold(path.Base(stem), "index") {
			if strings.HasSuffix(lower, ".html") {
				continue // already an index.html
			}
			target = stem[:len(stem)-len(path.Base(stem))] + "index.html" // index.htm
		}
		if _, taken := files[target]; taken || blockedByFile(files, target) {
			continue
		}
		copied := files[p]
		copied.Path = target
		files[target] = copied
	}
}

// blockedByFile reports whether a file already sits where target needs a folder.
func blockedByFile(files map[string]LocalFile, target string) bool {
	for dir := path.Dir(target); dir != "." && dir != "/"; dir = path.Dir(dir) {
		if _, ok := files[dir]; ok {
			return true
		}
	}
	return false
}

// readable fails early, with the path, for a file the user cannot read.
func readable(abs string) error {
	f, err := os.Open(abs)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", abs, unwrapPathError(err))
	}
	return f.Close()
}

func unwrapPathError(err error) error {
	if pe, ok := err.(*fs.PathError); ok {
		return pe.Err
	}
	return err
}
