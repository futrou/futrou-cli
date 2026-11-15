package statico_test

import (
	"futrou-cli/src/utils"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	. "futrou-cli/src/statico"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func paths(files []LocalFile) []string {
	out := []string{}
	for _, f := range files {
		out = append(out, f.Path)
	}
	return out
}

func resolve(t *testing.T, base string, rules utils.GlobRules) *Resolved {
	t.Helper()
	resolved, err := Resolve(base, Selection{Rules: rules, NoHTMLToFolders: true})
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func resolvePaths(t *testing.T, base string, rules utils.GlobRules) []string {
	t.Helper()
	return paths(resolve(t, base, rules).Files)
}

// project is the folder the examples of the include object are written for.
func project(t *testing.T) string {
	root := t.TempDir()
	for _, f := range []string{
		"src/index.js", "src/public/app.js", "src/public/style.css", "src/public/logo.svg", "src/public/main.html",
		"src/public/images/a.jpg", "src/public/images/b.webp", "src/public/images/c.png", "src/public/images/deep/d.jpg",
		"README.md", ".git/config", ".futrou/old", "other/x.txt",
	} {
		write(t, root, f, f)
	}
	return root
}

func TestResolveWithoutRulesTakesTheWholeFolderAtTheRootButNeverGitOrFutrou(t *testing.T) {
	got := resolvePaths(t, project(t), nil)
	for _, p := range got {
		if strings.HasPrefix(p, ".git") || strings.HasPrefix(p, ".futrou") {
			t.Fatalf("%s must never be uploaded: %v", p, got)
		}
	}
	if len(got) != 11 || got[0] != "README.md" {
		t.Fatalf("paths = %v", got)
	}
}

func TestResolveTrueKeepsThePathOfFilesAndGlobsAndPutsAFolderAtTheRoot(t *testing.T) {
	root := project(t)
	for name, tc := range map[string]struct {
		rules utils.GlobRules
		want  []string
	}{
		"a file":                            {utils.GlobRules{{Source: "./src/index.js"}}, []string{"src/index.js"}},
		"a file without ./":                 {utils.GlobRules{{Source: "src/index.js"}}, []string{"src/index.js"}},
		"a folder is its files at the root": {utils.GlobRules{{Source: "./src/public/images"}}, []string{"a.jpg", "b.webp", "c.png", "deep/d.jpg"}},
		"a glob":                            {utils.GlobRules{{Source: "./src/public/*.css"}}, []string{"src/public/style.css"}},
		"a recursive glob":                  {utils.GlobRules{{Source: "./src/public/**/*.jpg"}}, []string{"src/public/images/a.jpg", "src/public/images/deep/d.jpg"}},
		"two files in order":                {utils.GlobRules{{Source: "README.md"}, {Source: "other/x.txt"}}, []string{"README.md", "other/x.txt"}},
	} {
		t.Run(name, func(t *testing.T) {
			if got := resolvePaths(t, root, tc.rules); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("paths = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestResolveDestinationMapsTheSource(t *testing.T) {
	root := project(t)
	for name, tc := range map[string]struct {
		rules utils.GlobRules
		want  []string
	}{
		"folder to the root":        {utils.GlobRules{{Source: "./src/public/images", Dest: "./"}}, []string{"a.jpg", "b.webp", "c.png", "deep/d.jpg"}},
		"folder to the root, slash": {utils.GlobRules{{Source: "./src/public/images", Dest: "/"}}, []string{"a.jpg", "b.webp", "c.png", "deep/d.jpg"}},
		"folder to the root, dot":   {utils.GlobRules{{Source: "./src/public/images", Dest: "."}}, []string{"a.jpg", "b.webp", "c.png", "deep/d.jpg"}},
		"folder below a name":       {utils.GlobRules{{Source: "./src/public/images", Dest: "img"}}, []string{"img/a.jpg", "img/b.webp", "img/c.png", "img/deep/d.jpg"}},
		"folder below a path":       {utils.GlobRules{{Source: "./src/public/images", Dest: "./static/img/"}}, []string{"static/img/a.jpg", "static/img/b.webp", "static/img/c.png", "static/img/deep/d.jpg"}},
		"file renamed":              {utils.GlobRules{{Source: "./src/index.js", Dest: "main.js"}}, []string{"main.js"}},
		"file renamed into a path":  {utils.GlobRules{{Source: "./src/index.js", Dest: "js/main.js"}}, []string{"js/main.js"}},
		"file into the root":        {utils.GlobRules{{Source: "./src/index.js", Dest: "./"}}, []string{"index.js"}},
		"file into a folder":        {utils.GlobRules{{Source: "./src/index.js", Dest: "static/"}}, []string{"static/index.js"}},
		"glob keeps the tail":       {utils.GlobRules{{Source: "./src/public/images/**/*.jpg", Dest: "photos"}}, []string{"photos/a.jpg", "photos/deep/d.jpg"}},
		"glob to the root":          {utils.GlobRules{{Source: "./src/public/*.css", Dest: "./"}}, []string{"style.css"}},
		"braces":                    {utils.GlobRules{{Source: "./src/public/*.{js,css}", Dest: "./"}}, []string{"app.js", "style.css"}},
	} {
		t.Run(name, func(t *testing.T) {
			if got := resolvePaths(t, root, tc.rules); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("paths = %v, want %v", got, tc.want)
			}
		})
	}
}

// The example of the include object: rules run in the order of the keys.
func TestResolveTheDocumentedExampleRunsInOrder(t *testing.T) {
	rules := utils.GlobRules{
		{Source: "./src/index.js"},                         // true: the file, its path kept
		{Source: "./src/public", Dest: "./"},               // the whole folder, at the root
		{Source: "./src/public/*.{js,css}", Exclude: true}, // false: leave out the .js and .css of it
		{Source: "./src/public/images/*.{jpg,webp}"},       // true: these images again, their path kept
	}
	want := []string{
		"images/a.jpg", "images/b.webp", "images/c.png", "images/deep/d.jpg", "logo.svg", "main.html", // the folder minus app.js and style.css
		"src/index.js",
		"src/public/images/a.jpg", "src/public/images/b.webp",
	}
	if got := resolvePaths(t, project(t), rules); !reflect.DeepEqual(got, want) {
		t.Fatalf("paths = %v\nwant     %v", got, want)
	}
}

func TestResolveOrderMatters(t *testing.T) {
	root := project(t)
	excludeAfter := resolvePaths(t, root, utils.GlobRules{{Source: "./src/public", Dest: "./"}, {Source: "./src/public/*.js", Exclude: true}})
	if contains(excludeAfter, "app.js") || !contains(excludeAfter, "style.css") {
		t.Fatalf("exclude after include: %v", excludeAfter)
	}
	includeAfter := resolvePaths(t, root, utils.GlobRules{{Source: "./src/public/*.js", Exclude: true}, {Source: "./src/public", Dest: "./"}})
	if !contains(includeAfter, "app.js") {
		t.Fatalf("an exclude before the include removes nothing: %v", includeAfter)
	}
	reincluded := resolvePaths(t, root, utils.GlobRules{{Source: "./src/public", Dest: "./"}, {Source: "./src/public/*.js", Exclude: true}, {Source: "./src/public/app.js", Dest: "./"}})
	if !contains(reincluded, "app.js") {
		t.Fatalf("an include after an exclude brings the file back: %v", reincluded)
	}
}

func contains(list []string, item string) bool {
	for _, x := range list {
		if x == item {
			return true
		}
	}
	return false
}

func TestResolveExcludeMatchesTheSourceNotTheDestination(t *testing.T) {
	root := project(t)
	// the folder is mounted under another name; excluding by the source path still finds it
	got := resolvePaths(t, root, utils.GlobRules{{Source: "./src/public", Dest: "pub"}, {Source: "./src/public/images", Exclude: true}})
	for _, p := range got {
		if strings.Contains(p, "images") {
			t.Fatalf("%s should have been excluded: %v", p, got)
		}
	}
	if !contains(got, "pub/app.js") {
		t.Fatalf("paths = %v", got)
	}
	// a pattern with a destination-looking path matches nothing
	same := resolvePaths(t, root, utils.GlobRules{{Source: "./src/public", Dest: "pub"}, {Source: "pub/app.js", Exclude: true}})
	if !contains(same, "pub/app.js") {
		t.Fatalf("an exclude is matched on disk, not in the website: %v", same)
	}
}

func TestResolveExcludeWithBracesGlobsFoldersAndFiles(t *testing.T) {
	root := project(t)
	all := utils.GlobRules{{Source: "./src/public", Dest: "./"}}
	for name, tc := range map[string]struct {
		exclude string
		gone    []string
	}{
		"braces":              {"./src/public/*.{js,css}", []string{"app.js", "style.css"}},
		"recursive":           {"./src/public/**/*.jpg", []string{"images/a.jpg", "images/deep/d.jpg"}},
		"a folder":            {"./src/public/images", []string{"images/a.jpg", "images/b.webp", "images/c.png", "images/deep/d.jpg"}},
		"a folder with slash": {"./src/public/images/", []string{"images/a.jpg", "images/b.webp", "images/c.png", "images/deep/d.jpg"}},
		"a subfolder":         {"./src/public/images/deep", []string{"images/deep/d.jpg"}},
		"a file":              {"./src/public/logo.svg", []string{"logo.svg"}},
		"a glob naming a dir": {"./src/public/im*", []string{"images/a.jpg", "images/b.webp", "images/c.png", "images/deep/d.jpg"}},
		"nothing matches":     {"./src/public/*.nope", nil},
		"a missing file":      {"./src/public/missing.txt", nil},
		"a missing folder":    {"./nowhere/*", nil},
	} {
		t.Run(name, func(t *testing.T) {
			before := resolvePaths(t, root, all)
			after := resolvePaths(t, root, append(append(utils.GlobRules{}, all...), utils.GlobRule{Source: tc.exclude, Exclude: true}))
			for _, g := range tc.gone {
				if contains(after, g) {
					t.Errorf("%s was not excluded: %v", g, after)
				}
			}
			if len(before)-len(after) != len(tc.gone) {
				t.Errorf("excluded %d files, want %d (%v)", len(before)-len(after), len(tc.gone), after)
			}
		})
	}
}

func TestResolveLaterRuleWinsOnTheSamePath(t *testing.T) {
	root := project(t)
	write(t, root, "over/app.js", "override")
	resolved := resolve(t, root, utils.GlobRules{{Source: "./src/public", Dest: "./"}, {Source: "./over", Dest: "./"}})
	for _, f := range resolved.Files {
		if f.Path == "app.js" && !strings.HasSuffix(filepath.ToSlash(f.Abs), "over/app.js") {
			t.Fatalf("app.js comes from %s", f.Abs)
		}
	}
}

func TestResolveAbsoluteSources(t *testing.T) {
	root := project(t)
	outside := t.TempDir()
	write(t, outside, "site/index.html", "i")
	write(t, outside, "site/css/a.css", "c")
	write(t, outside, "one.txt", "1")
	for name, tc := range map[string]struct {
		rules utils.GlobRules
		want  []string
	}{
		"a folder outside at the root": {utils.GlobRules{{Source: filepath.Join(outside, "site"), Dest: "./"}}, []string{"css/a.css", "index.html"}},
		"a folder outside, true":       {utils.GlobRules{{Source: filepath.Join(outside, "site")}}, []string{"css/a.css", "index.html"}},
		"a file outside":               {utils.GlobRules{{Source: filepath.Join(outside, "one.txt")}}, []string{"one.txt"}},
		"a glob outside":               {utils.GlobRules{{Source: filepath.Join(outside, "site", "**", "*.css"), Dest: "styles"}}, []string{"styles/css/a.css"}},
		"a path in the base, absolute": {utils.GlobRules{{Source: filepath.Join(root, "README.md")}}, []string{"README.md"}},
		"relative to the parent":       {utils.GlobRules{{Source: "../" + filepath.Base(outside) + "/one.txt"}}, []string{"one.txt"}},
	} {
		t.Run(name, func(t *testing.T) {
			if got := resolvePaths(t, root, tc.rules); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("paths = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestResolveErrors(t *testing.T) {
	root := project(t)
	for name, tc := range map[string]struct {
		rules utils.GlobRules
		want  string
	}{
		"missing include":          {utils.GlobRules{{Source: "nope"}}, "does not exist"},
		"missing folder in a glob": {utils.GlobRules{{Source: "gone/*.css"}}, "matches no files"},
		"glob matches none":        {utils.GlobRules{{Source: "src/*.css"}}, "matches no files"},
		"empty folder":             {utils.GlobRules{{Source: "emptydir"}}, ""},
		"destination escapes":      {utils.GlobRules{{Source: "README.md", Dest: "../x.md"}}, "leaves the website"},
		"destination escapes deep": {utils.GlobRules{{Source: "src/public", Dest: "a/../../x"}}, "leaves the website"},
		"everything excluded":      {utils.GlobRules{{Source: "README.md"}, {Source: "README.md", Exclude: true}}, "everything selected was excluded"},
		"only an exclude":          {utils.GlobRules{{Source: "README.md", Exclude: true}}, "everything selected was excluded"},
	} {
		t.Run(name, func(t *testing.T) {
			os.MkdirAll(filepath.Join(root, "emptydir"), 0o755)
			_, err := Resolve(root, Selection{Rules: tc.rules})
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestResolveSkipsLinksAndNeverFollowsThem(t *testing.T) {
	root := project(t)
	outside := t.TempDir()
	write(t, outside, "secret.txt", "secret")
	if err := os.Symlink(outside, filepath.Join(root, "src", "public", "link")); err != nil {
		t.Skip("symlinks not supported")
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "src", "public", "secret.txt")); err != nil {
		t.Skip("symlinks not supported")
	}
	got := resolvePaths(t, root, utils.GlobRules{{Source: "./src/public", Dest: "./"}})
	for _, p := range got {
		if strings.Contains(p, "secret") || strings.HasPrefix(p, "link") {
			t.Fatalf("a link was followed: %v", got)
		}
	}
	// an explicit link to a folder is the user's choice and is followed
	if err := os.Symlink(outside, filepath.Join(root, "explicit")); err == nil {
		if got := resolvePaths(t, root, utils.GlobRules{{Source: "./explicit", Dest: "./"}}); !reflect.DeepEqual(got, []string{"secret.txt"}) {
			t.Fatalf("explicit link: %v", got)
		}
	}
}

func TestResolveNeverUploadsTheIgnoredConfigFile(t *testing.T) {
	root := t.TempDir()
	write(t, root, "futrou.json", "{}")
	write(t, root, "index.html", "i")
	resolved, err := Resolve(root, Selection{NoHTMLToFolders: true, Ignore: []string{filepath.Join(root, "futrou.json")}})
	if err != nil || !reflect.DeepEqual(paths(resolved.Files), []string{"index.html"}) {
		t.Fatalf("paths = %v err = %v", paths(resolved.Files), err)
	}
}

func TestResolveRefusesNamesTheAPIRejects(t *testing.T) {
	root := t.TempDir()
	write(t, root, "ok.html", "x")
	write(t, root, "bad:name.html", "x")
	_, err := Resolve(root, Selection{})
	if err == nil || !strings.Contains(err.Error(), "bad:name.html") {
		t.Fatalf("err = %v", err)
	}
	// a destination can rename it into something valid
	write(t, root, "src/a:b.txt", "x")
	if _, err := Resolve(root, Selection{Rules: utils.GlobRules{{Source: "src/a:b.txt", Dest: "ab.txt"}}}); err != nil {
		t.Fatalf("renamed: %v", err)
	}
}

func TestResolveReportsUnreadableFilesAndFolders(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read everything")
	}
	root := project(t)
	locked := filepath.Join(root, "src", "public", "app.js")
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(locked, 0o644)
	_, err := Resolve(root, Selection{Rules: utils.GlobRules{{Source: "./src/public", Dest: "./"}}})
	if err == nil || !strings.Contains(err.Error(), "app.js") || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("an unreadable file must be named: %v", err)
	}
	os.Chmod(locked, 0o644)

	dir := filepath.Join(root, "src", "public", "images")
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o755)
	_, err = Resolve(root, Selection{Rules: utils.GlobRules{{Source: "./src/public", Dest: "./"}}})
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("an unreadable folder must fail: %v", err)
	}
}

func TestResolveHTMLToFolders(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{
		"index.html", "about.html", "contact.htm", "blog/post.html", "blog/index.html", "docs/index.htm",
		"taken.html", "taken/index.html", "blocked.html", "blocked", "UPPER.HTML", "a.b.html", "404.html", "style.css", "img/logo.png",
	} {
		write(t, root, f, f)
	}
	resolved, err := Resolve(root, Selection{})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, f := range resolved.Files {
		got[f.Path] = filepath.Base(f.Abs)
	}
	for copied, from := range map[string]string{
		"about/index.html":     "about.html",
		"contact/index.html":   "contact.htm",
		"blog/post/index.html": "post.html",
		"docs/index.html":      "index.htm", // index.htm becomes index.html in its folder
		"UPPER/index.html":     "UPPER.HTML",
		"a.b/index.html":       "a.b.html",
		"404/index.html":       "404.html",
	} {
		if got[copied] != from {
			t.Errorf("%s should be a copy of %s, got %q", copied, from, got[copied])
		}
	}
	for _, original := range []string{"about.html", "contact.htm", "blog/post.html", "docs/index.htm", "index.html", "blog/index.html", "style.css", "img/logo.png"} {
		if _, ok := got[original]; !ok {
			t.Errorf("the original %s is no longer there", original)
		}
	}
	if got["taken/index.html"] != "index.html" {
		t.Errorf("an existing taken/index.html must stay the file it was: %q", got["taken/index.html"])
	}
	if _, ok := got["blocked/index.html"]; ok {
		t.Error("blocked.html cannot become blocked/index.html: a file named blocked exists")
	}
	for _, p := range []string{"index/index.html", "blog/index/index.html", "style/index.html"} {
		if _, ok := got[p]; ok {
			t.Errorf("%s must not exist", p)
		}
	}
}

func TestResolveHTMLToFoldersCopiesTheContent(t *testing.T) {
	root := t.TempDir()
	write(t, root, "about.html", "<h1>about</h1>")
	resolved, err := Resolve(root, Selection{})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range resolved.Files {
		if b, _ := os.ReadFile(f.Abs); string(b) != "<h1>about</h1>" {
			t.Fatalf("%s holds %q", f.Path, b)
		}
	}
}

func TestResolveHTMLToFoldersCanBeSwitchedOff(t *testing.T) {
	root := t.TempDir()
	write(t, root, "about.html", "x")
	resolved, err := Resolve(root, Selection{NoHTMLToFolders: true})
	if err != nil || !reflect.DeepEqual(paths(resolved.Files), []string{"about.html"}) {
		t.Fatalf("paths = %v err = %v", paths(resolved.Files), err)
	}
}

func TestResolveHTMLToFoldersWorksOnDestinationPaths(t *testing.T) {
	root := t.TempDir()
	write(t, root, "pages/team.html", "t")
	resolved, err := Resolve(root, Selection{Rules: utils.GlobRules{{Source: "pages", Dest: "company"}}})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"company/team.html", "company/team/index.html"}; !reflect.DeepEqual(paths(resolved.Files), want) {
		t.Fatalf("paths = %v, want %v", paths(resolved.Files), want)
	}
}

func TestResolveReadsTheRedirectsFileAndDoesNotUploadIt(t *testing.T) {
	root := t.TempDir()
	write(t, root, "public/index.html", "i")
	write(t, root, "public/_redirects", "# comment\n/old /new 301\n/blog/* /news/:splat\n/bad\n")
	resolved, err := Resolve(root, Selection{Rules: utils.GlobRules{{Source: "public", Dest: "./"}}, NoHTMLToFolders: true})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(paths(resolved.Files), []string{"index.html"}) {
		t.Fatalf("_redirects must not be uploaded: %v", paths(resolved.Files))
	}
	if len(resolved.Redirects) != 2 || len(resolved.Warnings) != 1 || !strings.Contains(resolved.Warnings[0], "line 4") {
		t.Fatalf("redirects = %v warnings = %v", resolved.Redirects, resolved.Warnings)
	}
}

func TestResolveRedirectsFileMustBeAtTheRootOfTheWebsite(t *testing.T) {
	root := t.TempDir()
	write(t, root, "public/index.html", "i")
	write(t, root, "public/_redirects", "/old /new\n")
	// kept under its path (not at the root), so it is an ordinary file
	resolved, err := Resolve(root, Selection{Rules: utils.GlobRules{{Source: "public/index.html"}, {Source: "public/_redirects"}}, NoHTMLToFolders: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Redirects) != 0 || !contains(paths(resolved.Files), "public/_redirects") {
		t.Fatalf("redirects = %v files = %v", resolved.Redirects, paths(resolved.Files))
	}
	// excluded, it configures nothing
	resolved, err = Resolve(root, Selection{Rules: utils.GlobRules{{Source: "public", Dest: "./"}, {Source: "public/_redirects", Exclude: true}}, NoHTMLToFolders: true})
	if err != nil || len(resolved.Redirects) != 0 {
		t.Fatalf("redirects = %v err = %v", resolved.Redirects, err)
	}
	// renamed into place, it is the file
	write(t, root, "conf/redirects.txt", "/a /b\n")
	resolved, err = Resolve(root, Selection{Rules: utils.GlobRules{{Source: "public", Dest: "./"}, {Source: "conf/redirects.txt", Dest: "_redirects"}}, NoHTMLToFolders: true})
	if err != nil || len(resolved.Redirects) != 1 {
		t.Fatalf("redirects = %v err = %v", resolved.Redirects, err)
	}
}

func TestValidSitePath(t *testing.T) {
	for p, ok := range map[string]bool{
		"index.html": true, "a/b/c.txt": true, "with space/x.txt": true, "ünï.txt": true,
		"": false, "/abs": false, "a//b": false, "a/../b": false, "./a": false, " lead.txt": false,
		"a:b": false, "a*b": false, "a?b": false, `a"b`: false, "a<b": false, "a|b": false, "a\\b": false, "a\x01b": false,
	} {
		if reason := ValidSitePath(p); (reason == "") != ok {
			t.Errorf("ValidSitePath(%q) = %q, want ok=%v", p, reason, ok)
		}
	}
}

// ---- hidden files and source maps are never uploaded by accident ----

func secretsTree(t *testing.T) string {
	root := t.TempDir()
	for _, f := range []string{
		"dist/index.html", "dist/app.js", "dist/app.js.map", "dist/styles.css.MAP",
		"dist/.env", "dist/.htpasswd", "dist/img/logo.png", "dist/img/.DS_Store",
		"dist/.well-known/security.txt", "dist/.cache/x/y.txt", "dist/deep/.secret/key.pem", "dist/deep/ok.txt",
		".env", "notes.map",
	} {
		write(t, root, f, f)
	}
	return root
}

func TestResolveNeverUploadsHiddenFilesOrSourceMapsFoundInAFolder(t *testing.T) {
	root := secretsTree(t)
	for name, rules := range map[string]utils.GlobRules{
		"a folder":            {{Source: "./dist"}},
		"a folder to a place": {{Source: "./dist", Dest: "site"}},
		"the whole folder":    nil,
		"a recursive glob":    {{Source: "./dist/**"}},
		"a glob by extension": {{Source: "./dist/**/*.{html,js,png,txt}"}},
	} {
		t.Run(name, func(t *testing.T) {
			resolved, err := Resolve(root, Selection{Rules: rules, NoHTMLToFolders: true})
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range paths(resolved.Files) {
				for _, segment := range strings.Split(p, "/") {
					if strings.HasPrefix(segment, ".") {
						t.Errorf("%s: a hidden path was selected", p)
					}
				}
				if strings.HasSuffix(strings.ToLower(p), ".map") {
					t.Errorf("%s: a source map was selected", p)
				}
			}
			if len(resolved.Files) == 0 {
				t.Fatal("the visible files are still selected")
			}
		})
	}
}

func TestResolveKeepsTheVisibleFilesAndCountsWhatItLeftOut(t *testing.T) {
	root := secretsTree(t)
	resolved, err := Resolve(root, Selection{Rules: utils.GlobRules{{Source: "./dist", Dest: "./"}}, NoHTMLToFolders: true})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"app.js", "deep/ok.txt", "img/logo.png", "index.html"}
	if !reflect.DeepEqual(paths(resolved.Files), want) {
		t.Fatalf("paths = %v, want %v", paths(resolved.Files), want)
	}
	// .env .htpasswd img/.DS_Store .well-known/security.txt (hidden folders are not walked, so not counted)
	if resolved.Skipped < 4 {
		t.Fatalf("skipped = %d, want the hidden and .map files that were found", resolved.Skipped)
	}
}

func TestResolveASourceThatNamesAHiddenThingTakesIt(t *testing.T) {
	root := secretsTree(t)
	for name, tc := range map[string]struct {
		rules utils.GlobRules
		want  []string
	}{
		"a hidden file":                   {utils.GlobRules{{Source: "./dist/.env"}}, []string{"dist/.env"}},
		"a hidden file to a place":        {utils.GlobRules{{Source: "./dist/.htpasswd", Dest: "auth.txt"}}, []string{"auth.txt"}},
		"a hidden folder":                 {utils.GlobRules{{Source: "./dist/.well-known"}}, []string{"security.txt"}},
		"a hidden folder to the root":     {utils.GlobRules{{Source: "./dist/.well-known", Dest: ".well-known"}}, []string{".well-known/security.txt"}},
		"a glob inside a hidden folder":   {utils.GlobRules{{Source: "./dist/.well-known/*.txt", Dest: "./"}}, []string{"security.txt"}},
		"a source map by name":            {utils.GlobRules{{Source: "./dist/app.js.map"}}, []string{"dist/app.js.map"}},
		"source maps by a glob naming it": {utils.GlobRules{{Source: "./dist/*.map", Dest: "maps"}}, []string{"maps/app.js.map"}},
	} {
		t.Run(name, func(t *testing.T) {
			resolved, err := Resolve(root, Selection{Rules: tc.rules, NoHTMLToFolders: true})
			if err != nil {
				t.Fatal(err)
			}
			if got := paths(resolved.Files); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("paths = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestResolveAHiddenFolderNamedAsASourceStillSkipsHiddenThingsInsideIt(t *testing.T) {
	root := t.TempDir()
	write(t, root, "pub/.well-known/ok.txt", "o")
	write(t, root, "pub/.well-known/.private", "p")
	resolved, err := Resolve(root, Selection{Rules: utils.GlobRules{{Source: "./pub/.well-known", Dest: "./"}}, NoHTMLToFolders: true})
	if err != nil || !reflect.DeepEqual(paths(resolved.Files), []string{"ok.txt"}) {
		t.Fatalf("paths = %v err = %v", paths(resolved.Files), err)
	}
}

func TestResolveTheDotEnvAtTheRootOfTheConfigFolderIsNeverUploadedByTheDefaultSelection(t *testing.T) {
	root := t.TempDir()
	write(t, root, "index.html", "i")
	write(t, root, ".env", "SECRET=1")
	write(t, root, ".env.production", "SECRET=2")
	write(t, root, "app.js.map", "m")
	resolved, err := Resolve(root, Selection{NoHTMLToFolders: true})
	if err != nil || !reflect.DeepEqual(paths(resolved.Files), []string{"index.html"}) {
		t.Fatalf("paths = %v err = %v", paths(resolved.Files), err)
	}
}

func TestResolveAnExcludeStillWorksOnTopOfTheDefaults(t *testing.T) {
	root := secretsTree(t)
	resolved, err := Resolve(root, Selection{Rules: utils.GlobRules{{Source: "./dist", Dest: "./"}, {Source: "./dist/img", Exclude: true}}, NoHTMLToFolders: true})
	if err != nil || !reflect.DeepEqual(paths(resolved.Files), []string{"app.js", "deep/ok.txt", "index.html"}) {
		t.Fatalf("paths = %v err = %v", paths(resolved.Files), err)
	}
}

func TestResolveWhenOnlyHiddenFilesAreFoundThereIsNothingToUpload(t *testing.T) {
	root := t.TempDir()
	write(t, root, "pub/.env", "x")
	write(t, root, "pub/a.map", "m")
	_, err := Resolve(root, Selection{Rules: utils.GlobRules{{Source: "./pub", Dest: "./"}}})
	if err == nil || !strings.Contains(err.Error(), "matches no files") {
		t.Fatalf("err = %v", err)
	}
}
