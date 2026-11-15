package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// GlobMatch reports whether name matches pattern using shell-style globs
// over '/'-separated segments. '*' matches within a single segment; '**'
// matches across any number of segments (including zero). '?' matches any
// single rune within a segment.
func GlobMatch(pattern, name string) bool {
	return globMatchSegments(splitSegments(pattern), splitSegments(name))
}

// GlobRoot returns the longest path prefix of pattern that contains no glob
// metacharacters: the folder a search for the pattern starts from (and the one
// a remote listing is scoped to). A leading '/' is kept, so an absolute path
// stays absolute; "*.txt" has no root and gives "".
func GlobRoot(pattern string) string {
	absolute := strings.HasPrefix(pattern, "/")
	segments := strings.Split(strings.Trim(pattern, "/"), "/")
	root := make([]string, 0, len(segments))
	for _, segment := range segments {
		if HasGlobMeta(segment) {
			break
		}
		root = append(root, segment)
	}
	joined := strings.Join(root, "/")
	if absolute {
		return "/" + joined
	}
	return joined
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

// ExpandBraces expands {a,b} groups, as in *.{js,css}, into one pattern each.
func ExpandBraces(pattern string) []string {
	start := strings.IndexByte(pattern, '{')
	if start < 0 {
		return []string{pattern}
	}
	depth, end := 0, -1
	var commas []int
	for i := start; i < len(pattern); i++ {
		switch pattern[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i
			}
		case ',':
			if depth == 1 {
				commas = append(commas, i)
			}
		}
		if end >= 0 {
			break
		}
	}
	if end < 0 || len(commas) == 0 {
		return []string{pattern} // not a group: braces without a comma are literal
	}
	prefix, suffix := pattern[:start], pattern[end+1:]
	var out []string
	from := start + 1
	for _, to := range append(commas, end) {
		for _, tail := range ExpandBraces(pattern[from:to] + suffix) {
			out = append(out, prefix+tail)
		}
		from = to + 1
	}
	return out
}

// MatchPattern reports whether p (a file, a folder or a glob, as in .gitignore
// but without negation) selects name, a slash separated relative path. A
// pattern without a slash matches at any depth, a folder matches everything
// below it, and "**" matches across folders.
func MatchPattern(pattern, name string) bool {
	pattern = strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(pattern)), "./")
	pattern = strings.TrimSuffix(pattern, "/")
	name = strings.TrimPrefix(name, "./")
	if pattern == "" || pattern == "." {
		return pattern == "." // "." is everything
	}
	for _, expanded := range ExpandBraces(pattern) {
		candidates := []string{expanded}
		if !strings.Contains(expanded, "/") {
			candidates = append(candidates, "**/"+expanded)
		}
		for _, c := range candidates {
			if GlobMatch(c, name) || GlobMatch(path.Join(c, "**"), name) {
				return true
			}
		}
	}
	return false
}

// GlobRule is one step of a website's file selection. GlobRules run in order, like the
// keys of the "include" object in futrou.json:
//
//	"./src/index.js": true          include the file (its path relative to the config is kept)
//	"./src/public": "./"            include the folder's files, at the root of the website
//	"./src/public/*.{js,css}": false exclude what matches
type GlobRule struct {
	// Source is a file, a folder or a glob, relative to the config file (or the
	// current directory on the command line), or absolute.
	Source string
	// Dest is where the matched files land in the website. Empty means the path
	// of the source relative to the config is kept (true in the config).
	Dest string
	// Exclude removes what the source matches from the files selected so far (false in the config).
	Exclude bool
}

// GlobRules is an ordered list of rules. In JSON and YAML it is an object, whose key
// order is the rule order, or an array of "source" strings and one-key objects.
type GlobRules []GlobRule

// ParseGlobRule reads a command-line include: "SOURCE" or "SOURCE:DESTINATION".
// The colon is the last one, except one that is a Windows drive ("C:\\dist"), so
// a source may hold colons itself.
func ParseGlobRule(spec string) (GlobRule, error) {
	source, dest := spec, ""
	mapped := false
	if i := strings.LastIndex(spec, ":"); i >= 0 && !isDriveColon(spec, i) {
		source, dest, mapped = spec[:i], spec[i+1:], true
	}
	if strings.TrimSpace(source) == "" {
		return GlobRule{}, fmt.Errorf("include %q needs a source", spec)
	}
	if mapped && strings.TrimSpace(dest) == "" {
		return GlobRule{}, fmt.Errorf("include %q needs a destination after \":\"", spec)
	}
	return GlobRule{Source: source, Dest: dest}, nil
}

// isDriveColon reports whether the colon at i is the one of a Windows path such as C:\dist.
func isDriveColon(spec string, i int) bool {
	if i != 1 {
		return false
	}
	c := spec[0]
	letter := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
	return letter && (len(spec) == 2 || spec[2] == '\\' || spec[2] == '/')
}

// GlobRuleFromValue builds a rule from a config entry: true, false or a destination.
func GlobRuleFromValue(source string, value interface{}) (GlobRule, error) {
	if strings.TrimSpace(source) == "" {
		return GlobRule{}, fmt.Errorf("an include needs a source")
	}
	switch v := value.(type) {
	case bool:
		return GlobRule{Source: source, Exclude: !v}, nil
	case string:
		if strings.TrimSpace(v) == "" {
			return GlobRule{}, fmt.Errorf("include %q: the destination must not be empty (use true to keep the path)", source)
		}
		return GlobRule{Source: source, Dest: v}, nil
	}
	return GlobRule{}, fmt.Errorf("include %q must be true, false or a destination path", source)
}

type keyed struct {
	Key   string
	Value json.RawMessage
}

// orderedObject decodes a JSON object keeping the order of its keys.
func orderedObject(data []byte) ([]keyed, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := token.(json.Delim); !ok || delim != '{' {
		return nil, fmt.Errorf("expected an object")
	}
	var out []keyed
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		out = append(out, keyed{Key: keyToken.(string), Value: value})
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	return out, nil
}

func ruleFromRaw(source string, raw json.RawMessage) (GlobRule, error) {
	var value interface{}
	if err := json.Unmarshal(raw, &value); err != nil {
		return GlobRule{}, err
	}
	return GlobRuleFromValue(source, value)
}

// UnmarshalJSON accepts an object (the key order is the rule order) or an array
// whose items are source strings or objects with one key.
func (r *GlobRules) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if bytes.Equal(trimmed, []byte("null")) {
		*r = nil
		return nil
	}
	var rules GlobRules
	switch {
	case len(trimmed) > 0 && trimmed[0] == '{':
		entries, err := orderedObject(trimmed)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			rule, err := ruleFromRaw(entry.Key, entry.Value)
			if err != nil {
				return err
			}
			rules = append(rules, rule)
		}
	case len(trimmed) > 0 && trimmed[0] == '[':
		var items []json.RawMessage
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return err
		}
		for i, item := range items {
			var source string
			if json.Unmarshal(item, &source) == nil {
				if strings.TrimSpace(source) == "" {
					return fmt.Errorf("include item %d must not be empty", i)
				}
				rules = append(rules, GlobRule{Source: source})
				continue
			}
			entries, err := orderedObject(item)
			if err != nil || len(entries) != 1 {
				return fmt.Errorf("include item %d must be a source string or an object with one source", i)
			}
			rule, err := ruleFromRaw(entries[0].Key, entries[0].Value)
			if err != nil {
				return err
			}
			rules = append(rules, rule)
		}
	default:
		return fmt.Errorf("include must be an object or an array")
	}
	*r = rules
	return nil
}

func (r GlobRule) value() interface{} {
	switch {
	case r.Exclude:
		return false
	case r.Dest != "":
		return r.Dest
	}
	return true
}

// MarshalJSON writes the object form, whose key order is the rule order. GlobRules
// that repeat a source cannot be an object, so they are written as an array.
func (r GlobRules) MarshalJSON() ([]byte, error) {
	seen := map[string]bool{}
	unique := true
	for _, rule := range r {
		if seen[rule.Source] {
			unique = false
		}
		seen[rule.Source] = true
	}
	var buf bytes.Buffer
	if unique {
		buf.WriteByte('{')
	} else {
		buf.WriteByte('[')
	}
	for i, rule := range r {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, _ := json.Marshal(rule.Source)
		value, err := json.Marshal(rule.value())
		if err != nil {
			return nil, err
		}
		if !unique {
			buf.WriteByte('{')
		}
		buf.Write(key)
		buf.WriteByte(':')
		buf.Write(value)
		if !unique {
			buf.WriteByte('}')
		}
	}
	if unique {
		buf.WriteByte('}')
	} else {
		buf.WriteByte(']')
	}
	return buf.Bytes(), nil
}

// OrderedKeys returns the keys of a JSON object in file order; the config loader
// uses it to keep the order of "include" through the unordered map it decodes into.
func OrderedKeys(data []byte) ([]string, error) {
	entries, err := orderedObject(data)
	if err != nil {
		return nil, err
	}
	keys := make([]string, len(entries))
	for i, entry := range entries {
		keys[i] = entry.Key
	}
	return keys, nil
}
