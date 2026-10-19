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
	v.Field("region").Description("Region name or ID applied to every resource in the project that does not set its own regionId.").Optional().String().MinLength(1).MaxLength(255)
	v.Field("networks").Description("Private networks managed by this project; serverlets join them by name.").Optional().Array(initNetworkValidator()).Custom(uniqueNames("name"))
	v.Field("serverlets").Description("Serverlets to create or update in this project.").Optional().Array(initServerletValidator()).Custom(uniqueNames("name")).Custom(validateNeeds)
	v.Field("dns").Description("DNS zones and records managed by this project.").Optional().Array(initDNSValidator()).Custom(uniqueNames("domain", "name"))
	v.Field("proxies").Description("HTTP, TCP, or UDP proxies managed by this project.").Optional().Array(initProxyValidator()).Custom(uniqueNames("domain"))
	v.Field("storages").Description("Persistent storage volumes available to the project.").Optional().Array(initStorageValidator()).Custom(uniqueNames("name"))
	v.Field("crons").Description("Scheduled HTTP or code jobs for the project.").Optional().Array(initCronValidator()).Custom(uniqueNames("name"))
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
