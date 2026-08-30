package utils

import "strings"

// GlobMatch reports whether name matches pattern using shell-style globs
// over '/'-separated segments. '*' matches within a single segment; '**'
// matches across any number of segments (including zero). '?' matches any
// single rune within a segment.
func GlobMatch(pattern, name string) bool {
	return globMatchSegments(splitSegments(pattern), splitSegments(name))
}

// HasGlobMeta reports whether pattern contains glob metacharacters, so
// callers can treat plain paths (no meta) as exact matches without walking
// a full remote/local listing.
func HasGlobMeta(pattern string) bool {
	return strings.ContainsAny(pattern, "*?")
}

func splitSegments(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

func globMatchSegments(pattern, name []string) bool {
	if len(pattern) == 0 {
		return len(name) == 0
	}
	head := pattern[0]
	if head == "**" {
		if globMatchSegments(pattern[1:], name) {
			return true
		}
		if len(name) == 0 {
			return false
		}
		return globMatchSegments(pattern, name[1:])
	}
	if len(name) == 0 {
		return false
	}
	if !segmentMatch(head, name[0]) {
		return false
	}
	return globMatchSegments(pattern[1:], name[1:])
}

// segmentMatch matches a single path segment against a pattern segment
// containing '*' and '?' wildcards (no '/' in either).
func segmentMatch(pattern, segment string) bool {
	return matchHere(pattern, segment)
}

func matchHere(pattern, s string) bool {
	for len(pattern) > 0 {
		switch pattern[0] {
		case '*':
			// Try every possible split; collapse consecutive '*' first.
			for len(pattern) > 0 && pattern[0] == '*' {
				pattern = pattern[1:]
			}
			if len(pattern) == 0 {
				return true
			}
			for i := 0; i <= len(s); i++ {
				if matchHere(pattern, s[i:]) {
					return true
				}
			}
			return false
		case '?':
			if len(s) == 0 {
				return false
			}
			pattern = pattern[1:]
			s = s[1:]
		default:
			if len(s) == 0 || pattern[0] != s[0] {
				return false
			}
			pattern = pattern[1:]
			s = s[1:]
		}
	}
	return len(s) == 0
}
