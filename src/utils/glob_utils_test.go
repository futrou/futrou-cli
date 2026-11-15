package utils

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

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

func TestRulesObjectKeepsTheOrderOfTheKeys(t *testing.T) {
	var rules GlobRules
	doc := `{"./z": true, "./a/*.{js,css}": false, "./m": "./", "./b": "static/"}`
	if err := json.Unmarshal([]byte(doc), &rules); err != nil {
		t.Fatal(err)
	}
	want := GlobRules{{Source: "./z"}, {Source: "./a/*.{js,css}", Exclude: true}, {Source: "./m", Dest: "./"}, {Source: "./b", Dest: "static/"}}
	if !reflect.DeepEqual(rules, want) {
		t.Fatalf("rules = %#v", rules)
	}
}

func TestRulesArrayForm(t *testing.T) {
	var rules GlobRules
	doc := `["./dist", {"./dist/*.map": false}, {"./extra": "x/"}, "./dist"]`
	if err := json.Unmarshal([]byte(doc), &rules); err != nil {
		t.Fatal(err)
	}
	want := GlobRules{{Source: "./dist"}, {Source: "./dist/*.map", Exclude: true}, {Source: "./extra", Dest: "x/"}, {Source: "./dist"}}
	if !reflect.DeepEqual(rules, want) {
		t.Fatalf("rules = %#v", rules)
	}
}

func TestRulesRejectBadValues(t *testing.T) {
	for name, doc := range map[string]string{
		"number value":      `{"./a": 5}`,
		"null value":        `{"./a": null}`,
		"empty destination": `{"./a": ""}`,
		"blank destination": `{"./a": "  "}`,
		"array value":       `{"./a": []}`,
		"empty source":      `{"": true}`,
		"scalar":            `"./a"`,
		"number":            `5`,
		"empty array item":  `[""]`,
		"number item":       `[5]`,
		"two-key item":      `[{"./a": true, "./b": true}]`,
		"empty item":        `[{}]`,
	} {
		t.Run(name, func(t *testing.T) {
			var rules GlobRules
			if err := json.Unmarshal([]byte(doc), &rules); err == nil {
				t.Fatalf("expected an error, got %#v", rules)
			}
		})
	}
}

func TestRulesNullIsNoRules(t *testing.T) {
	rules := GlobRules{{Source: "x"}}
	if err := json.Unmarshal([]byte(`null`), &rules); err != nil || rules != nil {
		t.Fatalf("rules = %v err = %v", rules, err)
	}
}

func TestRulesMarshalKeepsTheOrder(t *testing.T) {
	rules := GlobRules{{Source: "./z"}, {Source: "./a", Exclude: true}, {Source: "./m", Dest: "./"}}
	data, err := json.Marshal(rules)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"./z":true,"./a":false,"./m":"./"}` {
		t.Fatalf("json = %s", data)
	}
	var back GlobRules
	if err := json.Unmarshal(data, &back); err != nil || !reflect.DeepEqual(back, rules) {
		t.Fatalf("round trip = %#v err = %v", back, err)
	}
}

func TestRulesMarshalAsAnArrayWhenASourceRepeats(t *testing.T) {
	rules := GlobRules{{Source: "./dist"}, {Source: "./dist/*.map", Exclude: true}, {Source: "./dist/*.map"}}
	data, err := json.Marshal(rules)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `[{"./dist":true},{"./dist/*.map":false},{"./dist/*.map":true}]` {
		t.Fatalf("json = %s", data)
	}
	var back GlobRules
	if err := json.Unmarshal(data, &back); err != nil || !reflect.DeepEqual(back, rules) {
		t.Fatalf("round trip = %#v err = %v", back, err)
	}
}

func TestRulesMarshalEscapesSources(t *testing.T) {
	rules := GlobRules{{Source: `a "quoted" \ path`, Dest: "d\"e"}}
	data, _ := json.Marshal(rules)
	var back GlobRules
	if err := json.Unmarshal(data, &back); err != nil || !reflect.DeepEqual(back, rules) {
		t.Fatalf("round trip = %#v err = %v (%s)", back, err, data)
	}
}

func TestParseGlobRule(t *testing.T) {
	for spec, want := range map[string]GlobRule{
		"./dist":                      {Source: "./dist"},
		"./dist:./":                   {Source: "./dist", Dest: "./"},
		"src/*.js:js/":                {Source: "src/*.js", Dest: "js/"},
		"a.txt:b.txt":                 {Source: "a.txt", Dest: "b.txt"},
		"/abs/path:site":              {Source: "/abs/path", Dest: "site"},
		"./robots.txt:seo/robots.txt": {Source: "./robots.txt", Dest: "seo/robots.txt"},
		"./a:b.txt:renamed":           {Source: "./a:b.txt", Dest: "renamed"}, // the last colon separates
		`C:\site`:                     {Source: `C:\site`},
		`C:/site`:                     {Source: `C:/site`},
		`C:\site:./`:                  {Source: `C:\site`, Dest: "./"},
		`D:\a:b:docs`:                 {Source: `D:\a:b`, Dest: "docs"},
		"./dist=./":                   {Source: "./dist=./"}, // = is just a character now
	} {
		got, err := ParseGlobRule(spec)
		if err != nil || got != want {
			t.Errorf("ParseGlobRule(%q) = %#v, %v, want %#v", spec, got, err, want)
		}
	}
	for _, spec := range []string{"", "  ", ":dest", "src:", "src:  "} {
		if _, err := ParseGlobRule(spec); err == nil || !strings.Contains(err.Error(), "include") {
			t.Errorf("ParseGlobRule(%q) err = %v", spec, err)
		}
	}
}

func TestOrderedKeys(t *testing.T) {
	keys, err := OrderedKeys([]byte(`{"b": {"z": 1, "a": 2}, "a": [1], "c": null}`))
	if err != nil || !reflect.DeepEqual(keys, []string{"b", "a", "c"}) {
		t.Fatalf("keys = %v err = %v", keys, err)
	}
	for _, bad := range []string{``, `[]`, `5`, `{"a":`} {
		if _, err := OrderedKeys([]byte(bad)); err == nil {
			t.Errorf("OrderedKeys(%q) should fail", bad)
		}
	}
}

func TestRuleFromValue(t *testing.T) {
	for _, tc := range []struct {
		value interface{}
		want  GlobRule
	}{
		{true, GlobRule{Source: "s"}},
		{false, GlobRule{Source: "s", Exclude: true}},
		{"./", GlobRule{Source: "s", Dest: "./"}},
	} {
		if got, err := GlobRuleFromValue("s", tc.value); err != nil || got != tc.want {
			t.Errorf("GlobRuleFromValue(%v) = %#v, %v", tc.value, got, err)
		}
	}
	for _, bad := range []interface{}{nil, 1.5, []interface{}{}, "", " "} {
		if _, err := GlobRuleFromValue("s", bad); err == nil {
			t.Errorf("GlobRuleFromValue(%v) should fail", bad)
		}
	}
	if _, err := GlobRuleFromValue("", true); err == nil {
		t.Error("an empty source should fail")
	}
}

func TestExpandBraces(t *testing.T) {
	for pattern, want := range map[string][]string{
		"a/*.{js,css}":   {"a/*.js", "a/*.css"},
		"{a,b}/{x,y}":    {"a/x", "a/y", "b/x", "b/y"},
		"a/{b,c{d,e}}":   {"a/b", "a/cd", "a/ce"},
		"plain":          {"plain"},
		"{single}":       {"{single}"},
		"{unclosed,a":    {"{unclosed,a"},
		"a/*.{jpg,webp}": {"a/*.jpg", "a/*.webp"},
		"x{,.min}.js":    {"x.js", "x.min.js"},
	} {
		if got := ExpandBraces(pattern); !reflect.DeepEqual(got, want) {
			t.Errorf("ExpandBraces(%q) = %v, want %v", pattern, got, want)
		}
	}
}

func TestMatchPattern(t *testing.T) {
	for _, tc := range []struct {
		pattern, name string
		want          bool
	}{
		{"*.map", "a/b/c.js.map", true},
		{"*.map", "c.js", false},
		{"drafts", "a/drafts/x.html", true},
		{"drafts", "a/draftsy/x.html", false},
		{"a/b", "a/b/c.txt", true},
		{"a/b", "x/a/b/c.txt", false},
		{"./a/", "a/b.txt", true},
		{"**/*.png", "x/y/z.png", true},
		{"*.{js,css}", "a/b.css", true},
		{".", "anything", true},
		{"", "anything", false},
		{"a?c.txt", "abc.txt", true},
	} {
		if got := MatchPattern(tc.pattern, tc.name); got != tc.want {
			t.Errorf("MatchPattern(%q, %q) = %v, want %v", tc.pattern, tc.name, got, tc.want)
		}
	}
}

func TestGlobRoot(t *testing.T) {
	for in, want := range map[string]string{
		"a/b/*.txt":      "a/b",
		"*.txt":          "",
		"a/**/c":         "a",
		"/abs/*":         "/abs",
		"/*.log":         "/",
		"/":              "/",
		"plain/path":     "plain/path",
		"/plain/path":    "/plain/path",
		"a/b?/c":         "a",
		"/site/dist/x.y": "/site/dist/x.y",
	} {
		if got := GlobRoot(in); got != want {
			t.Errorf("GlobRoot(%q) = %q, want %q", in, got, want)
		}
	}
}
