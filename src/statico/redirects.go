package statico

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// MaxRedirects is the most rules a version holds by default.
const MaxRedirects = 2000

var (
	placeholderPattern = regexp.MustCompile(`:([A-Za-z_][A-Za-z0-9_]*)`)
	redirectStatuses   = map[int]bool{301: true, 302: true, 303: true, 307: true, 308: true}
)

// Redirect is a redirect declared in the config: From is matched against the
// request path (default every path) with the syntax of a _redirects source, To
// is where it goes, and StatusCode is 301, 302 (default), 303, 307 or 308.
type Redirect struct {
	From       string `json:"from,omitempty"`
	To         string `json:"to"`
	StatusCode int    `json:"statusCode,omitempty"`
}

// Rule converts the redirect into the rule of the node, as ParseRedirects does for a line.
func (r Redirect) Rule() (map[string]interface{}, error) {
	if strings.TrimSpace(r.To) == "" || strings.ContainsAny(r.To, " \t\r\n") {
		return nil, fmt.Errorf("a redirect needs a \"to\" without spaces")
	}
	from := r.From
	if from == "" {
		from = "/*" // no source: every path
	}
	status := r.StatusCode
	if status == 0 {
		status = 302
	}
	if !redirectStatuses[status] {
		return nil, fmt.Errorf("the status %d is not one of 301, 302, 303, 307, 308", status)
	}
	return convertRedirect(from, r.To, status)
}

// RedirectRules converts the redirects of a config, in order.
func RedirectRules(list []Redirect) ([]map[string]interface{}, error) {
	var rules []map[string]interface{}
	for i, redirect := range list {
		rule, err := redirect.Rule()
		if err != nil {
			return nil, fmt.Errorf("redirect %d: %w", i+1, err)
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

// ParseRedirects reads a _redirects file in the syntax of Cloudflare Pages
// (https://developers.cloudflare.com/pages/configuration/redirects/): one
// "source destination [status]" per line, "#" comments, the status 302 unless
// given, :name placeholders (one path segment) and a * splat (the rest of the
// path, written :splat in the destination). It returns the rules of the version
// in the format of the node, in file order, and a warning for every line that
// cannot be converted (those lines are skipped, like Cloudflare skips invalid
// ones): a status of 200 (proxying), a query string or fragment in the source,
// a second splat, an unknown status.
func ParseRedirects(content string) ([]map[string]interface{}, []string) {
	var rules []map[string]interface{}
	var warnings []string
	for number, line := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		warn := func(format string, args ...interface{}) {
			warnings = append(warnings, fmt.Sprintf("_redirects line %d: %s", number+1, fmt.Sprintf(format, args...)))
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || len(fields) > 3 {
			warn("expected \"source destination [status]\", got %d value(s)", len(fields))
			continue
		}
		status := 302
		if len(fields) == 3 {
			code, err := strconv.Atoi(fields[2])
			switch {
			case err != nil:
				warn("the status %q is not a number", fields[2])
				continue
			case code == 200:
				warn("the status 200 (proxying) is not supported; use a redirect status")
				continue
			case !redirectStatuses[code]:
				warn("the status %d is not one of 301, 302, 303, 307, 308", code)
				continue
			}
			status = code
		}
		rule, err := convertRedirect(fields[0], fields[1], status)
		if err != nil {
			warn("%v", err)
			continue
		}
		rules = append(rules, rule)
	}
	if len(rules) > MaxRedirects {
		warnings = append(warnings, fmt.Sprintf("_redirects has %d rules; a version holds at most %d by default", len(rules), MaxRedirects))
	}
	return rules, warnings
}

// convertRedirect makes the node's rule out of one line. A source without
// placeholders is a plain path; with them it is a regular expression whose
// groups are named after the placeholders, and the destination refers to them
// as ${name}.
func convertRedirect(source, dest string, status int) (map[string]interface{}, error) {
	host := ""
	path := source
	if scheme := strings.Index(source, "://"); scheme > 0 {
		rest := source[scheme+3:]
		slash := strings.IndexByte(rest, '/')
		if slash < 0 {
			host, path = rest, "/"
		} else {
			host, path = rest[:slash], rest[slash:]
		}
		if host == "" || strings.ContainsAny(host, ":*") && !regexp.MustCompile(`^[^:*]+:[0-9]+$`).MatchString(host) {
			return nil, fmt.Errorf("the host of %q cannot hold placeholders", source)
		}
		if i := strings.IndexByte(host, ':'); i > 0 {
			host = host[:i] // the port is not part of the host a request is matched on
		}
	}
	if !strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("the source %q must start with a slash", source)
	}
	if strings.ContainsAny(path, "?") {
		return nil, fmt.Errorf("the source %q holds a query string, which cannot be matched", source)
	}
	if strings.Contains(path, "#") {
		return nil, fmt.Errorf("the source %q holds a fragment", source)
	}

	condition := map[string]interface{}{}
	if host != "" {
		condition["host"] = host
	}
	to := dest
	if !strings.Contains(path, "*") && !placeholderPattern.MatchString(path) {
		condition["path"] = path
	} else {
		pattern, names, err := pathToPattern(path)
		if err != nil {
			return nil, err
		}
		condition["path"] = "regex:" + pattern
		to = fillPlaceholders(dest, names)
	}
	return map[string]interface{}{
		"if":   condition,
		"then": []interface{}{map[string]interface{}{"type": "redirect", "to": to, "statusCode": status}},
	}, nil
}

// pathToPattern turns "/a/:id/*" into an anchored regular expression with the
// named groups id and splat, and reports the names it defined.
func pathToPattern(path string) (string, map[string]bool, error) {
	names := map[string]bool{}
	var b strings.Builder
	b.WriteByte('^')
	endsWithSplat := false
	for i := 0; i < len(path); {
		switch c := path[i]; {
		case c == '*':
			if names["splat"] {
				return "", nil, fmt.Errorf("the source %q has more than one splat", path)
			}
			names["splat"] = true
			b.WriteString(`(?P<splat>.*)`)
			endsWithSplat = i == len(path)-1
			i++
		case c == ':' && placeholderPattern.MatchString(path[i:]) && placeholderPattern.FindStringIndex(path[i:])[0] == 0:
			name := placeholderPattern.FindStringSubmatch(path[i:])[1]
			if names[name] {
				return "", nil, fmt.Errorf("the placeholder :%s is used twice in %q", name, path)
			}
			names[name] = true
			b.WriteString(`(?P<` + name + `>[^/]+)`)
			i += 1 + len(name)
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
			i++
		}
	}
	// "/about" and "/about/" are the same page, as with a plain path
	if !endsWithSplat && !strings.HasSuffix(path, "/") {
		b.WriteString(`/?`)
	}
	b.WriteByte('$')
	if len(b.String()) > 1024 {
		return "", nil, fmt.Errorf("the source %q is too long", path)
	}
	if _, err := regexp.Compile(b.String()); err != nil {
		return "", nil, fmt.Errorf("the source %q is not valid: %v", path, err)
	}
	return b.String(), names, nil
}

// fillPlaceholders rewrites :name and :splat in a destination as ${name}, for
// the names the source defined; a dollar sign stays a dollar sign.
func fillPlaceholders(dest string, names map[string]bool) string {
	dest = strings.ReplaceAll(dest, "$", "$$")
	return placeholderPattern.ReplaceAllStringFunc(dest, func(token string) string {
		if name := token[1:]; names[name] {
			return "${" + name + "}"
		}
		return token
	})
}

// MergeRedirects puts the rules a config declares before those of a _redirects file.
func MergeRedirects(declared, fromFile []map[string]interface{}) []map[string]interface{} {
	if len(declared) == 0 && len(fromFile) == 0 {
		return nil
	}
	merged := make([]map[string]interface{}, 0, len(declared)+len(fromFile))
	merged = append(merged, declared...)
	return append(merged, fromFile...)
}
