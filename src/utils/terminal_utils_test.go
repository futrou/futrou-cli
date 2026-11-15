package utils

import (
	"bytes"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestPlainProgressPrintsAFilePerLineWithTheTotal(t *testing.T) {
	var out bytes.Buffer
	p := NewTransferProgress(&out, false)
	p.Message("Created version v1")
	p.Begin(2, 200)
	p.FileStart("a.txt", 100)
	p.FileBytes("a.txt", 100)
	p.FileDone("a.txt")
	p.FileStart("b.txt", 100)
	p.FileBytes("b.txt", 100)
	p.FileDone("b.txt")
	p.End()
	text := out.String()
	for _, want := range []string{
		"Created version v1", "uploading 2 file(s), 200 B", "uploading a.txt", "uploaded a.txt: 1/2 files, 50%", "uploading b.txt", "uploaded b.txt: 2/2 files, 100%",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("output lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "\033[") {
		t.Errorf("a plain output must have no escape codes: %q", text)
	}
}

func TestInteractiveProgressDrawsBatchesWithPercentagesAndATotal(t *testing.T) {
	var out bytes.Buffer
	p := NewTransferProgress(&out, true)
	p.Begin(2, 200)
	p.FileStart("a.txt", 100)
	p.FileStart("b.txt", 100)
	p.FileBytes("a.txt", 50)
	p.FileDone("a.txt")
	p.FileBytes("b.txt", 100)
	p.FileDone("b.txt")
	p.End()
	text := out.String()
	for _, want := range []string{"↑ a.txt", "↑ b.txt", "✓ a.txt", "✓ b.txt", "total", "2/2 files", "\033["} {
		if !strings.Contains(text, want) {
			t.Errorf("output lacks %q:\n%q", want, text)
		}
	}
	if !strings.Contains(text, "100%") {
		t.Errorf("the total must reach 100%%: %q", text)
	}
}

func TestProgressWithNothingToSendPrintsNoTotal(t *testing.T) {
	var out bytes.Buffer
	p := NewTransferProgress(&out, true)
	p.Begin(0, 0)
	p.End()
	if out.Len() != 0 {
		t.Fatalf("output = %q", out.String())
	}
	out.Reset()
	p = NewTransferProgress(&out, false)
	p.Begin(0, 0)
	p.End()
	if out.Len() != 0 {
		t.Fatalf("plain output = %q", out.String())
	}
}

func TestProgressIsSafeFromManyGoroutines(t *testing.T) {
	for _, interactive := range []bool{true, false} {
		var out bytes.Buffer
		p := NewTransferProgress(&out, interactive)
		p.Begin(40, 4000)
		var wg sync.WaitGroup
		for i := 0; i < 40; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				name := string(rune('a'+i%26)) + string(rune('a'+i/26)) + ".txt"
				p.FileStart(name, 100)
				for j := 0; j < 10; j++ {
					p.FileBytes(name, 10)
				}
				p.FileDone(name)
			}(i)
		}
		wg.Wait()
		p.End()
		if !strings.Contains(out.String(), "40/40") {
			t.Errorf("interactive=%v: the total must reach 40/40: %q", interactive, out.String())
		}
	}
}

func TestProgressShortensLongPaths(t *testing.T) {
	var out bytes.Buffer
	p := NewTransferProgress(&out, true)
	long := strings.Repeat("dir/", 40) + "file.txt"
	p.Begin(1, 10)
	p.FileStart(long, 10)
	p.End()
	if strings.Contains(out.String(), long) || !strings.Contains(out.String(), "file.txt") || !strings.Contains(out.String(), "…") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestProgressZeroByteFileCountsAsDone(t *testing.T) {
	var out bytes.Buffer
	p := NewTransferProgress(&out, false)
	p.Begin(1, 0)
	p.FileStart("empty.txt", 0)
	p.FileDone("empty.txt")
	if !strings.Contains(out.String(), "1/1 files, 100%") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestIsTerminalAndHumanBytes(t *testing.T) {
	if IsTerminal(&bytes.Buffer{}) {
		t.Error("a buffer is not a terminal")
	}
	f, err := os.CreateTemp(t.TempDir(), "x")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if IsTerminal(f) {
		t.Error("a file is not a terminal")
	}
	for n, want := range map[int64]string{0: "0 B", 1023: "1023 B", 1024: "1.0 KiB", 1536: "1.5 KiB", 5 << 20: "5.0 MiB", 3 << 30: "3.0 GiB"} {
		if got := HumanBytes(n); got != want {
			t.Errorf("HumanBytes(%d) = %q, want %q", n, got, want)
		}
	}
}
