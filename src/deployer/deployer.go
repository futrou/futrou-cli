// Package deployer plans and applies declarative Futrou project resources.
package deployer

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"futrou-cli/src/config"
	"futrou-cli/src/services"
)

type ActionType string

const (
	Create ActionType = "create"
	Update ActionType = "update"
	Delete ActionType = "delete"
)

// Action is one API operation in a deploy plan.
type Action struct {
	Type       ActionType             `json:"type"`
	Resource   string                 `json:"resource"`
	Name       string                 `json:"name"`
	RemoteName string                 `json:"-"`
	ID         string                 `json:"id,omitempty"`
	Payload    map[string]interface{} `json:"payload,omitempty"`
	Changes    map[string]interface{} `json:"changes,omitempty"`
	Previous   map[string]interface{} `json:"previous,omitempty"`
	Path       string                 `json:"-"`
}

type Plan struct {
	Actions []Action `json:"actions"`
}

type resourceSpec struct {
	resource string
	path     string
	nameKey  string
	items    func(*config.Config) interface{}
}

var resources = []resourceSpec{
	{"serverlet", "/v2/serverlets", "name", func(c *config.Config) interface{} { return c.Serverlets }},
	{"dns zone", "/v2/dns", "domain", func(c *config.Config) interface{} { return c.DNS }},
	{"proxy", "/v2/proxies", "domain", func(c *config.Config) interface{} { return c.Proxies }},
	{"volume", "/v2/volumes", "name", func(c *config.Config) interface{} { return c.Volumes }},
	{"cron", "/v2/crons", "name", func(c *config.Config) interface{} { return c.Crons }},
}

// BuildPlan compares every resource declared in cfg with the API. destroy
// plans deletion of only the resources declared in cfg; it never prunes
// unrelated remote infrastructure.
func BuildPlan(client *services.ApiClient, cfg *config.Config, destroy bool) (*Plan, error) {
	var remoteConfig *config.Config
	if cfg.Project != "" {
		remoteConfig = &config.Config{Workspace: cfg.Workspace, Project: cfg.Project}
		if err := remoteConfig.Pull(client, ""); err != nil {
			return nil, fmt.Errorf("pulling cloud configuration: %w", err)
		}
	}
	plan := &Plan{}
	for _, spec := range resources {
		desired, err := itemsAsMaps(spec.items(cfg))
		if err != nil {
			return nil, fmt.Errorf("reading %s config: %w", spec.resource, err)
		}
		if remoteConfig == nil && len(desired) == 0 {
			continue
		}
		var remote []map[string]interface{}
		if remoteConfig != nil {
			remote, err = itemsAsMaps(spec.items(remoteConfig))
			if err != nil {
				return nil, fmt.Errorf("reading cloud %s config: %w", spec.resource, err)
			}
			// Some resource endpoints do not consistently honor projectId. Fall
			// back to their live collection so existing named resources are not
			// mistaken for creates.
			if len(remote) == 0 {
				var unscoped []map[string]interface{}
				if _, err := client.RequestInto("GET", spec.path, nil, &unscoped); err != nil {
					return nil, fmt.Errorf("listing %ss: %w", spec.resource, err)
				}
				remote = matchingResources(unscoped, desired, spec.nameKey)
			}
		} else {
			if _, err := client.RequestInto("GET", spec.path, nil, &remote); err != nil {
				return nil, fmt.Errorf("listing %ss: %w", spec.resource, err)
			}
		}
		usedRemote := map[string]bool{}
		for _, item := range desired {
			name := itemName(item, spec.nameKey)
			match := findRemote(remote, item, spec.nameKey)
			if match == nil && remoteConfig != nil {
				match = findLockedRemote(remote, usedRemote, cfg.Locks, remoteConfig.Locks, lockGroup(spec.resource))
			}
			if match != nil {
				usedRemote[stringValue(match["id"])+itemName(match, spec.nameKey)] = true
			}
			if destroy {
				if match != nil {
					plan.Actions = append(plan.Actions, Action{Type: Delete, Resource: spec.resource, Name: name, ID: stringValue(match["id"]), Path: spec.path})
				}
				continue
			}
			payload := writablePayload(item)
			if match == nil {
				plan.Actions = append(plan.Actions, Action{Type: Create, Resource: spec.resource, Name: name, Payload: payload, Path: spec.path})
				continue
			}
			changes := changedFields(payload, match)
			if len(changes) > 0 {
				plan.Actions = append(plan.Actions, Action{Type: Update, Resource: spec.resource, Name: name, RemoteName: itemName(match, spec.nameKey), ID: stringValue(match["id"]), Changes: changes, Previous: previousFields(changes, match), Path: spec.path})
			}
			if spec.resource == "dns zone" {
				zoneID, err := resolveDNSZoneID(client, item)
				if err != nil {
					return nil, err
				}
				if err := planDNSRecords(client, plan, item, zoneID, cfg.Locks); err != nil {
					return nil, err
				}
			}
		}
		if !destroy {
			for _, item := range remote {
				if usedRemote[stringValue(item["id"])+itemName(item, spec.nameKey)] {
					continue
				}
				managed := false
				for _, wanted := range desired {
					if findRemote([]map[string]interface{}{item}, wanted, spec.nameKey) != nil || itemName(item, spec.nameKey) == itemName(wanted, spec.nameKey) {
						managed = true
						break
					}
				}
				if !managed {
					plan.Actions = append(plan.Actions, Action{Type: Delete, Resource: spec.resource, Name: itemName(item, spec.nameKey), ID: stringValue(item["id"]), Path: spec.path})
				}
			}
		}
	}
	return plan, nil
}

func matchingResources(remote, desired []map[string]interface{}, nameKey string) []map[string]interface{} {
	result := []map[string]interface{}{}
	for _, item := range remote {
		for _, wanted := range desired {
			if findRemote([]map[string]interface{}{item}, wanted, nameKey) != nil {
				result = append(result, item)
				break
			}
		}
	}
	return result
}

func lockGroup(resource string) string {
	switch resource {
	case "serverlet":
		return "serverlets"
	case "dns zone":
		return "dns"
	case "proxy":
		return "proxies"
	case "volume":
		return "volumes"
	case "cron":
		return "crons"
	}
	return ""
}

func findLockedRemote(remote []map[string]interface{}, used map[string]bool, localLocks, cloudLocks map[string]string, group string) map[string]interface{} {
	if group == "" {
		return nil
	}
	for cloudKey, fingerprint := range cloudLocks {
		if !strings.HasPrefix(cloudKey, group+".") {
			continue
		}
		found := false
		for localKey, localFingerprint := range localLocks {
			if strings.HasPrefix(localKey, group+".") && localFingerprint == fingerprint {
				found = true
				break
			}
		}
		if !found {
			continue
		}
		name := strings.TrimPrefix(cloudKey, group+".")
		for _, item := range remote {
			if !used[stringValue(item["id"])+itemName(item, "name")] && (itemName(item, "name") == name || itemName(item, "domain") == name) {
				return item
			}
		}
	}
	return nil
}

// planDNSRecords reconciles records separately from their zone. Records are
// intentionally not sent to the zone endpoint, because the API exposes them
// through /v2/dns/<zone>/records.
func resolveDNSZoneID(client *services.ApiClient, desired map[string]interface{}) (string, error) {
	var zones []map[string]interface{}
	if _, err := client.RequestInto("GET", "/v2/dns", nil, &zones); err != nil {
		return "", fmt.Errorf("listing DNS zones: %w", err)
	}
	zone := findRemote(zones, desired, "domain")
	if zone == nil || stringValue(zone["id"]) == "" {
		return "", fmt.Errorf("DNS zone %q not found", itemName(desired, "domain"))
	}
	return stringValue(zone["id"]), nil
}

func planDNSRecords(client *services.ApiClient, plan *Plan, desiredZone map[string]interface{}, zoneID string, locks map[string]string) error {
	desired, err := arrayMaps(desiredZone["records"])
	if err != nil {
		return fmt.Errorf("reading DNS records: %w", err)
	}
	var remote []map[string]interface{}
	if _, err := client.RequestInto("GET", "/v2/dns/"+zoneID+"/records", nil, &remote); err != nil {
		return fmt.Errorf("listing DNS records: %w", err)
	}
	matchedRemote := map[string]bool{}
	seen := map[string]int{}
	for index, record := range desired {
		name := itemName(desiredZone, "domain") + " / " + itemName(record, "name") + " " + stringValue(record["type"])
		lockKey := dnsRecordLockKey(itemName(desiredZone, "domain"), record, seen)
		available := make([]map[string]interface{}, 0, len(remote))
		for _, candidate := range remote {
			if !matchedRemote[stringValue(candidate["id"])] {
				available = append(available, candidate)
			}
		}
		var match map[string]interface{}
		if fingerprint := locks[lockKey]; fingerprint != "" {
			for _, candidate := range available {
				if config.LockHash(stringValue(candidate["id"])) == fingerprint {
					match = candidate
					break
				}
			}
		}
		if match == nil && len(locks) == 0 {
			match = findDNSRecord(available, desired, index, record)
		}
		if match == nil && len(locks) == 0 && index < len(remote) {
			match = remote[index]
		}
		if match != nil {
			matchedRemote[stringValue(match["id"])] = true
		}
		path := "/v2/dns/" + zoneID + "/records"
		payload := writablePayload(record)
		if match == nil {
			plan.Actions = append(plan.Actions, Action{Type: Create, Resource: "dns record", Name: name, Payload: payload, Path: path})
			continue
		}
		if changes := changedFields(payload, match); len(changes) > 0 {
			if stringValue(record["type"]) == "SOA" {
				delete(changes, "value")
			}
			if len(changes) == 0 {
				continue
			}
			plan.Actions = append(plan.Actions, Action{Type: Update, Resource: "dns record", Name: name, ID: stringValue(match["id"]), Changes: changes, Previous: previousFields(changes, match), Path: path})
		}
	}
	for _, record := range remote {
		if !matchedRemote[stringValue(record["id"])] && stringValue(record["id"]) != "" {
			name := itemName(desiredZone, "domain") + " / " + itemName(record, "name") + " " + stringValue(record["type"])
			plan.Actions = append(plan.Actions, Action{Type: Delete, Resource: "dns record", Name: name, ID: stringValue(record["id"]), Path: "/v2/dns/" + zoneID + "/records"})
		}
	}
	return nil
}

func dnsRecordLockKey(zone string, record map[string]interface{}, seen map[string]int) string {
	escape := func(value string) string { return strings.NewReplacer("\\", "\\\\", ".", "\\.").Replace(value) }
	key := "dns." + escape(zone) + ".records." + escape(stringValue(record["name"])) + "." + escape(stringValue(record["type"]))
	seen[key]++
	if seen[key] > 1 {
		key += fmt.Sprintf("[%d]", seen[key]-1)
	}
	return key
}

func previousFields(changes, remote map[string]interface{}) map[string]interface{} {
	previous := map[string]interface{}{}
	for key := range changes {
		previous[key] = remote[key]
	}
	return previous
}

func arrayMaps(value interface{}) ([]map[string]interface{}, error) {
	if value == nil {
		return nil, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var result []map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func findDNSRecord(remote, desiredRecords []map[string]interface{}, index int, desired map[string]interface{}) map[string]interface{} {
	if id := stringValue(desired["id"]); id != "" {
		for _, item := range remote {
			if id == stringValue(item["id"]) {
				return item
			}
		}
	}
	// DNS allows duplicate name/type records (for example multiple A values).
	// Their declaration order is therefore the stable fallback identity.
	duplicates := 0
	for _, record := range desiredRecords {
		if itemName(record, "name") == itemName(desired, "name") && stringValue(record["type"]) == stringValue(desired["type"]) {
			duplicates++
		}
	}
	if duplicates > 1 && index < len(remote) {
		return remote[index]
	}
	for _, item := range remote {
		if itemName(item, "name") == itemName(desired, "name") && stringValue(item["type"]) == stringValue(desired["type"]) {
			return item
		}
	}
	return nil
}

// Apply executes every action in order.
func Apply(client *services.ApiClient, plan *Plan) error {
	for _, action := range plan.Actions {
		var body interface{}
		path := action.Path
		method := "POST"
		switch action.Type {
		case Create:
			body = action.Payload
		case Update:
			if action.ID == "" {
				var err error
				action.ID, err = resolveActionID(client, action)
				if err != nil {
					return err
				}
			}
			method, body, path = "PATCH", action.Changes, action.Path+"/"+action.ID
		case Delete:
			if action.ID == "" {
				var err error
				action.ID, err = resolveActionID(client, action)
				if err != nil {
					return err
				}
			}
			method, body, path = "DELETE", map[string]interface{}{}, action.Path+"/"+action.ID
		}
		var result interface{}
		status, err := client.RequestInto(method, path, body, &result)
		if err != nil {
			return fmt.Errorf("%s %s %q: %w", action.Type, action.Resource, action.Name, err)
		}
		if status >= 400 {
			return fmt.Errorf("%s %s %q failed with status %d", action.Type, action.Resource, action.Name, status)
		}
	}
	return nil
}

func resolveActionID(client *services.ApiClient, action Action) (string, error) {
	var resources []map[string]interface{}
	if _, err := client.RequestInto("GET", action.Path, nil, &resources); err != nil {
		return "", fmt.Errorf("resolving %s %q: %w", action.Resource, action.Name, err)
	}
	name := action.Name
	if action.RemoteName != "" {
		name = action.RemoteName
	}
	if action.Resource == "dns record" {
		parts := strings.SplitN(name, " / ", 2)
		if len(parts) == 2 {
			name = parts[1]
		}
	}
	for _, resource := range resources {
		if action.Resource == "dns record" {
			fields := strings.Fields(name)
			if len(fields) >= 2 && itemName(resource, "name") == fields[0] && stringValue(resource["type"]) == fields[len(fields)-1] {
				return stringValue(resource["id"]), nil
			}
		} else if itemName(resource, "domain") == name || itemName(resource, "name") == name {
			return stringValue(resource["id"]), nil
		}
	}
	return "", fmt.Errorf("%s %q not found", action.Resource, action.Name)
}

func itemsAsMaps(items interface{}) ([]map[string]interface{}, error) {
	data, err := json.Marshal(items)
	if err != nil {
		return nil, err
	}
	var result []map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func findRemote(remote []map[string]interface{}, desired map[string]interface{}, nameKey string) map[string]interface{} {
	id := stringValue(desired["id"])
	name := itemName(desired, nameKey)
	for _, item := range remote {
		if id != "" && id == stringValue(item["id"]) {
			return item
		}
		if id == "" && name != "" && name == itemName(item, nameKey) {
			return item
		}
		// DNS API responses have historically used both `name` and `domain`
		// for the zone identity. Treat either field as equivalent.
		if nameKey == "domain" && name != "" && (name == stringValue(item["name"]) || name == stringValue(item["domain"])) {
			return item
		}
	}
	return nil
}

func itemName(item map[string]interface{}, nameKey string) string {
	if value := stringValue(item[nameKey]); value != "" {
		return value
	}
	return stringValue(item["name"])
}

func stringValue(value interface{}) string {
	stringValue, _ := value.(string)
	return stringValue
}

func writablePayload(item map[string]interface{}) map[string]interface{} {
	payload := make(map[string]interface{}, len(item))
	for key, value := range item {
		switch key {
		case "id", "createdAt", "updatedAt", "instances", "state", "records":
			continue
		}
		payload[key] = value
	}
	return payload
}

func changedFields(desired, remote map[string]interface{}) map[string]interface{} {
	changes := map[string]interface{}{}
	for key, value := range desired {
		if !reflect.DeepEqual(normalize(value), normalize(remote[key])) {
			changes[key] = value
		}
	}
	return changes
}

func normalize(value interface{}) interface{} {
	if number, ok := value.(float64); ok && number == float64(int(number)) {
		return int(number)
	}
	return value
}

// SortedActions returns a stable display order without changing apply order.
func SortedActions(plan *Plan) []Action {
	actions := append([]Action(nil), plan.Actions...)
	sort.SliceStable(actions, func(i, j int) bool { return actions[i].Resource+actions[i].Name < actions[j].Resource+actions[j].Name })
	return actions
}
