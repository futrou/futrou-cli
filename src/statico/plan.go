package statico

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"futrou-cli/src/utils"
)

// HashedFile is a local file with its sha256; the hash is empty for a file known
// to differ from the remote one without having been read.
type HashedFile struct {
	LocalFile
	Hash string
}

func hashFile(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", fmt.Errorf("cannot read %s: %w", p, unwrapPathError(err))
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("reading %s: %w", p, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Plan is what an upload changes relative to the files it starts from, worked
// out by comparing the folders of both sides: a folder that is the same on both
// is skipped whole, a folder that is gone is deleted in one step, and a folder
// that is new is uploaded whole.
type Plan struct {
	// Upload are the new and changed files. Their Hash is empty when they are
	// known to differ without being read (a new file, or another size).
	Upload []HashedFile
	// Delete are single files that are gone; DeleteDirs are folders that are gone, with everything in them.
	Delete     []string
	DeleteDirs []string
	// Unchanged counts files that are the same, UnchangedDirs the folders skipped whole.
	Unchanged     int
	UnchangedDirs int
	// Deleted counts the files removed, those in deleted folders included.
	Deleted int
}

// Empty reports whether the website already has exactly these files.
func (p Plan) Empty() bool { return len(p.Upload) == 0 && len(p.Delete) == 0 && len(p.DeleteDirs) == 0 }

// Bytes is the size of the files to upload.
func (p Plan) Bytes() int64 {
	var n int64
	for _, f := range p.Upload {
		n += f.Size
	}
	return n
}

type leaf struct {
	size  int64
	hash  string
	local *HashedFile
}

// node is a folder of a version, local or remote.
type node struct {
	files map[string]leaf
	dirs  map[string]*node

	digested bool
	digest   string // empty when a file's hash is not known
	count    int    // files below
}

func newNode() *node { return &node{files: map[string]leaf{}, dirs: map[string]*node{}} }

func (n *node) insert(p string, l leaf) {
	parts := strings.Split(p, "/")
	cur := n
	for _, part := range parts[:len(parts)-1] {
		next, ok := cur.dirs[part]
		if !ok {
			next = newNode()
			cur.dirs[part] = next
		}
		cur = next
	}
	cur.files[parts[len(parts)-1]] = l
}

// sum is the digest of the folder (from the names, sizes and hashes below it, so
// equal digests mean equal contents) and the number of files in it. The digest is
// empty when some file below it has no hash yet: such a folder differs.
func (n *node) sum() (string, int) {
	if n.digested {
		return n.digest, n.count
	}
	n.digested = true
	known := true
	h := sha256.New()
	names := make([]string, 0, len(n.files))
	for name := range n.files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		l := n.files[name]
		n.count++
		if l.hash == "" {
			known = false
			continue
		}
		h.Write([]byte("f\x00" + name + "\x00" + strconv.FormatInt(l.size, 10) + "\x00" + l.hash + "\n"))
	}
	dirNames := make([]string, 0, len(n.dirs))
	for name := range n.dirs {
		dirNames = append(dirNames, name)
	}
	sort.Strings(dirNames)
	for _, name := range dirNames {
		digest, count := n.dirs[name].sum()
		n.count += count
		if digest == "" {
			known = false
			continue
		}
		h.Write([]byte("d\x00" + name + "\x00" + digest + "\n"))
	}
	if known {
		n.digest = hex.EncodeToString(h.Sum(nil))
	}
	return n.digest, n.count
}

func (n *node) collect(p *Plan) {
	for _, l := range n.files {
		p.Upload = append(p.Upload, *l.local)
	}
	for _, d := range n.dirs {
		d.collect(p)
	}
}

func join(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "/" + name
}

func diffNodes(l, r *node, prefix string, p *Plan) {
	if r == nil {
		l.collect(p) // a folder that is new: everything in it is new
		return
	}
	if ld, count := l.sum(); ld != "" {
		if rd, _ := r.sum(); ld == rd {
			p.Unchanged += count
			if prefix != "" {
				p.UnchangedDirs++
			}
			return
		}
	}
	for name, lf := range l.files {
		if rd, ok := r.dirs[name]; ok { // a folder becomes a file
			_, count := rd.sum()
			p.DeleteDirs = append(p.DeleteDirs, join(prefix, name))
			p.Deleted += count
		}
		if rf, ok := r.files[name]; ok && lf.hash != "" && rf.hash == lf.hash && rf.size == lf.size {
			p.Unchanged++
			continue
		}
		p.Upload = append(p.Upload, *lf.local)
	}
	for name := range r.files {
		if _, ok := l.files[name]; ok {
			continue
		}
		p.Delete = append(p.Delete, join(prefix, name)) // gone, or a file that becomes a folder
		p.Deleted++
	}
	for name, ld := range l.dirs {
		diffNodes(ld, r.dirs[name], join(prefix, name), p)
	}
	for name, rd := range r.dirs {
		if _, ok := l.dirs[name]; ok {
			continue
		}
		if _, becomesFile := l.files[name]; becomesFile {
			continue // deleted above
		}
		_, count := rd.sum()
		p.DeleteDirs = append(p.DeleteDirs, join(prefix, name))
		p.Deleted += count
	}
}

// MakePlan compares local files with those of a remote version, folder by
// folder. A local file with an empty Hash is known to differ. A version holds
// exactly the selected files, so what the remote has and local lacks is deleted.
func MakePlan(local []HashedFile, remote []RemoteFile) Plan {
	l, r := newNode(), newNode()
	for i := range local {
		l.insert(local[i].Path, leaf{size: local[i].Size, hash: local[i].Hash, local: &local[i]})
	}
	for _, f := range remote {
		r.insert(f.Path, leaf{size: f.Size, hash: f.Hash})
	}
	var plan Plan
	diffNodes(l, r, "", &plan)
	sort.Slice(plan.Upload, func(i, j int) bool { return plan.Upload[i].Path < plan.Upload[j].Path })
	sort.Strings(plan.Delete)
	sort.Strings(plan.DeleteDirs)
	return plan
}

// Prepare compares local files with a remote version's. A file that is new or
// has another size is known to differ without being read; only a file of the
// same size is hashed (from the cache when it did not change), which is what
// keeps an upload of a big, mostly unchanged site from reading all of it.
func Prepare(local []LocalFile, remote []RemoteFile, base string) (Plan, error) {
	remoteSize := make(map[string]int64, len(remote))
	for _, f := range remote {
		remoteSize[f.Path] = f.Size
	}
	cache := openHashCache(base)
	hashed := make([]HashedFile, len(local))
	err := parallel(len(local), func(i int) error {
		hashed[i] = HashedFile{LocalFile: local[i]}
		if size, ok := remoteSize[local[i].Path]; ok && size == local[i].Size {
			hash, err := cache.hash(local[i])
			if err != nil {
				return err
			}
			hashed[i].Hash = hash
		}
		return nil
	})
	cache.save()
	if err != nil {
		return Plan{}, err
	}
	return MakePlan(hashed, remote), nil
}

// A file whose modification time is this recent when it is hashed is not
// cached: it could still change within the resolution of the time stamp.
const cacheSettleTime = 2 * time.Second

type cacheEntry struct {
	Size  int64  `json:"s"`
	MTime int64  `json:"m"`
	Hash  string `json:"h"`
}

// hashCache remembers the sha256 of files by path, size and modification time,
// so a file that did not change is not read again on the next upload. It lives
// in <base>/.futrou/cache, which a run keeps while it clears the rest of .futrou. Only the files of the current run are written back, so
// it does not grow with files that are gone.
type hashCache struct {
	mu      sync.Mutex
	base    string
	path    string
	entries map[string]cacheEntry
	used    map[string]cacheEntry
}

func openHashCache(base string) *hashCache {
	c := &hashCache{base: base, path: filepath.Join(utils.CacheDir(base), "statico-hashes.json"), entries: map[string]cacheEntry{}, used: map[string]cacheEntry{}}
	if c.path == "" {
		return c
	}
	if data, err := os.ReadFile(c.path); err == nil {
		_ = json.Unmarshal(data, &c.entries) // a damaged cache is an empty one
	}
	return c
}

// hash returns the sha256 of f, from the cache when its size and time are unchanged.
func (c *hashCache) hash(f LocalFile) (string, error) {
	mtime := f.ModTime.UnixNano()
	c.mu.Lock()
	entry, ok := c.entries[f.Abs]
	c.mu.Unlock()
	if ok && entry.Size == f.Size && entry.MTime == mtime && entry.Hash != "" {
		c.mu.Lock()
		c.used[f.Abs] = entry
		c.mu.Unlock()
		return entry.Hash, nil
	}
	hash, err := hashFile(f.Abs)
	if err != nil {
		return "", err
	}
	if time.Since(f.ModTime) > cacheSettleTime {
		c.mu.Lock()
		c.used[f.Abs] = cacheEntry{Size: f.Size, MTime: mtime, Hash: hash}
		c.mu.Unlock()
	}
	return hash, nil
}

// save writes the entries used in this run. Failing to is not an error: the
// cache only saves time.
func (c *hashCache) save() {
	c.mu.Lock()
	data, err := json.Marshal(c.used)
	empty := len(c.used) == 0
	c.mu.Unlock()
	if err != nil || empty || c.base == "" {
		return // nothing worth keeping: no folder is made for it
	}
	if _, err := utils.MakeCacheDir(c.base); err != nil {
		return
	}
	utils.WriteFileAtomic(c.path, data)
}
