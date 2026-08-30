package utils

import "testing"

func TestGlobMatch(t *testing.T) {
	tests := []struct {
		pattern string
		name    string
		want    bool
	}{
		{"*.txt", "a.txt", true},
		{"*.txt", "a.log", false},
		{"*.txt", "dir/a.txt", false},
		{"**/*.txt", "a.txt", true},
		{"**/*.txt", "dir/a.txt", true},
		{"**/*.txt", "dir/sub/a.txt", true},
		{"**/*.txt", "dir/sub/a.log", false},
		{"images/*", "images/cat.png", true},
		{"images/*", "images/sub/cat.png", false},
		{"images/**", "images/sub/cat.png", true},
		{"images/**", "images/cat.png", true},
		{"images/**", "other/cat.png", false},
		{"a?c.txt", "abc.txt", true},
		{"a?c.txt", "ac.txt", false},
		{"exact/path.txt", "exact/path.txt", true},
		{"exact/path.txt", "exact/other.txt", false},
		{"", "", true},
		{"", "a", false},
	}
	for _, tt := range tests {
		if got := GlobMatch(tt.pattern, tt.name); got != tt.want {
			t.Errorf("GlobMatch(%q, %q) = %v, want %v", tt.pattern, tt.name, got, tt.want)
		}
	}
}

func TestHasGlobMeta(t *testing.T) {
	if !HasGlobMeta("*.txt") {
		t.Error("expected *.txt to have glob meta")
	}
	if !HasGlobMeta("a?.txt") {
		t.Error("expected a?.txt to have glob meta")
	}
	if HasGlobMeta("plain/path.txt") {
		t.Error("expected plain path to have no glob meta")
	}
}
