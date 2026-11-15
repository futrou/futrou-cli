package utils

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"golang.org/x/term"
)

// IsTerminal reports whether w is an interactive terminal.
func IsTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}

// HumanBytes writes a size as 1.5 MiB.
func HumanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

type transferFile struct {
	path       string
	size, sent int64
}

// TransferProgress shows files being sent. On an interactive terminal the batch
// in flight is drawn live, one line per file with its percentage, above a line
// with the total; otherwise a line is printed when a file starts and one when it
// is done, with the total so far. All methods are safe to call from several
// goroutines.
type TransferProgress struct {
	w           io.Writer
	interactive bool

	mu         sync.Mutex
	files      int
	done       int
	total      int64
	sent       int64
	active     []*transferFile
	drawnLines int
	lastDraw   time.Time
}

// NewTransferProgress shows transfers on w; interactive draws the live batch.
func NewTransferProgress(w io.Writer, interactive bool) *TransferProgress {
	return &TransferProgress{w: w, interactive: interactive}
}

const maxPathWidth = 56

func shorten(path string) string {
	if len(path) <= maxPathWidth {
		return path
	}
	return "…" + path[len(path)-maxPathWidth+1:]
}

func percent(sent, total int64) int {
	if total <= 0 {
		return 100
	}
	if sent > total {
		sent = total
	}
	return int(sent * 100 / total)
}

func (d *TransferProgress) totalLine() string {
	pct := percent(d.sent, d.total)
	if d.total == 0 && d.files > 0 {
		pct = d.done * 100 / d.files
	}
	return fmt.Sprintf("total %3d%%  %d/%d files  %s / %s", pct, d.done, d.files, HumanBytes(d.sent), HumanBytes(d.total))
}

// clear erases the live block drawn last.
func (d *TransferProgress) clear() {
	if d.drawnLines > 0 {
		fmt.Fprintf(d.w, "\033[%dA\r\033[J", d.drawnLines)
		d.drawnLines = 0
	}
}

func (d *TransferProgress) draw() {
	d.clear()
	if d.files == 0 {
		return
	}
	for _, f := range d.active {
		fmt.Fprintf(d.w, "  ↑ %-*s %3d%%\n", maxPathWidth, shorten(f.path), percent(f.sent, f.size))
	}
	fmt.Fprintf(d.w, "  %s\n", d.totalLine())
	d.drawnLines = len(d.active) + 1
	d.lastDraw = time.Now()
}

// Message prints a line that is not a file.
func (d *TransferProgress) Message(text string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.interactive {
		d.clear()
		fmt.Fprintln(d.w, text)
		if len(d.active) > 0 {
			d.draw()
		}
		return
	}
	fmt.Fprintln(d.w, text)
}

// Begin says how many files and bytes will be sent.
func (d *TransferProgress) Begin(files int, bytes int64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.files, d.total, d.done, d.sent, d.active = files, bytes, 0, 0, nil
	if files > 0 && !d.interactive {
		fmt.Fprintf(d.w, "uploading %d file(s), %s\n", files, HumanBytes(bytes))
	}
}

// FileStart is called when a file begins to be sent.
func (d *TransferProgress) FileStart(path string, size int64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.interactive {
		fmt.Fprintf(d.w, "uploading %s\n", path)
		return
	}
	d.active = append(d.active, &transferFile{path: path, size: size})
	d.draw()
}

// FileBytes reports bytes of a file just sent.
func (d *TransferProgress) FileBytes(path string, n int64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.sent += n
	for _, f := range d.active {
		if f.path == path {
			f.sent += n
		}
	}
	if d.interactive && time.Since(d.lastDraw) > 100*time.Millisecond {
		d.draw()
	}
}

// FileDone is called when a file was sent.
func (d *TransferProgress) FileDone(path string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.done++
	if !d.interactive {
		fmt.Fprintf(d.w, "uploaded %s: %d/%d files, %d%% (%s / %s)\n", path, d.done, d.files, percent(d.sent, d.total), HumanBytes(d.sent), HumanBytes(d.total))
		return
	}
	for i, f := range d.active {
		if f.path == path {
			d.sent += f.size - f.sent // a file that sent less than its size still counts as whole
			d.active = append(d.active[:i], d.active[i+1:]...)
			break
		}
	}
	d.clear()
	fmt.Fprintf(d.w, "  ✓ %s\n", shorten(path))
	d.draw()
}

// End is called when all files were sent, or the transfer stopped.
func (d *TransferProgress) End() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.interactive {
		d.clear()
		if d.files > 0 {
			fmt.Fprintf(d.w, "  %s\n", d.totalLine())
		}
	}
	d.active = nil
}
