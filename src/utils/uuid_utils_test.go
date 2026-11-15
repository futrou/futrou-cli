package utils

import "testing"

func TestIsUuid(t *testing.T) {
	for in, want := range map[string]bool{
		"11111111-1111-1111-1111-111111111111":  true,
		"019ec1d7-e8d3-718d-9bb2-5aef81dc7911":  true,
		"ABCDEF01-2345-6789-abcd-ef0123456789":  true,
		"default":                               false,
		"my-workspace":                          false,
		"":                                      false,
		"11111111-1111-1111-1111-11111111111":   false, // one char short
		"11111111111111111111111111111111":      false, // no dashes
		"11111111-1111-1111-1111-11111111111g":  false,
		" 11111111-1111-1111-1111-111111111111": false,
	} {
		if got := IsUuid(in); got != want {
			t.Errorf("IsUuid(%q) = %v, want %v", in, got, want)
		}
	}
}
