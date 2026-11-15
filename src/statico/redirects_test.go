package statico_test

import (
	"regexp"
	"strings"
	"testing"

	. "futrou-cli/src/statico"
)

func rule(t *testing.T, line string) map[string]interface{} {
	t.Helper()
	rules, warnings := ParseRedirects(line)
	if len(rules) != 1 || len(warnings) != 0 {
		t.Fatalf("%q: rules = %v warnings = %v", line, rules, warnings)
	}
	return rules[0]
}

func parts(t *testing.T, r map[string]interface{}) (path, host, to string, status int) {
	t.Helper()
	cond := r["if"].(map[string]interface{})
	path, _ = cond["path"].(string)
	host, _ = cond["host"].(string)
	action := r["then"].([]interface{})[0].(map[string]interface{})
	if action["type"] != "redirect" {
		t.Fatalf("action = %v", action)
	}
	return path, host, action["to"].(string), action["statusCode"].(int)
}

func TestParseRedirectsStaticLines(t *testing.T) {
	for line, want := range map[string][4]interface{}{
		"/home / 301":                         {"/home", "", "/", 301},
		"/old /new":                           {"/old", "", "/new", 302},
		"/twitter https://twitter.com/futrou": {"/twitter", "", "https://twitter.com/futrou", 302},
		"  /a    /b    308  ":                 {"/a", "", "/b", 308},
		"/a\t/b\t303":                         {"/a", "", "/b", 303},
		"/a /b 307":                           {"/a", "", "/b", 307},
		"/a /b#section 301":                   {"/a", "", "/b#section", 301},
		"/a /b?x=1&y=2 301":                   {"/a", "", "/b?x=1&y=2", 301},
	} {
		path, host, to, status := parts(t, rule(t, line))
		if path != want[0] || host != want[1] || to != want[2] || status != want[3] {
			t.Errorf("%q = %q %q %q %d, want %v", line, path, host, to, status, want)
		}
	}
}

func TestParseRedirectsSplat(t *testing.T) {
	path, _, to, status := parts(t, rule(t, "/blog/* /news/:splat 301"))
	if path != `regex:^/blog/(?P<splat>.*)$` || to != "/news/${splat}" || status != 301 {
		t.Fatalf("%q %q %d", path, to, status)
	}
	re := regexp.MustCompile(strings.TrimPrefix(path, "regex:"))
	for p, want := range map[string]string{"/blog/a": "a", "/blog/a/b/c": "a/b/c", "/blog/": "", "/blog/x.html": "x.html"} {
		m := re.FindStringSubmatch(p)
		if m == nil || m[1] != want {
			t.Errorf("%s: %v, want splat %q", p, m, want)
		}
	}
	for _, p := range []string{"/blog", "/other/a", "/xblog/a"} {
		if re.MatchString(p) {
			t.Errorf("%s must not match", p)
		}
	}
}

func TestParseRedirectsPlaceholders(t *testing.T) {
	path, _, to, _ := parts(t, rule(t, "/products/:code/:name /products?code=:code&name=:name 301"))
	if path != `regex:^/products/(?P<code>[^/]+)/(?P<name>[^/]+)/?$` || to != "/products?code=${code}&name=${name}" {
		t.Fatalf("%q %q", path, to)
	}
	re := regexp.MustCompile(strings.TrimPrefix(path, "regex:"))
	if m := re.FindStringSubmatch("/products/7/shoe"); m == nil || m[1] != "7" || m[2] != "shoe" {
		t.Fatalf("match = %v", m)
	}
	if !re.MatchString("/products/7/shoe/") {
		t.Error("a trailing slash is the same page")
	}
	for _, p := range []string{"/products/7", "/products/7/a/b", "/products//x"} {
		if re.MatchString(p) {
			t.Errorf("%s must not match", p)
		}
	}
}

func TestParseRedirectsPlaceholderNextToLiteralText(t *testing.T) {
	path, _, to, _ := parts(t, rule(t, "/user-:id.html /u/:id"))
	if path != `regex:^/user-(?P<id>[^/]+)\.html/?$` || to != "/u/${id}" {
		t.Fatalf("%q %q", path, to)
	}
}

func TestParseRedirectsSplatAndPlaceholdersTogether(t *testing.T) {
	path, _, to, _ := parts(t, rule(t, "/:lang/docs/* /docs/:lang/:splat"))
	if path != `regex:^/(?P<lang>[^/]+)/docs/(?P<splat>.*)$` || to != "/docs/${lang}/${splat}" {
		t.Fatalf("%q %q", path, to)
	}
}

func TestParseRedirectsUnknownPlaceholderInTheDestinationStaysLiteral(t *testing.T) {
	_, _, to, _ := parts(t, rule(t, "/a/:id /b/:other"))
	if to != "/b/:other" {
		t.Fatalf("to = %q", to)
	}
	// a port is not a placeholder
	if _, _, to, _ := parts(t, rule(t, "/a/:id http://example.com:8080/:id")); to != "http://example.com:8080/${id}" {
		t.Fatalf("to = %q", to)
	}
}

func TestParseRedirectsDollarSignsAreEscaped(t *testing.T) {
	_, _, to, _ := parts(t, rule(t, "/a/* /cost$5/:splat"))
	if to != "/cost$$5/${splat}" {
		t.Fatalf("to = %q", to)
	}
	// a static rule is sent as written: the node only expands regex rules
	if _, _, to, _ := parts(t, rule(t, "/a /cost$5")); to != "/cost$5" {
		t.Fatalf("static to = %q", to)
	}
}

func TestParseRedirectsRegexMetaCharactersInTheSourceAreLiteral(t *testing.T) {
	path, _, _, _ := parts(t, rule(t, "/a.b+c/(x)/* /y"))
	re := regexp.MustCompile(strings.TrimPrefix(path, "regex:"))
	if !re.MatchString("/a.b+c/(x)/z") || re.MatchString("/aXb+c/(x)/z") {
		t.Fatalf("pattern %q", path)
	}
}

func TestParseRedirectsWithAHostMatchesTheHost(t *testing.T) {
	path, host, to, _ := parts(t, rule(t, "https://example.pages.dev/* https://example.com/:splat 301"))
	if host != "example.pages.dev" || path != `regex:^/(?P<splat>.*)$` || to != "https://example.com/${splat}" {
		t.Fatalf("%q %q %q", path, host, to)
	}
	path, host, _, _ = parts(t, rule(t, "https://old.example.com/home /"))
	if host != "old.example.com" || path != "/home" {
		t.Fatalf("%q %q", path, host)
	}
	if _, host, _, _ := parts(t, rule(t, "https://example.com https://other.com 301")); host != "example.com" {
		t.Fatalf("host = %q", host)
	}
	if path, host, _, _ := parts(t, rule(t, "http://example.com:8080/x /y")); host != "example.com" || path != "/x" {
		t.Fatalf("a port is not part of the host: %q %q", host, path)
	}
}

func TestParseRedirectsSkipsCommentsAndBlankLinesAndKeepsTheOrder(t *testing.T) {
	file := "# redirects\n\n/a /1\r\n   # indented comment\n/b /2 301\n\n/c /3 308\n"
	rules, warnings := ParseRedirects(file)
	if len(warnings) != 0 || len(rules) != 3 {
		t.Fatalf("rules = %v warnings = %v", rules, warnings)
	}
	for i, want := range []string{"/a", "/b", "/c"} {
		if path, _, _, _ := parts(t, rules[i]); path != want {
			t.Errorf("rule %d = %s, want %s", i, path, want)
		}
	}
}

func TestParseRedirectsWarnsAboutLinesItCannotConvert(t *testing.T) {
	for line, want := range map[string]string{
		"/only-one":                     "expected",
		"/a /b 301 extra":               "expected",
		"/a /b abc":                     "not a number",
		"/a /b 200":                     "proxying",
		"/a /b 404":                     "not one of",
		"/a /b 304":                     "not one of",
		"/a?x=1 /b":                     "query string",
		"/a#frag /b":                    "fragment",
		"nodomain /b":                   "must start with a slash",
		"/a/*/b/* /c":                   "more than one splat",
		"/a/:x/:x /b":                   "used twice",
		"https://:sub.example.com/* /x": "host",
		"https://*.example.com/* /x":    "host",
	} {
		rules, warnings := ParseRedirects("/ok /fine\n" + line + "\n")
		if len(rules) != 1 {
			t.Errorf("%q: the valid line must still be converted, rules = %d", line, len(rules))
		}
		if len(warnings) != 1 || !strings.Contains(warnings[0], "line 2") || !strings.Contains(warnings[0], want) {
			t.Errorf("%q: warnings = %v, want one about %q on line 2", line, warnings, want)
		}
	}
}

func TestParseRedirectsWarnsAboutTooManyRules(t *testing.T) {
	var b strings.Builder
	for i := 0; i <= MaxRedirects; i++ {
		b.WriteString("/p")
		b.WriteString(strings.Repeat("a", 1+i%7))
		b.WriteString(string(rune('a' + i%26)))
		b.WriteString(" /x\n")
	}
	rules, warnings := ParseRedirects(b.String())
	if len(rules) != MaxRedirects+1 || len(warnings) != 1 || !strings.Contains(warnings[0], "at most") {
		t.Fatalf("rules = %d warnings = %v", len(rules), warnings)
	}
}

func TestParseRedirectsEmptyFile(t *testing.T) {
	if rules, warnings := ParseRedirects(""); len(rules) != 0 || len(warnings) != 0 {
		t.Fatalf("rules = %v warnings = %v", rules, warnings)
	}
	if rules, _ := ParseRedirects("# only a comment\n"); len(rules) != 0 {
		t.Fatalf("rules = %v", rules)
	}
}

func TestRedirectFromTheConfig(t *testing.T) {
	r, err := Redirect{From: "/old", To: "/new"}.Rule()
	if err != nil {
		t.Fatal(err)
	}
	if path, _, to, status := parts(t, r); path != "/old" || to != "/new" || status != 302 {
		t.Fatalf("%q %q %d", path, to, status)
	}
	r, _ = Redirect{From: "/docs/*", To: "/new/:splat", StatusCode: 301}.Rule()
	if path, _, to, status := parts(t, r); path != `regex:^/docs/(?P<splat>.*)$` || to != "/new/${splat}" || status != 301 {
		t.Fatalf("%q %q %d", path, to, status)
	}
	// no source: every path
	r, _ = Redirect{To: "https://example.com/:splat", StatusCode: 308}.Rule()
	path, _, to, _ := parts(t, r)
	if path != `regex:^/(?P<splat>.*)$` || to != "https://example.com/${splat}" {
		t.Fatalf("%q %q", path, to)
	}
	re := regexp.MustCompile(strings.TrimPrefix(path, "regex:"))
	for _, p := range []string{"/", "/a", "/a/b/c"} {
		if !re.MatchString(p) {
			t.Errorf("%s must match a redirect without a source", p)
		}
	}
}

func TestRedirectFromTheConfigErrors(t *testing.T) {
	for name, r := range map[string]Redirect{
		"no destination":  {From: "/a"},
		"space in to":     {From: "/a", To: "/b c"},
		"bad status":      {From: "/a", To: "/b", StatusCode: 200},
		"bad status 404":  {From: "/a", To: "/b", StatusCode: 404},
		"relative source": {From: "a", To: "/b"},
		"query in source": {From: "/a?x=1", To: "/b"},
		"two splats":      {From: "/*/*", To: "/b"},
	} {
		if _, err := r.Rule(); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if _, err := RedirectRules([]Redirect{{From: "/a", To: "/b"}, {From: "/c"}}); err == nil || !strings.Contains(err.Error(), "redirect 2") {
		t.Fatalf("err = %v", err)
	}
	if rules, err := RedirectRules(nil); err != nil || rules != nil {
		t.Fatalf("rules = %v err = %v", rules, err)
	}
}

func TestMergeRedirects(t *testing.T) {
	a := []map[string]interface{}{{"n": 1}}
	b := []map[string]interface{}{{"n": 2}, {"n": 3}}
	if got := MergeRedirects(a, b); len(got) != 3 || got[0]["n"] != 1 || got[2]["n"] != 3 {
		t.Fatalf("merged = %v", got)
	}
	if MergeRedirects(nil, nil) != nil {
		t.Fatal("nothing merges to nil")
	}
	if got := MergeRedirects(nil, b); len(got) != 2 {
		t.Fatalf("merged = %v", got)
	}
}
