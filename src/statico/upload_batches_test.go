package statico

import (
	"fmt"
	"testing"
)

func TestBatchesGroupFilesForSending(t *testing.T) {
	small := func(n int) []HashedFile {
		out := make([]HashedFile, n)
		for i := range out {
			out[i] = HashedFile{LocalFile: LocalFile{Path: fmt.Sprintf("f%d", i), Size: 10}}
		}
		return out
	}
	if got := batches(small(0)); len(got) != 0 {
		t.Fatalf("no files, no batches: %v", got)
	}
	if got := batches(small(5)); len(got) != 1 || len(got[0]) != 5 {
		t.Fatalf("5 small files are one batch: %v", got)
	}
	if got := batches(small(4500)); len(got) != 3 || len(got[0]) != 2000 || len(got[2]) != 500 {
		t.Fatalf("4500 small files are 3 batches: %d", len(got))
	}
	// a 5 MiB file shares an archive with the small ones around it
	mixed := append(small(3), HashedFile{LocalFile: LocalFile{Path: "big", Size: 5 << 20}})
	mixed = append(mixed, small(2)...)
	if got := batches(mixed); len(got) != 1 || len(got[0]) != 6 {
		t.Fatalf("one archive of 6 files: %v", got)
	}
	// a file above the byte limit is an archive of its own
	huge := append(small(2), HashedFile{LocalFile: LocalFile{Path: "huge", Size: batchBytes + 1}})
	huge = append(huge, small(1)...)
	got := batches(huge)
	if len(got) != 3 || len(got[1]) != 1 || got[1][0].Path != "huge" {
		t.Fatalf("a huge file is alone: %v", got)
	}
}
