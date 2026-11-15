package statico

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"futrou-cli/src/services"
	"futrou-cli/src/utils"
)

// Workers is how many requests run at once.
const Workers = 8

const (
	// An archive holds at most this many files and bytes (a bigger file is an archive of its own).
	batchFiles = 2000
	batchBytes = 32 << 20
)

func decode(from, to interface{}) error {
	data, err := json.Marshal(from)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, to)
}

// parallel runs fn for every index below n on up to Workers goroutines and
// returns the first error; work already started finishes, nothing new starts.
func parallel(n int, fn func(i int) error) error {
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		first   error
		jobs    = make(chan int)
		stopped = func() bool { mu.Lock(); defer mu.Unlock(); return first != nil }
	)
	for w := 0; w < Workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				if stopped() {
					continue
				}
				if err := fn(i); err != nil {
					mu.Lock()
					if first == nil {
						first = err
					}
					mu.Unlock()
				}
			}
		}()
	}
	for i := 0; i < n; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return first
}

// Progress receives the events of an upload, from several goroutines at once.
// utils.TransferProgress is one that draws them.
type Progress interface {
	// Message is a step of the upload that is not a file.
	Message(text string)
	// Begin says how many files and bytes will be sent.
	Begin(files int, bytes int64)
	FileStart(path string, size int64)
	// FileBytes reports bytes of a file just sent.
	FileBytes(path string, n int64)
	FileDone(path string)
	// End is called when all files were sent, or the upload stopped.
	End()
}

type silent struct{}

func (silent) Message(string)          {}
func (silent) Begin(int, int64)        {}
func (silent) FileStart(string, int64) {}
func (silent) FileBytes(string, int64) {}
func (silent) FileDone(string)         {}
func (silent) End()                    {}

// Silent returns the Progress that reports nothing (JSON output).
func Silent() Progress { return silent{} }

// VersionOptions is how a new StaticoVersion answers requests.
type VersionOptions struct {
	// Name of the version; the server names it by time when empty.
	Name string
	// Empty starts from no files instead of a copy of the served version.
	Empty          bool
	DefaultFile    string
	DefaultStatus  int
	FallbackFile   string
	FallbackStatus int
	// Redirects are the rules of the node, from the config and from a _redirects file.
	Redirects []map[string]interface{}
}

// answers are the settings a version keeps: how it answers requests.
func (o VersionOptions) answers() map[string]interface{} {
	body := map[string]interface{}{}
	if o.DefaultFile != "" {
		body["defaultFile"] = o.DefaultFile
	}
	if o.DefaultStatus != 0 {
		body["defaultStatus"] = o.DefaultStatus
	}
	if o.FallbackFile != "" {
		body["fallbackFile"] = o.FallbackFile
	}
	if o.FallbackStatus != 0 {
		body["fallbackStatus"] = o.FallbackStatus
	}
	if len(o.Redirects) > 0 {
		body["redirects"] = o.Redirects
	}
	return body
}

// create is the body that creates the version; its redirects are set afterwards.
func (o VersionOptions) create() map[string]interface{} {
	body := o.answers()
	delete(body, "redirects")
	if o.Name != "" {
		body["name"] = o.Name
	}
	if o.Empty {
		body["from"] = "empty"
	}
	return body
}

// UploadOptions control Upload.
type UploadOptions struct {
	VersionOptions
	// NoActivate leaves the new version ready but not served.
	NoActivate bool
	// Progress is told about each step; nil is silent.
	Progress Progress
	// Base is the folder of the config, whose .futrou holds the cache and the
	// staged files. Empty means no cache and no staging.
	Base string
	// Stage sends copies of the changed files made in Base/.futrou (see Stage)
	// instead of the files where they are.
	Stage bool
}

func (o UploadOptions) progress() Progress {
	if o.Progress == nil {
		return silent{}
	}
	return o.Progress
}

// Compare plans an upload against the served version without changing
// anything. A website with no served version has every file to upload.
func Compare(c Client, st Statico, local []LocalFile, opts UploadOptions) (Plan, error) {
	if opts.Empty {
		return Prepare(local, nil, opts.Base)
	}
	active, err := activeStaticoVersion(c, st)
	if err != nil || active == nil {
		if err != nil {
			return Plan{}, err
		}
		return Prepare(local, nil, opts.Base)
	}
	remote, err := Hashes(c, st, *active)
	if err != nil {
		return Plan{}, err
	}
	return Prepare(local, remote, opts.Base)
}

// AnswersDiffer reports whether the served version answers differently from
// what opts ask for (only the settings opts give are compared), so a changed
// default file or redirect list is deployed even when the files are the same.
func AnswersDiffer(c Client, st Statico, opts VersionOptions) (bool, error) {
	want := opts.answers()
	if len(want) == 0 {
		return false, nil
	}
	var active map[string]interface{}
	status, err := c.RequestInto("GET", st.Path()+"/versions/active", nil, &active)
	if versionMissing(status, err) {
		return false, nil
	}
	if err := failed("reading the active version", status, err); err != nil {
		return false, err
	}
	for key, value := range want {
		got := active[key]
		if key == "defaultFile" || key == "fallbackFile" {
			value = "/" + strings.TrimPrefix(value.(string), "/")
			if text, ok := got.(string); ok {
				got = "/" + strings.TrimPrefix(text, "/")
			}
		}
		if !sameJSON(value, got) {
			return true, nil
		}
	}
	return false, nil
}

func sameJSON(a, b interface{}) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

// activeStaticoVersion is the served version, or nil when nothing was deployed yet.
func activeStaticoVersion(c Client, st Statico) (*StaticoVersion, error) {
	var v StaticoVersion
	status, err := c.RequestInto("GET", st.Path()+"/versions/active", nil, &v)
	if versionMissing(status, err) {
		return nil, nil
	}
	if err := failed("reading the active version", status, err); err != nil {
		return nil, err
	}
	if v.Name == "" {
		return nil, nil
	}
	return &v, nil
}

// Diff is what an upload of a selection would change; the deploy plan and the
// upload command both use it.
type Diff struct {
	// Resolved is the selection, with the redirects of a _redirects file.
	Resolved *Resolved
	Plan     Plan
	// AnswersChanged is true when the served version answers differently
	// (default and fallback file, redirects) from what the selection asks for.
	AnswersChanged bool
	// New is true when there is no website yet, so everything is new.
	New     bool
	Options UploadOptions
}

// Changed reports whether an upload would create a new version.
func (d *Diff) Changed() bool { return d.New || !d.Plan.Empty() || d.AnswersChanged }

// Describe is a short summary of the changes, for a plan.
func (d *Diff) Describe() string {
	p := d.Plan
	text := fmt.Sprintf("%d file(s) selected: %d to upload (%s), %d unchanged, %d to delete", len(d.Resolved.Files), len(p.Upload), utils.HumanBytes(p.Bytes()), p.Unchanged, p.Deleted)
	if p.UnchangedDirs > 0 {
		text += fmt.Sprintf("; %d unchanged folder(s) skipped", p.UnchangedDirs)
	}
	if d.Resolved.Skipped > 0 {
		text += fmt.Sprintf("; %d hidden or .map file(s) left out", d.Resolved.Skipped)
	}
	return text
}

// withFileRedirects adds the rules of a _redirects file after the declared ones.
func withFileRedirects(opts UploadOptions, resolved *Resolved) UploadOptions {
	opts.Redirects = MergeRedirects(opts.Redirects, resolved.Redirects)
	return opts
}

// Check resolves the selection and compares it with the served version of st
// (nil when the website does not exist yet), changing nothing. It reads only
// the files that could be unchanged.
func Check(c Client, st *Statico, base string, sel Selection, opts UploadOptions) (*Diff, error) {
	resolved, err := Resolve(base, sel)
	if err != nil {
		return nil, err
	}
	opts = withFileRedirects(opts, resolved)
	opts.Base = base
	diff := &Diff{Resolved: resolved, Options: opts, New: st == nil}
	if st == nil {
		diff.Plan, err = Prepare(resolved.Files, nil, base)
		return diff, err
	}
	if diff.Plan, err = Compare(c, *st, resolved.Files, opts); err != nil {
		return nil, err
	}
	diff.AnswersChanged, err = AnswersDiffer(c, *st, opts.VersionOptions)
	return diff, err
}

// Sync uploads the selection to st as a new StaticoVersion, from copies of the
// changed files staged in <base>/.futrou/statico/<name>/<version> (removed when
// the upload worked, kept for inspection when it failed). It does nothing when
// the served version already has these files and answers.
func Sync(c Client, st Statico, base string, sel Selection, opts UploadOptions) (*Result, error) {
	resolved, err := Resolve(base, sel)
	if err != nil {
		return nil, err
	}
	for _, warning := range resolved.Warnings {
		opts.progress().Message("warning: " + warning)
	}
	opts = withFileRedirects(opts, resolved)
	opts.Base, opts.Stage = base, true
	return Upload(c, st, resolved.Files, opts)
}

// Result describes a finished upload.
type Result struct {
	Version   string `json:"version"`
	Uploaded  int    `json:"uploaded"`
	Deleted   int    `json:"deleted"`
	Unchanged int    `json:"unchanged"`
	Bytes     int64  `json:"bytes"`
	Activated bool   `json:"activated"`
	// Skipped is true when the served version already had these files.
	Skipped bool `json:"skipped"`
	// URL is where the website is served; empty when the API did not say.
	URL string `json:"url,omitempty"`
}

// Message is the success line of an upload, the same for `futrou statico upload`
// and `futrou deploy`.
func (r Result) Message(name string) string {
	where := ""
	if r.URL != "" {
		where = " at " + r.URL
	}
	switch {
	case r.Skipped:
		return fmt.Sprintf("✓ %s is already up to date%s", name, where)
	case r.Activated:
		return fmt.Sprintf("✓ %s: version %s is live%s (%d uploaded, %d deleted, %d unchanged)", name, r.Version, where, r.Uploaded, r.Deleted, r.Unchanged)
	}
	return fmt.Sprintf("✓ %s: version %s uploaded, not activated (%s)", name, r.Version, ActivateHint(name, r.Version))
}

// ActivateHint is the command that serves a version, as a message shows it.
func ActivateHint(name, version string) string {
	return fmt.Sprintf("futrou statico version activate --version %s %s", version, name)
}

// Upload makes a new StaticoVersion out of the local files. The new version
// starts as a copy of the served one, so only what differs is sent: new and
// changed files (small ones together in archives, big ones one by one), and what
// is gone is deleted (a folder that is gone in one step). Then its redirects are
// set and it is activated. Until the activation the website serves its old
// files, so a failure halfway changes nothing; the version is then marked failed.
func Upload(c Client, st Statico, local []LocalFile, opts UploadOptions) (*Result, error) {
	if len(local) == 0 {
		return nil, fmt.Errorf("nothing to upload")
	}
	progress := opts.progress()
	plan, err := Compare(c, st, local, opts)
	if err != nil {
		return nil, err
	}
	differ, err := AnswersDiffer(c, st, opts.VersionOptions)
	if err != nil {
		return nil, err
	}
	if plan.Empty() && !differ {
		progress.Message(fmt.Sprintf("%s is already up to date (%d files)", st.Name, plan.Unchanged))
		return &Result{Unchanged: plan.Unchanged, Skipped: true, URL: st.URL()}, nil
	}

	var version StaticoVersion
	status, err := c.RequestInto("POST", st.Path()+"/versions", opts.create(), &version)
	if err := failed("creating a version", status, err); err != nil {
		return nil, err
	}
	if version.Name == "" {
		return nil, fmt.Errorf("creating a version: the API returned no version name")
	}
	versionPath := st.Path() + "/versions/" + url.PathEscape(version.Name)
	progress.Message("Created version " + version.Name)

	var staged *Staged
	fail := func(cause error) (*Result, error) {
		if staged != nil {
			cause = fmt.Errorf("%w (the staged files are in %s)", cause, staged.Dir)
		}
		if errors.Is(cause, context.Canceled) {
			cause = errors.New("cancelled")
		}
		report := map[string]interface{}{"status": "failed", "error": truncate(cause.Error(), 1000)}
		var reporter Client = c
		if d, ok := c.(interface{ Detached() *services.ApiClient }); ok {
			reporter = d.Detached() // the cancel that stopped the upload must not stop this report
		}
		_, _ = reporter.RequestInto("PATCH", versionPath, report, &struct{}{})
		return nil, fmt.Errorf("uploading to version %s: %w", version.Name, cause)
	}
	if _, err := c.RequestInto("PATCH", versionPath, map[string]interface{}{"status": "uploading"}, &struct{}{}); err != nil {
		return fail(err)
	}
	// the redirects of the version are set once it exists, before anything is sent
	if len(opts.Redirects) > 0 {
		status, err := c.RequestInto("PATCH", versionPath, map[string]interface{}{"redirects": opts.Redirects}, &struct{}{})
		if err := failed("setting the redirects", status, err); err != nil {
			return fail(err)
		}
	}

	// the version's own files are those compared above, unless a draft was replaced
	remote, err := Hashes(c, st, version)
	if err != nil {
		return fail(err)
	}
	if plan, err = Prepare(local, remote, opts.Base); err != nil {
		return fail(err)
	}
	send := plan.Upload
	if opts.Stage && opts.Base != "" && len(send) > 0 {
		if staged, err = Stage(opts.Base, st.Name, version.Name, send); err != nil {
			return fail(err)
		}
		send = staged.Files
	}
	storage := version.storage(st)

	// what is gone first: a path that changes between a file and a folder must be free
	deletions := append(append([]string{}, plan.DeleteDirs...), plan.Delete...)
	err = parallel(len(deletions), func(i int) error {
		path := filePath(storage, version.Name, deletions[i])
		if i < len(plan.DeleteDirs) {
			path += "?recursive=true"
		}
		status, err := c.RequestInto("DELETE", path, nil, &struct{}{})
		return failed("deleting "+deletions[i], status, err)
	})
	if err != nil {
		return fail(err)
	}
	for _, p := range deletions {
		progress.Message("deleted " + p)
	}

	var total int64
	for _, f := range send {
		total += f.Size
	}
	progress.Begin(len(send), total)
	err = sendAll(c, storage, version.Name, send, progress)
	// An interrupted upload continues where it stopped: what the version already
	// holds is read again (a file cut short differs and is sent again), and only
	// the rest is sent, up to Retries times. A cancel is never retried.
	for attempt := 1; err != nil && attempt <= Retries && !errors.Is(err, context.Canceled); attempt++ {
		progress.End()
		time.Sleep(time.Duration(attempt) * RetryDelay)
		remote, herr := Hashes(c, st, version)
		if herr != nil {
			progress.Begin(0, 0)
			err = fmt.Errorf("%w (and the files already uploaded could not be listed: %v)", err, herr)
			break
		}
		again, perr := Prepare(local, remote, opts.Base)
		if perr != nil {
			progress.Begin(0, 0)
			err = perr
			break
		}
		send = remaining(again.Upload, staged, send)
		progress.Message(fmt.Sprintf("the upload was interrupted (%v): continuing with the %d files left (retry %d of %d)", err, len(send), attempt, Retries))
		total = 0
		for _, f := range send {
			total += f.Size
		}
		progress.Begin(len(send), total)
		err = sendAll(c, storage, version.Name, send, progress)
		if len(send) == 0 {
			err = nil
		}
	}
	progress.End()
	if err != nil {
		return fail(err)
	}

	result := &Result{Version: version.Name, Uploaded: len(send), Deleted: plan.Deleted, Unchanged: plan.Unchanged, Bytes: total, URL: st.URL()}
	if opts.NoActivate {
		result.URL = ""
		staged.Remove()
		return result, nil
	}
	status, err = c.RequestInto("POST", versionPath+"/activate", nil, &struct{}{})
	if err := failed("activating version "+version.Name, status, err); err != nil {
		return nil, err // the API marks a refused activation failed itself
	}
	result.Activated = true
	staged.Remove()
	return result, nil
}

// Retries is how many times an interrupted upload continues before it gives up,
// and RetryDelay the wait before the first retry (the second waits twice as long).
var (
	Retries    = 2
	RetryDelay = time.Second
)

// sendAll sends the files in compressed archives, several at once.
func sendAll(c Client, storage, version string, files []HashedFile, progress Progress) error {
	units := batches(files)
	return parallel(len(units), func(i int) error { return sendUnit(c, storage, version, units[i], progress) })
}

// remaining are the files of todo that are still to be sent: the staged copy of
// each when the files were staged (what is sent is the copy), else the file itself.
func remaining(todo []HashedFile, staged *Staged, previous []HashedFile) []HashedFile {
	copies := map[string]HashedFile{}
	for _, f := range previous {
		copies[f.Path] = f
	}
	var out []HashedFile
	for _, f := range todo {
		if sent, ok := copies[f.Path]; ok && staged != nil {
			out = append(out, sent)
		} else {
			out = append(out, f)
		}
	}
	return out
}

// batches groups files for sending, in order, up to batchFiles files and
// batchBytes bytes each; a file bigger than batchBytes is a batch of its own.
// Every batch is sent as one compressed archive.
func batches(files []HashedFile) [][]HashedFile {
	var units [][]HashedFile
	var current []HashedFile
	var bytes int64
	flush := func() {
		if len(current) > 0 {
			units = append(units, current)
			current, bytes = nil, 0
		}
	}
	for _, f := range files {
		if len(current) >= batchFiles || bytes+f.Size > batchBytes {
			flush()
		}
		current = append(current, f)
		bytes += f.Size
	}
	flush()
	return units
}

// noArchive holds the clients whose API refused an archive upload, so the rest
// of the upload sends files one by one at once.
var noArchive sync.Map

// archiveRefused reports whether the API rejected the archive route itself (an
// API whose Content-Type check does not exempt it), not the files in it.
func archiveRefused(err error) bool {
	if err == nil {
		return false
	}
	text := err.Error()
	return strings.Contains(text, "INVALID_CONTENT_TYPE") || strings.Contains(text, "Content-Type must be application/json")
}

// sendUnit sends a batch of files as one compressed archive (a .tar.gz), never
// as bare files. Only an API that does not accept archives yet (see
// archiveRefused) gets the files one by one, with a notice.
func sendUnit(c Client, storage, version string, files []HashedFile, progress Progress) error {
	if _, refused := noArchive.Load(c); !refused {
		held := &heldProgress{}
		err := putArchive(c, storage, version, files, held)
		if err == nil {
			held.replay(progress)
			return nil
		}
		if errors.Is(err, context.Canceled) {
			return err // a cancel is not a failed archive
		}
		if archiveRefused(err) {
			// the API will not take archives at all: not tried again in this run
			if _, already := noArchive.LoadOrStore(c, true); !already {
				progress.Message("warning: the API does not accept archives: sending the files one by one, uncompressed")
			}
		} else {
			progress.Message(fmt.Sprintf("warning: uploading %d files as an archive failed (%v): sending them one by one", len(files), err))
		}
	}
	for _, file := range files {
		if err := putFile(c, storage, version, file, progress); err != nil {
			return err
		}
	}
	return nil
}

// heldProgress remembers what an archive upload reported, so that it is shown
// only when the archive was accepted (otherwise the files are sent again).
type heldProgress struct {
	mu     sync.Mutex
	events []func(Progress)
}

func (h *heldProgress) add(f func(Progress)) {
	h.mu.Lock()
	h.events = append(h.events, f)
	h.mu.Unlock()
}
func (h *heldProgress) Message(text string)          { h.add(func(p Progress) { p.Message(text) }) }
func (h *heldProgress) Begin(files int, bytes int64) {}
func (h *heldProgress) FileStart(path string, size int64) {
	h.add(func(p Progress) { p.FileStart(path, size) })
}
func (h *heldProgress) FileBytes(path string, n int64) {
	h.add(func(p Progress) { p.FileBytes(path, n) })
}
func (h *heldProgress) FileDone(path string) { h.add(func(p Progress) { p.FileDone(path) }) }
func (h *heldProgress) End()                 {}

func (h *heldProgress) replay(p Progress) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, event := range h.events {
		event(p)
	}
}

func putFile(c Client, storage, version string, file HashedFile, progress Progress) error {
	f, err := os.Open(file.Abs)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", file.Abs, unwrapPathError(err))
	}
	defer f.Close()
	progress.FileStart(file.Path, file.Size)
	reader := &countingReader{r: f, count: func(n int64) { progress.FileBytes(file.Path, n) }}
	_, status, err := c.SendBody("PUT", filePath(storage, version, file.Path), "application/octet-stream", reader)
	if err := failed("uploading "+file.Path, status, err); err != nil {
		return err
	}
	progress.FileDone(file.Path)
	return nil
}

// putArchive streams the files as one tar.gz to the storage's archive route,
// which unpacks it below the version's folder in a single request.
func putArchive(c Client, storage, version string, files []HashedFile, progress Progress) error {
	reader, writer := io.Pipe()
	go func() { writer.CloseWithError(writeArchive(writer, files, progress)) }()
	path := "/v2/storages/" + url.PathEscape(storage) + "/archive?path=" + url.QueryEscape(version)
	_, status, err := c.SendBody("POST", path, "application/gzip", reader)
	reader.CloseWithError(io.ErrClosedPipe) // lets the writer stop if the request ended early
	return failed(fmt.Sprintf("uploading %d files as an archive", len(files)), status, err)
}

func writeArchive(w io.Writer, files []HashedFile, progress Progress) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	for _, file := range files {
		if err := writeEntry(tw, file, progress); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

func writeEntry(tw *tar.Writer, file HashedFile, progress Progress) error {
	f, err := os.Open(file.Abs)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", file.Abs, unwrapPathError(err))
	}
	defer f.Close()
	progress.FileStart(file.Path, file.Size)
	header := &tar.Header{Typeflag: tar.TypeReg, Name: file.Path, Mode: 0o644, Size: file.Size, ModTime: file.ModTime}
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	reader := &countingReader{r: f, count: func(n int64) { progress.FileBytes(file.Path, n) }}
	if n, err := io.Copy(tw, reader); err != nil {
		if errors.Is(err, tar.ErrWriteTooLong) {
			return fmt.Errorf("%s changed while it was being uploaded", file.Path)
		}
		return fmt.Errorf("reading %s: %w", file.Abs, err)
	} else if n != file.Size {
		return fmt.Errorf("%s changed while it was being uploaded", file.Path)
	}
	progress.FileDone(file.Path)
	return nil
}

// countingReader reports how many bytes of a file have been read for sending.
type countingReader struct {
	r     io.Reader
	count func(int64)
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 {
		c.count(int64(n))
	}
	return n, err
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
