package statico

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"futrou-cli/src/constants"
	"futrou-cli/src/utils"
)

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

// folderName is a website's name as a folder name: characters a folder name
// should not have become "_", and a name that is only dots ("." or "..", which
// would climb out of the staging folder) becomes "_" too.
func folderName(name string) string {
	clean := unsafeName.ReplaceAllString(name, "_")
	if strings.Trim(clean, ".") == "" {
		clean = "_"
	}
	if clean == name {
		return clean
	}
	// a name that was changed gets a hash of the original, so that "my site" and
	// "my_site" never share a folder
	sum := sha256.Sum256([]byte(name))
	return clean + "-" + hex.EncodeToString(sum[:4])
}

func workDir(base string) string { return utils.BuildDir(base) }

// ClearWorkDir empties <base>/.futrou, the leftovers of earlier runs (the staged
// files of an upload that failed). The folder itself, its .gitignore and its cache stay; a
// missing .futrou is left missing.
func ClearWorkDir(base string) error {
	entries, err := os.ReadDir(workDir(base))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", workDir(base), unwrapPathError(err))
	}
	for _, entry := range entries {
		if entry.Name() == ".gitignore" || entry.Name() == filepath.Base(constants.CacheDir) {
			continue // the cache is kept from run to run
		}
		if err := os.RemoveAll(filepath.Join(workDir(base), entry.Name())); err != nil {
			return fmt.Errorf("cannot clear %s: %w", filepath.Join(workDir(base), entry.Name()), unwrapPathError(err))
		}
	}
	return nil
}

// Staged is a copy of the files an upload sends, in
// <base>/.futrou/statico/<name>/<version>. What is sent is the copy, so a file
// edited while the upload runs cannot make a half-new, half-old version.
type Staged struct {
	Dir   string
	Files []HashedFile
	base  string
}

// Stage copies the files, which are the ones that changed and nothing else, and
// hashes each as it is copied, so the hash is of what is sent. A .futrou folder
// made for it gets a .gitignore that ignores everything in it.
func Stage(base, name, version string, files []HashedFile) (*Staged, error) {
	if version == "" || version == "." || version == ".." || unsafeName.MatchString(version) {
		return nil, fmt.Errorf("the version name %q cannot be a folder name", version)
	}
	futrou := workDir(base)
	dir := filepath.Join(utils.StaticoDir(base), folderName(name), version)
	_, statErr := os.Stat(futrou)
	if err := os.RemoveAll(dir); err != nil { // a stale folder of that version
		return nil, fmt.Errorf("cannot replace %s: %w", dir, unwrapPathError(err))
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("cannot create %s: %w", dir, unwrapPathError(err))
	}
	if os.IsNotExist(statErr) {
		_ = os.WriteFile(filepath.Join(futrou, ".gitignore"), []byte("*\n"), 0o644)
	}
	staged := &Staged{Dir: dir, base: base}
	for _, f := range files {
		target := filepath.Join(dir, filepath.FromSlash(f.Path))
		if rel, err := filepath.Rel(dir, target); err != nil || rel == ".." || len(rel) > 2 && rel[:3] == ".."+string(filepath.Separator) {
			staged.Remove()
			return nil, fmt.Errorf("the path %q leaves the staging folder", f.Path)
		}
		hash, size, err := copyHashed(f.Abs, target)
		if err != nil {
			staged.Remove()
			return nil, err
		}
		f.Abs, f.Size, f.Hash = target, size, hash
		staged.Files = append(staged.Files, f)
	}
	return staged, nil
}

func copyHashed(from, to string) (string, int64, error) {
	in, err := os.Open(from)
	if err != nil {
		return "", 0, fmt.Errorf("cannot read %s: %w", from, unwrapPathError(err))
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(to), 0o700); err != nil {
		return "", 0, fmt.Errorf("cannot create %s: %w", filepath.Dir(to), unwrapPathError(err))
	}
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", 0, fmt.Errorf("cannot write %s: %w", to, unwrapPathError(err))
	}
	h := sha256.New()
	size, err := io.Copy(io.MultiWriter(out, h), in)
	if err != nil {
		out.Close()
		return "", 0, fmt.Errorf("copying %s: %w", from, err)
	}
	return hex.EncodeToString(h.Sum(nil)), size, out.Close()
}

// Remove deletes the staged files and the folders above them that are left
// empty, down to .futrou, which goes too when only its .gitignore remains.
func (s *Staged) Remove() {
	if s == nil || s.Dir == "" {
		return
	}
	_ = os.RemoveAll(s.Dir)
	parent := filepath.Dir(s.Dir)
	_ = os.Remove(parent) // the website's folder, when empty
	_ = os.Remove(filepath.Dir(parent))
	futrou := workDir(s.base)
	if entries, err := os.ReadDir(futrou); err == nil && len(entries) == 1 && entries[0].Name() == ".gitignore" {
		_ = os.RemoveAll(futrou) // only the .gitignore this package made
	}
}
