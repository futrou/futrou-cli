package validators

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// ConfigValidator validates the top-level declarative Futrou config. It is a
// constructed validator so commands and future config consumers share exactly
// the same schema and fluent validation rules.
var ConfigValidator = initConfigValidator()

func initConfigValidator() *Validator {
	v := NewValidator().Description("Declarative Futrou project configuration.")
	v.Field("$schema").Description("URL of the Futrou JSON Schema used by editors for validation and completion.").Optional().String().URL()
	v.Field("workspace").Description("Workspace name or ID that owns this project.").Optional().String().MinLength(1).MaxLength(255)
	v.Field("project").Description("Project name or ID used as the deployment target.").Optional().String().MinLength(1).MaxLength(255)
	v.Field("installCommand").Description("Shell command that installs what the build needs (for example `npm ci`). It runs in the folder of this file, once on a device before the first `futrou deploy` or `futrou statico up`, and again only when the command changes.").Optional().Custom(shellCommand)
	v.Field("buildCommand").Description("Shell command that builds the project (for example `npm run build`). It runs in the folder of this file before every `futrou deploy` and `futrou statico up`, and a failure stops the deploy.").Optional().Custom(shellCommand)
	v.Field("region").Description("Region name or ID applied to every resource in the project that does not set its own regionId.").Optional().String().MinLength(1).MaxLength(255)
	v.Field("networks").Description("Private networks managed by this project; serverlets join them by name.").Optional().Array(initNetworkValidator()).Custom(uniqueNames("name"))
	v.Field("serverlets").Description("Serverlets to create or update in this project.").Optional().Array(initServerletValidator()).Custom(uniqueNames("name")).Custom(validateNeeds)
	v.Field("dns").Description("DNS zones and records managed by this project.").Optional().Array(initDNSValidator()).Custom(uniqueNames("domain", "name"))
	v.Field("proxies").Description("HTTP, TCP, or UDP proxies managed by this project.").Optional().Array(initProxyValidator()).Custom(uniqueNames("domain"))
	v.Field("storages").Description("Persistent storage volumes available to the project.").Optional().Array(initStorageValidator()).Custom(uniqueNames("name"))
	v.Field("crons").Description("Scheduled HTTP or code jobs for the project.").Optional().Array(initCronValidator()).Custom(uniqueNames("name"))
	v.Field("statico").Description("Static websites. `futrou deploy` creates each one and uploads its files as a new version, which it activates when something changed.").Optional().Array(initStaticoValidator()).Custom(uniqueNames("name"))
	v.Field("template").Description("Presentation metadata for the Futrou template store (title, description, category, tags, icon, ...). Not used by the CLI.").Optional().Custom(isObject)
	v.Field("locks").Description("Internal stable resource identifiers maintained by the CLI.").Optional().Custom(stringMap)
	return v
}

func initResourceValidator() *Validator {
	v := NewValidator().Description("Common Futrou API resource fields.")
	v.Field("id").Description("Futrou resource identifier.").Optional().String().MinLength(1).MaxLength(255)
	v.Field("name").Description("Resource name.").Optional().String().MinLength(1).MaxLength(255)
	v.Field("displayName").Description("Human-readable resource name.").Optional().String().MinLength(1).MaxLength(255)
	v.Field("domain").Description("Domain name assigned to the resource.").Optional().String().MinLength(1).MaxLength(255)
	return v
}

func initPortValidator() *Validator {
	v := NewValidator().Description("A port the serverlet listens on.")
	v.Field("port").Description("Port number (1–65535).").Required().Integer().Range(1, 65535)
	v.Field("protocol").Description("Port protocol.").Optional().String().Enum("tcp", "udp", "http")
	return v
}

// commandString accepts a non-empty string, or an array of non-empty strings
// that the config loader joins with spaces.
func commandString(value interface{}) error {
	if text, ok := value.(string); ok {
		if strings.TrimSpace(text) == "" {
			return fmt.Errorf("must not be empty")
		}
		return nil
	}
	if _, ok := value.([]interface{}); ok {
		return stringList(value)
	}
	return fmt.Errorf("must be a string or an array of strings")
}

// shellCommand accepts a non-blank string without a null character, up to 4096 characters.
func shellCommand(value interface{}) error {
	text, ok := value.(string)
	switch {
	case !ok:
		return fmt.Errorf("must be a string")
	case strings.TrimSpace(text) == "":
		return fmt.Errorf("must not be empty")
	case len(text) > 4096:
		return fmt.Errorf("must be at most 4096 characters")
	case strings.ContainsRune(text, 0):
		return fmt.Errorf("must not contain a null character")
	}
	return nil
}

// isObject accepts any JSON object.
func isObject(value interface{}) error {
	if _, ok := value.(map[string]interface{}); !ok {
		return fmt.Errorf("must be an object")
	}
	return nil
}

// NeedStatuses are the conditions a serverlet can wait for in "needs".
var NeedStatuses = []string{"started", "healthy"}

// ParseNeed splits a "needs" entry, "db" or "db:healthy", into the serverlet
// name and the optional status.
func ParseNeed(need string) (name, status string, err error) {
	name, status, found := strings.Cut(strings.TrimSpace(need), ":")
	name, status = strings.TrimSpace(name), strings.TrimSpace(status)
	if name == "" {
		return "", "", fmt.Errorf("%q needs a serverlet name", need)
	}
	if !found {
		return name, "", nil
	}
	for _, allowed := range NeedStatuses {
		if status == allowed {
			return name, status, nil
		}
	}
	return "", "", fmt.Errorf("%q has the unknown status %q (use %s)", need, status, strings.Join(NeedStatuses, " or "))
}

// needsList accepts an array of "name" or "name:status" strings.
func needsList(value interface{}) error {
	if err := stringList(value); err != nil {
		return err
	}
	for _, item := range value.([]interface{}) {
		if _, _, err := ParseNeed(item.(string)); err != nil {
			return err
		}
	}
	return nil
}

// validateNeeds checks the "needs" of all serverlets together: every needed
// serverlet must be declared, none may need itself, and the dependencies must
// not form a cycle.
func validateNeeds(value interface{}) error {
	items, ok := value.([]interface{})
	if !ok {
		return nil
	}
	needs := map[string][]string{}
	for _, item := range items {
		object, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := object["name"].(string)
		if name == "" {
			continue
		}
		list, _ := object["needs"].([]interface{})
		for _, entry := range list {
			text, ok := entry.(string)
			if !ok {
				continue
			}
			needed, _, err := ParseNeed(text)
			if err != nil {
				return fmt.Errorf("serverlet %q: %w", name, err)
			}
			needs[name] = append(needs[name], needed)
		}
		if _, seen := needs[name]; !seen {
			needs[name] = nil
		}
	}
	for name, list := range needs {
		for _, needed := range list {
			if needed == name {
				return fmt.Errorf("serverlet %q cannot need itself", name)
			}
			if _, declared := needs[needed]; !declared {
				return fmt.Errorf("serverlet %q needs %q, which is not declared in serverlets", name, needed)
			}
		}
	}
	state := map[string]int{} // 1 = visiting, 2 = done
	var visit func(name string, path []string) error
	visit = func(name string, path []string) error {
		if state[name] == 2 {
			return nil
		}
		if state[name] == 1 {
			return fmt.Errorf("serverlet needs form a cycle: %s", strings.Join(append(path, name), " -> "))
		}
		state[name] = 1
		for _, needed := range needs[name] {
			if err := visit(needed, append(path, name)); err != nil {
				return err
			}
		}
		state[name] = 2
		return nil
	}
	names := make([]string, 0, len(needs))
	for name := range needs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := visit(name, nil); err != nil {
			return err
		}
	}
	return nil
}

// stringList accepts an array whose items are all non-empty strings.
func stringList(value interface{}) error {
	items, ok := value.([]interface{})
	if !ok {
		return fmt.Errorf("must be an array")
	}
	for i, item := range items {
		if text, ok := item.(string); !ok || strings.TrimSpace(text) == "" {
			return fmt.Errorf("item %d must be a non-empty string", i)
		}
	}
	return nil
}

// stringMap accepts an object whose values are all strings.
func stringMap(value interface{}) error {
	object, ok := value.(map[string]interface{})
	if !ok {
		return fmt.Errorf("must be an object")
	}
	for key, item := range object {
		if _, ok := item.(string); !ok {
			return fmt.Errorf("%s must be a string", key)
		}
	}
	return nil
}

// numberMap accepts an object whose values are all numbers.
func numberMap(value interface{}) error {
	object, ok := value.(map[string]interface{})
	if !ok {
		return fmt.Errorf("must be an object")
	}
	for key, item := range object {
		if _, ok := item.(float64); !ok {
			return fmt.Errorf("%s must be a number", key)
		}
	}
	return nil
}

// uniqueNames requires every item to carry a non-empty value for one of keys
// and rejects duplicates, since the deployer identifies resources by it.
func uniqueNames(keys ...string) func(interface{}) error {
	return func(value interface{}) error {
		items, ok := value.([]interface{})
		if !ok {
			return nil // the array rule reports the wrong type
		}
		seen := map[string]int{}
		for i, item := range items {
			object, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			name := ""
			for _, key := range keys {
				if text, _ := object[key].(string); text != "" {
					name = text
					break
				}
			}
			if name == "" {
				return fmt.Errorf("item %d needs %s", i, strings.Join(keys, " or "))
			}
			if first, dup := seen[name]; dup {
				return fmt.Errorf("item %d duplicates item %d (%q)", i, first, name)
			}
			seen[name] = i
		}
		return nil
	}
}

func initServerletValidator() *Validator {
	v := initResourceValidator()
	v.Field("image").Description("Container image to run.").Optional().String().MinLength(1)
	v.Field("serverletPlanId").Description("Plan ID that determines serverlet sizing.").Optional().String().MinLength(1).MaxLength(255)
	v.Field("createdAt").Optional().String()
	v.Field("updatedAt").Optional().String()
	v.Field("status").Optional().String()
	v.Field("workspaceId").Optional().String().MinLength(1).MaxLength(255)
	v.Field("projectId").Optional().String().MinLength(1).MaxLength(255)
	v.Field("regionId").Optional().String().MinLength(1).MaxLength(255)
	v.Field("arch").Optional()
	v.Field("env").Description("Environment variables, name to value.").Optional().Custom(stringMap)
	v.Field("mounts").Description("Storage mounts, keyed by mount path to storage ID.").Optional().Custom(stringMap)
	v.Field("ports").Description("Ports the serverlet listens on.").Optional().Array(initPortValidator())
	v.Field("networks").Description("Networks (names or IDs) the serverlet is attached to.").Optional().Array().Custom(stringList)
	v.Field("minInstances").Description("Minimum number of running instances.").Optional().Integer().Min(0)
	v.Field("maxInstances").Description("Maximum number of running instances.").Optional().Integer().Min(0)
	v.Field("user").Description("User the entrypoint runs as.").Optional().String()
	v.Field("needs").Description("Serverlets this one waits for, as \"name\" or \"name:status\" where status is started or healthy.").Optional().Array().Custom(needsList)
	v.Field("healthcheck").Description("Healthcheck command or URL.").Optional().String()
	v.Field("healthcheckInterval").Description("Seconds between healthchecks.").Optional().Number().Min(0)
	v.Field("healthcheckTimeout").Description("Seconds before a healthcheck times out.").Optional().Number().Min(0)
	v.Field("healthcheckRetries").Description("Failed healthchecks before the instance is unhealthy.").Optional().Integer().Min(0)
	v.Field("startTimeout").Description("Seconds to wait for an instance to start.").Optional().Number().Min(0)
	v.Field("stopTimeout").Description("Seconds to wait for an instance to stop.").Optional().Number().Min(0)
	v.Field("strategy").Description("Rollout strategy settings.").Optional().Custom(isObject)
	v.Field("scaling").Description("Scaling settings, name to number.").Optional().Custom(numberMap)
	v.Field("entrypoint").Description("Command or path that replaces the image's default start command, as one string. An array of strings is accepted and joined with spaces.").Optional().Custom(commandString)
	v.Field("cwd").Description("Working directory for the entrypoint.").Optional().String()
	return v
}

func initProxyValidator() *Validator {
	v := initResourceValidator()
	v.Field("type").Optional().String().Enum("http", "tcp", "udp")
	v.Field("target").Optional().String().MinLength(1)
	v.Field("port").Description("Public proxy port (1–65535).").Optional().Integer().Range(1, 65535)
	v.Field("proxyPlanId").Optional().String().MinLength(1).MaxLength(255)
	v.Field("regionId").Optional().String().MinLength(1).MaxLength(255)
	v.Field("enforceHttps").Optional().Bool()
	v.Field("followRedirects").Optional().Bool()
	v.Field("preserveHeaders").Optional().Bool()
	v.Field("preserveHost").Optional().Bool()
	v.Field("preservePath").Optional().Bool()
	v.Field("preserveQuery").Optional().Bool()
	v.Field("verifyTls").Optional().Bool()
	v.Field("isVerified").Optional().Bool()
	v.Field("status").Optional().String()
	v.Field("strategy").Optional().String().Enum("round-robin", "primary-failover")
	v.Field("rules").Description("Routing rules.").Optional().Array()
	v.Field("variant").Description("How www is handled.").Optional().String().Enum("auto", "www", "non-www")
	v.Field("workspaceId").Optional().String().MinLength(1).MaxLength(255)
	v.Field("projectId").Optional().String().MinLength(1).MaxLength(255)
	v.Field("createdAt").Optional().String()
	v.Field("updatedAt").Optional().String()
	return v
}

func initDNSValidator() *Validator {
	v := initResourceValidator()
	v.Field("ttl").Description("DNS time-to-live in seconds.").Optional().Integer().Range(0, math.MaxInt32)
	v.Field("priority").Description("DNS record priority.").Optional().Integer().Range(0, 65535)
	v.Field("workspaceId").Optional().String().MinLength(1).MaxLength(255)
	v.Field("projectId").Optional().String().MinLength(1).MaxLength(255)
	v.Field("records").Optional().Array(initDNSRecordValidator())
	return v
}

func initDNSRecordValidator() *Validator {
	v := initResourceValidator()
	v.Field("type").Optional().String()
	v.Field("value").Optional().String()
	v.Field("ttl").Optional().Integer().Range(0, math.MaxInt32)
	v.Field("priority").Optional().Integer().Range(0, 65535)
	return v
}

func initNetworkValidator() *Validator {
	v := initResourceValidator()
	v.Field("ipv4").Description("IPv4 CIDR of the network.").Optional().String()
	v.Field("ipv6").Description("IPv6 CIDR of the network.").Optional().String()
	v.Field("internal").Description("Internal-only network without outbound access.").Optional().Bool()
	v.Field("workspaceId").Optional().String().MinLength(1).MaxLength(255)
	v.Field("projectId").Optional().String().MinLength(1).MaxLength(255)
	v.Field("regionId").Optional().String().MinLength(1).MaxLength(255)
	v.Field("createdAt").Optional().String()
	v.Field("updatedAt").Optional().String()
	return v
}

func initStorageValidator() *Validator {
	v := initResourceValidator()
	v.Field("storagePlanId").Description("Plan ID that determines storage sizing.").Optional().String().MinLength(1).MaxLength(255)
	v.Field("workspaceId").Optional().String().MinLength(1).MaxLength(255)
	v.Field("projectId").Optional().String().MinLength(1).MaxLength(255)
	v.Field("regionId").Optional().String().MinLength(1).MaxLength(255)
	v.Field("public").Optional().Bool()
	v.Field("createdAt").Optional().String()
	v.Field("updatedAt").Optional().String()
	return v
}

func initCronValidator() *Validator {
	v := initResourceValidator()
	v.Field("body").Optional().String()
	v.Field("code").Optional().String()
	v.Field("cronPlanId").Optional().String()
	v.Field("enabled").Optional().Bool()
	v.Field("headers").Optional().Custom(stringMap)
	v.Field("method").Optional().String()
	v.Field("type").Description("Cron type.").Optional().String().Enum("simple", "code")
	v.Field("projectId").Optional().String()
	v.Field("regionId").Optional().String()
	v.Field("schedule").Optional().String()
	v.Field("url").Optional().String().URL()
	v.Field("workspaceId").Optional().String()
	v.Field("createdAt").Optional().String().DateTime()
	v.Field("startedAt").Optional().String().DateTime()
	v.Field("updatedAt").Optional().String().DateTime()
	v.Field("cronPlan").Optional()
	v.Field("project").Optional()
	v.Field("region").Optional()
	v.Field("workspace").Optional()
	return v
}

// includeRule checks one include entry: its value is true (keep the path),
// false (exclude) or a destination in the website.
func includeRule(source string, value interface{}) error {
	if strings.TrimSpace(source) == "" {
		return fmt.Errorf("an include needs a source")
	}
	if strings.ContainsRune(source, 0) {
		return fmt.Errorf("include %q must not contain a null character", source)
	}
	switch v := value.(type) {
	case bool:
		return nil
	case string:
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("include %q: the destination must not be empty (use true to keep the path)", source)
		}
		if strings.ContainsRune(v, 0) {
			return fmt.Errorf("include %q: the destination must not contain a null character", source)
		}
		clean := strings.TrimPrefix(strings.ReplaceAll(v, "\\", "/"), "./")
		if clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
			return fmt.Errorf("include %q: the destination %q leaves the website", source, v)
		}
		return nil
	}
	return fmt.Errorf("include %q must be true, false or a destination path", source)
}

// includeRules accepts the ordered selection of a website's files: an object of
// source to true, false or a destination, or an array of sources and one-key
// objects of that kind (which is what the loader makes of an object, to keep
// its order).
func includeRules(value interface{}) error {
	switch v := value.(type) {
	case map[string]interface{}:
		for source, rule := range v {
			if err := includeRule(source, rule); err != nil {
				return err
			}
		}
		return nil
	case []interface{}:
		for i, item := range v {
			switch entry := item.(type) {
			case string:
				if err := includeRule(entry, true); err != nil {
					return err
				}
			case map[string]interface{}:
				if len(entry) != 1 {
					return fmt.Errorf("item %d must have exactly one source", i)
				}
				for source, rule := range entry {
					if err := includeRule(source, rule); err != nil {
						return err
					}
				}
			default:
				return fmt.Errorf("item %d must be a source or an object with one source", i)
			}
		}
		return nil
	}
	return fmt.Errorf("must be an object or an array")
}

// answerStatus accepts the HTTP statuses a website can answer with: 200-500
// except 204, 205 and 304, which have no body.
func answerStatus(value interface{}) error {
	number, ok := value.(float64)
	if !ok || number != math.Trunc(number) {
		return fmt.Errorf("must be an integer")
	}
	switch int(number) {
	case 204, 205, 304:
		return fmt.Errorf("must not be 204, 205 or 304")
	}
	if number < 200 || number > 500 {
		return fmt.Errorf("must be between 200 and 500")
	}
	return nil
}

// redirectStatus accepts the statuses of a redirect.
func redirectStatus(value interface{}) error {
	number, ok := value.(float64)
	if !ok {
		return fmt.Errorf("must be an integer")
	}
	switch int(number) {
	case 301, 302, 303, 307, 308:
		if number == math.Trunc(number) {
			return nil
		}
	}
	return fmt.Errorf("must be 301, 302, 303, 307 or 308")
}

// redirectFrom accepts a redirect source: a path, or a URL with a host.
func redirectFrom(value interface{}) error {
	from, _ := value.(string)
	if !strings.HasPrefix(from, "/") && !strings.Contains(from, "://") {
		return fmt.Errorf("must start with a slash, or be a URL")
	}
	if strings.ContainsAny(from, "? \t\r\n") || strings.Contains(from, "#") {
		return fmt.Errorf("must not hold spaces, a query string or a fragment")
	}
	return nil
}

// redirectTo accepts a redirect destination: no spaces or control characters.
func redirectTo(value interface{}) error {
	to, _ := value.(string)
	if strings.TrimSpace(to) == "" {
		return fmt.Errorf("must not be empty")
	}
	for _, r := range to {
		if r <= 0x20 || r == 0x7f {
			return fmt.Errorf("must not hold spaces, line breaks or control characters")
		}
	}
	return nil
}

func initStaticoRedirectValidator() *Validator {
	v := NewValidator().Description("A redirect of the website.")
	v.Field("from").Description("Path to match, with :name placeholders and a * splat as in a _redirects file (default: every path). A URL with a host also matches the host.").Optional().String().Custom(redirectFrom)
	v.Field("to").Description("Where to redirect to; :name and :splat from the source can be used.").Required().String().Custom(redirectTo)
	v.Field("statusCode").Description("Redirect status (default 302).").Optional().Custom(redirectStatus)
	return v
}

func initStaticoValidator() *Validator {
	v := initResourceValidator()
	v.Field("workspaceId").Optional().String().MinLength(1).MaxLength(255)
	v.Field("projectId").Optional().String().MinLength(1).MaxLength(255)
	v.Field("regionId").Optional().String().MinLength(1).MaxLength(255)
	v.Field("public").Description("Anybody can read the website (default true); false makes it answer only to a request with an API token.").Optional().Bool()
	v.Field("include").Description("The files of the website, as an object whose keys run in order. A key is a file, a folder or a glob (relative to this config file, or absolute; {a,b} and ** work). Its value is true (include: a folder's files land at the root of the website, a file or glob keeps its path relative to this file), a destination in the website (\"./\" is the root; a folder's files land below it, a file with a name as destination is renamed), or false (exclude what it matches from the files selected so far). Default: the folder of this file at the root. Hidden files and folders (such as .env and .git) and .map files found in a folder or glob are never uploaded; naming one as a source includes it. A _redirects file at the root of the website configures the redirects of the version and is not uploaded.").Optional().Custom(includeRules)
	v.Field("htmlToFolders").Description("Also copy every page.html as page/index.html (and index.htm as index.html) unless that path exists, so a proxy serves /page (default true).").Optional().Bool()
	v.Field("defaultFile").Description("File served for a folder (default /index.html).").Optional().String().MinLength(1).MaxLength(255)
	v.Field("defaultStatus").Description("Status of a default file answer (default 200).").Optional().Custom(answerStatus)
	v.Field("fallbackFile").Description("File served for a path that is not found (default /404.html); with fallbackStatus 200 and /index.html it serves a single page app.").Optional().String().MinLength(1).MaxLength(255)
	v.Field("fallbackStatus").Description("Status of a fallback answer (default 404).").Optional().Custom(answerStatus)
	v.Field("redirects").Description("Redirects, in order, before those of a _redirects file.").Optional().Array(initStaticoRedirectValidator())
	return v
}
