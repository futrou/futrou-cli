package config

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"futrou-cli/src/utils"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"futrou-cli/src/api"
	"futrou-cli/src/constants"
	"futrou-cli/src/logger"
	"futrou-cli/src/statico"
	"futrou-cli/src/validators"

	"gopkg.in/yaml.v3"
)

// Config is the declarative project configuration stored in futrou.json
// (or futrou.yaml/yml, or exported by futrou.js/cjs/mjs/ts). It is built from the generated API
// resource types, with deployment-only fields added where the API response
// models do not expose them.
type Config struct {
	// pulledWorkspaceID and pulledProjectID are the IDs a Pull resolved; they are
	// not part of the file (see Scope).
	pulledWorkspaceID, pulledProjectID string

	Schema    string `json:"$schema,omitempty"`
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project,omitempty"`
	// InstallCommand runs once on a device, before a deploy, until it has run
	// there (again when the command changes). BuildCommand runs before every
	// deploy. Both run in the folder of the config file.
	InstallCommand string `json:"installCommand,omitempty"`
	BuildCommand   string `json:"buildCommand,omitempty"`
	// Region (a region name or ID) is the default region of every resource in
	// the project; a resource's own regionId takes precedence.
	Region     string            `json:"region,omitempty"`
	Networks   []NetworkConfig   `json:"networks,omitempty"`
	Serverlets []ServerletConfig `json:"serverlets,omitempty"`
	DNS        []DNSConfig       `json:"dns,omitempty"`
	Proxies    []ProxyConfig     `json:"proxies,omitempty"`
	Storages   []StorageConfig   `json:"storages,omitempty"`
	Crons      []CronConfig      `json:"crons,omitempty"`
	// Staticos are static websites; `futrou deploy` uploads their files.
	Staticos []StaticoConfig `json:"statico,omitempty"`
	// Template holds presentation metadata for the Futrou template store
	// (title, description, category, icon, ...). The CLI does not use it; it
	// is declared so a template's futrou.json validates without warnings.
	Template map[string]interface{} `json:"template,omitempty"`
	Locks    map[string]string      `json:"locks,omitempty"`
	// File is the path the config was loaded from; relative paths in it (a
	// statico website's include) are resolved against its folder. Not part of the file.
	File string `json:"-"`
}

// StaticoConfig is a static website (statico) and the files that are uploaded
// to it. Name, RegionId and Public are the website itself; the rest describes
// its next StaticoVersion and is never sent to the API as part of the website.
type StaticoConfig struct {
	Name        string `json:"name,omitempty"`
	WorkspaceId string `json:"workspaceId,omitempty"`
	ProjectId   string `json:"projectId,omitempty"`
	RegionId    string `json:"regionId,omitempty"`
	Public      *bool  `json:"public,omitempty"`
	// Include selects the files, as an object whose keys run in order: a file,
	// folder or glob (relative to the config file, or absolute) with true (keep
	// its path), a destination in the website, or false (exclude what it matches).
	// Without it the whole folder of the config is uploaded to the root.
	Include utils.GlobRules `json:"include,omitempty"`
	// HtmlToFolders (default true) also copies every page.html as page/index.html.
	HtmlToFolders *bool `json:"htmlToFolders,omitempty"`
	// How the website answers: the file of a folder, and the file for a path that is not found.
	DefaultFile    string `json:"defaultFile,omitempty"`
	DefaultStatus  int    `json:"defaultStatus,omitempty"`
	FallbackFile   string `json:"fallbackFile,omitempty"`
	FallbackStatus int    `json:"fallbackStatus,omitempty"`
	// Redirects come before those of a _redirects file in the uploaded folder.
	Redirects []statico.Redirect `json:"redirects,omitempty"`
}

// StaticoSelection is the files a website uploads. The config file itself is never uploaded.
func (cfg *Config) StaticoSelection(site StaticoConfig) statico.Selection {
	sel := statico.Selection{Rules: site.Include, NoHTMLToFolders: site.HtmlToFolders != nil && !*site.HtmlToFolders}
	if cfg.File != "" {
		sel.Ignore = []string{cfg.File}
	}
	return sel
}

// BaseDir is the folder include paths are relative to: the one holding the config file.
func (cfg *Config) BaseDir() string {
	if cfg.File == "" {
		return "."
	}
	return filepath.Dir(cfg.File)
}

// VersionOptions is how the uploaded StaticoVersion answers.
func (s StaticoConfig) VersionOptions() (statico.VersionOptions, error) {
	rules, err := statico.RedirectRules(s.Redirects)
	if err != nil {
		return statico.VersionOptions{}, fmt.Errorf("statico %q: %w", s.Name, err)
	}
	return statico.VersionOptions{
		DefaultFile: s.DefaultFile, DefaultStatus: s.DefaultStatus,
		FallbackFile: s.FallbackFile, FallbackStatus: s.FallbackStatus,
		Redirects: rules,
	}, nil
}

// ServerletConfig exposes the Serverlet API model plus creation/deployment
// fields that are not part of a Serverlet response. Sizing is entirely
// determined by ServerletPlanId; the API's Serverlet model already carries
// Mounts (mountPath -> storageId) and Scaling as response/config fields.
type ServerletConfig struct {
	api.Serverlet
	ServerletPlanId string            `json:"serverletPlanId,omitempty"`
	WorkspaceId     string            `json:"workspaceId,omitempty"`
	ProjectId       string            `json:"projectId,omitempty"`
	Env             map[string]string `json:"env,omitempty"`
	Ports           []ServerletPort   `json:"ports,omitempty"`
	// Entrypoint and Cwd override the image's start command and working
	// directory, so an official image can run without building a new one.
	Entrypoint Entrypoint `json:"entrypoint,omitempty"`
	Cwd        string     `json:"cwd,omitempty"`
}

// MarshalJSON keeps declarative serverlet files stable and human-readable:
// identity and image are emitted before sizing and generated API fields.
func (s ServerletConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Name                string                 `json:"name,omitempty"`
		Image               string                 `json:"image,omitempty"`
		ServerletPlanId     string                 `json:"serverletPlanId,omitempty"`
		WorkspaceId         string                 `json:"workspaceId,omitempty"`
		ProjectId           string                 `json:"projectId,omitempty"`
		RegionId            string                 `json:"regionId,omitempty"`
		Env                 map[string]string      `json:"env,omitempty"`
		Mounts              map[string]string      `json:"mounts,omitempty"`
		Scaling             map[string]float64     `json:"scaling,omitempty"`
		MinInstances        float64                `json:"minInstances,omitempty"`
		MaxInstances        float64                `json:"maxInstances,omitempty"`
		Ports               []ServerletPort        `json:"ports,omitempty"`
		Entrypoint          Entrypoint             `json:"entrypoint,omitempty"`
		Cwd                 string                 `json:"cwd,omitempty"`
		User                string                 `json:"user,omitempty"`
		Networks            []string               `json:"networks,omitempty"`
		Needs               []string               `json:"needs,omitempty"`
		Healthcheck         string                 `json:"healthcheck,omitempty"`
		HealthcheckInterval float64                `json:"healthcheckInterval,omitempty"`
		HealthcheckTimeout  float64                `json:"healthcheckTimeout,omitempty"`
		HealthcheckRetries  float64                `json:"healthcheckRetries,omitempty"`
		StartTimeout        float64                `json:"startTimeout,omitempty"`
		StopTimeout         float64                `json:"stopTimeout,omitempty"`
		Strategy            map[string]interface{} `json:"strategy,omitempty"`
	}{s.Name, s.Image, s.ServerletPlanId, s.WorkspaceId, s.ProjectId, s.RegionId, s.Env, s.Mounts, s.Scaling, s.MinInstances, s.MaxInstances, s.Ports, s.Entrypoint, s.Cwd, s.User, s.Networks, s.Needs, s.Healthcheck, s.HealthcheckInterval, s.HealthcheckTimeout, s.HealthcheckRetries, s.StartTimeout, s.StopTimeout, s.Strategy})
}

// Entrypoint is the command or path that replaces the image's default start
// command. It is a single string; a JSON/YAML array is accepted too and its
// items are joined with spaces, so ["node", "server.js"] means "node server.js".
type Entrypoint string

// UnmarshalJSON accepts a string or an array of strings.
func (e *Entrypoint) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		*e = Entrypoint(text)
		return nil
	}
	var parts []string
	if err := json.Unmarshal(data, &parts); err != nil {
		return fmt.Errorf("entrypoint must be a string or an array of strings")
	}
	*e = Entrypoint(strings.Join(parts, " "))
	return nil
}

type ServerletPort struct {
	Port     int    `json:"port"`
	Protocol string `json:"protocol,omitempty"`
}

// DNSConfig represents a DNS zone and its records. DNS resources are not in
// the generated client model, so they are defined here using the API payload.
type DNSConfig struct {
	Id          string            `json:"id,omitempty"`
	WorkspaceId string            `json:"workspaceId,omitempty"`
	ProjectId   string            `json:"projectId,omitempty"`
	Name        string            `json:"name,omitempty"`
	Domain      string            `json:"domain,omitempty"`
	TTL         int               `json:"ttl,omitempty"`
	Priority    int               `json:"priority,omitempty"`
	Records     []DNSRecordConfig `json:"records,omitempty"`
}

type DNSRecordConfig struct {
	Id       string `json:"id,omitempty"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Value    string `json:"value"`
	TTL      int    `json:"ttl"`
	Priority int    `json:"priority,omitempty"`
}

func (d DNSConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Domain   string            `json:"domain,omitempty"`
		Name     string            `json:"name,omitempty"`
		TTL      int               `json:"ttl"`
		Priority int               `json:"priority"`
		Records  []DNSRecordConfig `json:"records,omitempty"`
	}{d.Domain, d.Name, d.TTL, d.Priority, d.Records})
}

func (d DNSRecordConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Name     string `json:"name"`
		Type     string `json:"type"`
		Value    string `json:"value"`
		TTL      int    `json:"ttl"`
		Priority int    `json:"priority"`
	}{d.Name, d.Type, d.Value, d.TTL, d.Priority})
}

// ProxyConfig and CronConfig are the generated API models. They need no
// project-config-specific adjustment, so aliases retain every API field
// without duplicating the generated definitions.
type ProxyConfig api.Proxy

const (
	defaultProxyPort     = 80
	defaultProxyType     = api.ProxyTypeHttp
	defaultProxyStrategy = api.ProxyStrategyRoundRobin
)

// UnmarshalJSON starts from the API defaults, so a proxy that leaves out a
// setting that defaults to true (preserveHeaders, preserveHost, preservePath,
// preserveQuery, verifyTls) keeps it instead of silently turning it off.
func (p *ProxyConfig) UnmarshalJSON(data []byte) error {
	type plain api.Proxy
	proxy := plain{PreserveHeaders: true, PreserveHost: true, PreservePath: true, PreserveQuery: true, VerifyTls: true}
	if err := json.Unmarshal(data, &proxy); err != nil {
		return err
	}
	*p = ProxyConfig(proxy)
	return nil
}

// MarshalJSON omits API defaults while retaining explicit overrides.
func (p ProxyConfig) MarshalJSON() ([]byte, error) {
	differentBool := func(value, defaultValue bool) *bool {
		if value == defaultValue {
			return nil
		}
		return &value
	}
	port := p.Port
	if port == defaultProxyPort {
		port = 0
	}
	var typeName api.ProxyType
	if p.Type != nil && *p.Type != defaultProxyType {
		typeName = *p.Type
	}
	var strategy api.ProxyStrategy
	if p.Strategy != nil && *p.Strategy != defaultProxyStrategy {
		strategy = *p.Strategy
	}
	return json.Marshal(struct {
		Domain          string            `json:"domain,omitempty"`
		Type            api.ProxyType     `json:"type,omitempty"`
		Target          string            `json:"target,omitempty"`
		Port            float64           `json:"port,omitempty"`
		EnforceHttps    *bool             `json:"enforceHttps,omitempty"`
		FollowRedirects *bool             `json:"followRedirects,omitempty"`
		PreserveHeaders *bool             `json:"preserveHeaders,omitempty"`
		PreserveHost    *bool             `json:"preserveHost,omitempty"`
		PreservePath    *bool             `json:"preservePath,omitempty"`
		PreserveQuery   *bool             `json:"preserveQuery,omitempty"`
		VerifyTls       *bool             `json:"verifyTls,omitempty"`
		Strategy        api.ProxyStrategy `json:"strategy,omitempty"`
		Variant         *api.ProxyVariant `json:"variant,omitempty"`
		ProxyPlanId     string            `json:"proxyPlanId,omitempty"`
		RegionId        string            `json:"regionId,omitempty"`
		Rules           []api.Rule        `json:"rules,omitempty"`
	}{p.Domain, typeName, p.Target, port, differentBool(p.EnforceHttps, false), differentBool(p.FollowRedirects, false), differentBool(p.PreserveHeaders, true), differentBool(p.PreserveHost, true), differentBool(p.PreservePath, true), differentBool(p.PreserveQuery, true), differentBool(p.VerifyTls, true), strategy, p.Variant, p.ProxyPlanId, p.RegionId, p.Rules})
}

// NetworkConfig is a private network that serverlets can join through their
// "networks" field. The generated API types do not include the Network
// model, so it is defined here using the API payload.
type NetworkConfig struct {
	Id          string `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	WorkspaceId string `json:"workspaceId,omitempty"`
	ProjectId   string `json:"projectId,omitempty"`
	RegionId    string `json:"regionId,omitempty"`
	Ipv4        string `json:"ipv4,omitempty"`
	Ipv6        string `json:"ipv6,omitempty"`
	Internal    *bool  `json:"internal,omitempty"`
	CreatedAt   string `json:"createdAt,omitempty"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
}

// StorageConfig and CronConfig are the generated API models. Their timestamps
// are not pointers, which encoding/json cannot omit; MarshalJSON drops the zero ones so a pulled config does not
// fill up with "0001-01-01T00:00:00Z" and a deploy never sends them.
type StorageConfig api.Storage
type CronConfig api.Cron

func (s StorageConfig) MarshalJSON() ([]byte, error) { return marshalWithoutZeroTimes(api.Storage(s)) }
func (c CronConfig) MarshalJSON() ([]byte, error)    { return marshalWithoutZeroTimes(api.Cron(c)) }

var timestampFields = []string{"createdAt", "updatedAt", "startedAt"}

func marshalWithoutZeroTimes(value interface{}) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return nil, err
	}
	for _, field := range timestampFields {
		var stamp time.Time
		if raw, ok := object[field]; ok && json.Unmarshal(raw, &stamp) == nil && stamp.IsZero() {
			delete(object, field)
		}
	}
	return json.Marshal(object)
}

// ToJSON serializes this project config as indented futrou.json content.
func (cfg *Config) ToJSON() ([]byte, error) {
	return json.MarshalIndent(cfg, "", "  ")
}

// ToJS serializes this project config as an ESM default export suitable for a
// futrou.mjs or futrou.ts config file.
func (cfg *Config) ToJS() (string, error) {
	data, err := cfg.ToJSON()
	if err != nil {
		return "", err
	}
	return "export default " + string(data) + ";\n", nil
}

// ToJSONSchema returns the generated project JSON Schema with its public ID.
func (cfg *Config) ToJSONSchema() map[string]interface{} {
	schema := validators.ConfigValidator.ToJSONSchema()
	schema["$id"] = constants.ProjectConfigSchemaURL
	if properties, ok := schema["properties"].(map[string]interface{}); ok {
		// Each collection is a list, or an object keyed by name.
		for collection := range keyedCollections {
			list, ok := properties[collection].(map[string]interface{})
			if !ok {
				continue
			}
			item, _ := list["items"].(map[string]interface{})
			keyed := map[string]interface{}{
				"type":                 "object",
				"description":          "The same items as an object whose keys are the " + keyedCollections[collection] + "s.",
				"additionalProperties": item,
			}
			properties[collection] = map[string]interface{}{
				"description": list["description"],
				"oneOf":       []interface{}{list, keyed},
			}
			delete(list, "description")
		}
		properties["locks"] = map[string]interface{}{
			"description":          "CLI-maintained opaque resource lock fingerprints.",
			"type":                 "object",
			"additionalProperties": map[string]interface{}{"type": "string"},
		}
	}
	return schema
}

// APIRequester is the API capability required to pull a cloud project.
type APIRequester interface {
	RequestInto(method, path string, body interface{}, value interface{}) (int, error)
}

// Scope is the workspace and project IDs the last Pull resolved, "" before one.
// New resources are created in them: the API refuses a create without one.
func (cfg *Config) Scope() (workspaceID, projectID string) {
	return cfg.pulledWorkspaceID, cfg.pulledProjectID
}

// Pull fills this config from the current cloud representation of selector.
// An empty selector uses cfg.Project.
func (cfg *Config) Pull(client APIRequester, selector string) error {
	if cfg == nil {
		return fmt.Errorf("config is required")
	}
	if selector == "" {
		selector = cfg.Project
	}
	if selector == "" {
		return fmt.Errorf("project name or ID is required")
	}
	workspaceID, err := resolveCloudWorkspaceID(client, cfg.Workspace)
	if err != nil {
		return err
	}
	project, err := findCloudProject(client, selector, workspaceID)
	if err != nil {
		return err
	}
	workspace, err := pullWorkspaceName(client, project)
	if err != nil {
		return err
	}
	q := "?projectId=" + url.QueryEscape(project.Id)
	networks, err := pullCollection(client, "/v2/networks"+q)
	if err != nil {
		return err
	}
	serverlets, err := pullCollection(client, "/v2/serverlets"+q)
	if err != nil {
		return err
	}
	dns, err := pullDNS(client, q)
	if err != nil {
		return err
	}
	proxies, err := pullCollection(client, "/v2/proxies"+q)
	if err != nil {
		return err
	}
	storages, err := pullCollection(client, "/v2/storages"+q)
	if err != nil {
		return err
	}
	crons, err := pullCollection(client, "/v2/crons"+q)
	if err != nil {
		return err
	}
	staticos, err := pullOptionalCollection(client, "/v2/staticos"+q)
	if err != nil {
		return err
	}
	nameNetworks(serverlets, networks)
	pathMounts(serverlets)
	locks := resourceLocks(project, workspace, networks, serverlets, dns, proxies, storages, crons)
	for _, item := range staticos {
		if id, _ := item["id"].(string); id != "" && stringValue(item["name"]) != "" {
			locks["statico."+stringValue(item["name"])] = LockHash(id)
		}
	}
	data, _ := json.Marshal(map[string]interface{}{"$schema": constants.ProjectConfigSchemaURL, "workspace": workspace, "project": project.Name, "networks": portableItems(networks), "serverlets": portableItems(serverlets), "dns": portableItems(dns), "proxies": portableItems(proxies), "storages": portableItems(storages), "crons": portableItems(crons), "statico": staticoWebsites(staticos), "locks": locks})
	// JSON unmarshalling merges maps into an existing struct. Clear it first so
	// locks and resources are a complete snapshot, never a partial merge.
	region, install, build := cfg.Region, cfg.InstallCommand, cfg.BuildCommand
	*cfg = Config{}
	cfg.Region, cfg.InstallCommand, cfg.BuildCommand = region, install, build
	if err := json.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("reading pulled configuration: %w", err)
	}
	cfg.pulledWorkspaceID, cfg.pulledProjectID = project.WorkspaceId, project.Id
	if cfg.pulledWorkspaceID == "" {
		cfg.pulledWorkspaceID = workspaceID
	}
	if cfg.Project == "" {
		cfg.Project = project.Id
	}
	return cfg.Validate()
}

// nameNetworks replaces the network IDs in the "networks" of every serverlet
// with the network's name: the API stores IDs, the config (and a deploy that
// compares against it) uses names. An ID of no pulled network stays.
func nameNetworks(serverlets, networks []map[string]interface{}) {
	names := map[string]string{}
	for _, network := range networks {
		if id, name := stringValue(network["id"]), stringValue(network["name"]); id != "" && name != "" {
			names[id] = name
		}
	}
	for _, serverlet := range serverlets {
		list, ok := serverlet["networks"].([]interface{})
		if !ok {
			continue
		}
		named := make([]interface{}, len(list))
		for i, entry := range list {
			named[i] = entry
			if name, ok := names[stringValue(entry)]; ok {
				named[i] = name
			}
		}
		serverlet["networks"] = named
	}
}

// pathMounts turns the mounts of every serverlet from the API's storage ID ->
// path into the config's path -> storage.
func pathMounts(serverlets []map[string]interface{}) {
	for _, serverlet := range serverlets {
		mounts, ok := serverlet["mounts"].(map[string]interface{})
		if !ok {
			continue
		}
		inverted := make(map[string]interface{}, len(mounts))
		for storage, path := range mounts {
			inverted[stringValue(path)] = storage
		}
		serverlet["mounts"] = inverted
	}
}

func resourceLocks(project *api.Project, workspace string, networks, serverlets, dns, proxies, storages, crons []map[string]interface{}) map[string]string {
	locks := map[string]string{}
	add := func(key string, item map[string]interface{}) {
		if key != "" {
			if id, _ := item["id"].(string); id != "" {
				locks[key] = LockHash(id)
			}
		}
	}
	if workspace != "" && project.WorkspaceId != "" {
		locks["workspace"] = LockHash(project.WorkspaceId)
	}
	if project.Name != "" && project.Id != "" {
		locks["project"] = LockHash(project.Id)
	}
	for _, item := range networks {
		add("networks."+stringValue(item["name"]), item)
	}
	for _, item := range serverlets {
		add("serverlets."+stringValue(item["name"]), item)
	}
	for _, zone := range dns {
		zoneName := stringValue(zone["domain"])
		if zoneName == "" {
			zoneName = stringValue(zone["name"])
		}
		add("dns."+zoneName, zone)
		if records, ok := zone["records"].([]map[string]interface{}); ok {
			seen := map[string]int{}
			for _, record := range records {
				key := "dns." + escapeLockPart(zoneName) + ".records." + escapeLockPart(stringValue(record["name"])) + "." + escapeLockPart(stringValue(record["type"]))
				seen[key]++
				if seen[key] > 1 {
					key += fmt.Sprintf("[%d]", seen[key]-1)
				}
				add(key, record)
			}
		}
	}
	for _, item := range proxies {
		add("proxies."+stringValue(item["domain"]), item)
	}
	for _, item := range storages {
		add("storages."+stringValue(item["name"]), item)
	}
	for _, item := range crons {
		add("crons."+stringValue(item["name"]), item)
	}
	return locks
}

// LockHash returns the compact opaque value persisted for a live API resource ID.
func LockHash(id string) string { sum := sha1.Sum([]byte(id)); return hex.EncodeToString(sum[:]) }

func escapeLockPart(value string) string {
	return strings.NewReplacer("\\", "\\\\", ".", "\\.").Replace(value)
}

func stringValue(value interface{}) string { result, _ := value.(string); return result }

func resolveCloudWorkspaceID(client APIRequester, workspace string) (string, error) {
	if workspace == "" {
		return "", fmt.Errorf("workspace name or ID is required to pull a project")
	}
	if looksLikeResourceID(workspace) {
		return workspace, nil
	}
	var workspaces []map[string]interface{}
	if _, err := client.RequestInto("GET", "/v2/workspaces", nil, &workspaces); err != nil {
		return "", fmt.Errorf("listing workspaces: %w", err)
	}
	for _, item := range workspaces {
		if item["name"] == workspace {
			if id, _ := item["id"].(string); id != "" {
				return id, nil
			}
		}
	}
	return "", fmt.Errorf("workspace %q not found", workspace)
}

func looksLikeResourceID(value string) bool {
	return len(value) == 36 && value[8] == '-' && value[13] == '-' && value[18] == '-' && value[23] == '-'
}

func pullWorkspaceName(client APIRequester, project *api.Project) (string, error) {
	if project.Workspace != nil && project.Workspace.Name != "" {
		return project.Workspace.Name, nil
	}
	if project.WorkspaceId == "" {
		return "", nil
	}
	var workspace map[string]interface{}
	if _, err := client.RequestInto("GET", "/v2/workspaces/"+url.PathEscape(project.WorkspaceId), nil, &workspace); err != nil {
		return "", fmt.Errorf("fetching workspace: %w", err)
	}
	name, _ := workspace["name"].(string)
	if name == "" {
		return "", fmt.Errorf("workspace %q has no name", project.WorkspaceId)
	}
	return name, nil
}

// ResolveWorkspaceID is the ID of the workspace cfg.Workspace names (a name or an ID).
func (cfg *Config) ResolveWorkspaceID(client APIRequester) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("workspace name or ID is required")
	}
	return resolveCloudWorkspaceID(client, cfg.Workspace)
}

// ResolveProject finds the cloud project selected by cfg.Project in the
// workspace selected by cfg.Workspace.
func (cfg *Config) ResolveProject(client APIRequester) (*api.Project, error) {
	if cfg == nil || cfg.Project == "" {
		return nil, fmt.Errorf("project name or ID is required")
	}
	workspaceID, err := resolveCloudWorkspaceID(client, cfg.Workspace)
	if err != nil {
		return nil, err
	}
	return findCloudProject(client, cfg.Project, workspaceID)
}

func findCloudProject(client APIRequester, selector, workspaceID string) (*api.Project, error) {
	var projects []api.Project
	path := "/v2/projects"
	if workspaceID != "" {
		path += "?workspaceId=" + url.QueryEscape(workspaceID)
	}
	if _, err := client.RequestInto("GET", path, nil, &projects); err != nil {
		return nil, fmt.Errorf("listing projects: %w", err)
	}
	for i := range projects {
		if projects[i].Id == selector || projects[i].Name == selector {
			return &projects[i], nil
		}
	}
	return nil, fmt.Errorf("project %q not found", selector)
}
func pullCollection(client APIRequester, path string) ([]map[string]interface{}, error) {
	var items []map[string]interface{}
	if _, err := client.RequestInto("GET", path, nil, &items); err != nil {
		return nil, fmt.Errorf("fetching %s: %w", path, err)
	}
	return items, nil
}

// pullOptionalCollection is pullCollection for a resource older API servers do
// not have: a 404 means none.
func pullOptionalCollection(client APIRequester, path string) ([]map[string]interface{}, error) {
	var items []map[string]interface{}
	status, err := client.RequestInto("GET", path, nil, &items)
	if status == 404 {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", path, err)
	}
	return items, nil
}
func pullDNS(client APIRequester, query string) ([]map[string]interface{}, error) {
	var zones []map[string]interface{}
	if _, err := client.RequestInto("GET", "/v2/dns"+query, nil, &zones); err != nil {
		return nil, fmt.Errorf("fetching /v2/dns: %w", err)
	}
	for _, zone := range zones {
		if id, _ := zone["id"].(string); id != "" {
			var records []map[string]interface{}
			if _, err := client.RequestInto("GET", "/v2/dns/"+url.PathEscape(id)+"/records", nil, &records); err != nil {
				return nil, err
			}
			zone["records"] = records
		}
	}
	return zones, nil
}

// staticoWebsites keeps what a pulled website declares: its name, region and
// visibility. Limits, the storage and the served version are the cloud's own.
func staticoWebsites(items []map[string]interface{}) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		site := map[string]interface{}{}
		for _, key := range []string{"name", "regionId", "public"} {
			if value, ok := item[key]; ok && value != nil {
				site[key] = value
			}
		}
		out = append(out, site)
	}
	return out
}

// expandedRelations are the objects the API embeds next to the ID of a related resource.
var expandedRelations = []string{"serverletPlan", "dnsPlan", "proxyPlan", "storagePlan", "cronPlan", "region", "project", "workspace"}

func portableItems(items []map[string]interface{}) []map[string]interface{} {
	for _, item := range items {
		delete(item, "id")
		delete(item, "createdAt")
		delete(item, "updatedAt")
		delete(item, "startedAt")
		delete(item, "instances")
		delete(item, "state")
		delete(item, "status")
		delete(item, "projectId")
		delete(item, "dnsId")
		delete(item, "workspaceId")
		delete(item, "recordId")
		// relations the API expands next to their IDs (serverletPlan next to
		// serverletPlanId, ...): the config keeps the ID, and the expanded copy
		// does not match the generated model (a plan's cpu is the string "8.00")
		for _, relation := range expandedRelations {
			delete(item, relation)
		}
		if records, ok := item["records"].([]map[string]interface{}); ok {
			portableItems(records)
		}
	}
	return items
}

// Validate checks this project configuration using the shared declarative
// config validator.
func (cfg *Config) Validate() error {
	object, err := toObject(cfg)
	if err != nil {
		return err
	}
	if _, validationErrors, _ := validators.ConfigValidator.ValidateWithWarnings(object); len(validationErrors) > 0 {
		return formatValidationErrors("config", validationErrors)
	}
	return nil
}

// ErrNoFutrouConfig reports that no supported project config exists in the
// selected directory. Callers can use it to provide an empty configuration.
var ErrNoFutrouConfig = errors.New("no Futrou config found")

// Load replaces cfg with the configuration found in dir, or with file when it
// is given. Without a file the first match in constants.ProjectConfigFiles
// wins (JSON, then JavaScript/TypeScript, then YAML); the format is chosen by
// the extension and read by LoadJSON, LoadJS or LoadYAML. It returns the path
// that was loaded.
func (cfg *Config) Load(dir, file string) (string, error) {
	return cfg.LoadProject(dir, file, "")
}

// LoadProject is Load for a named project. Without a file it first looks for
// futrou.<project>.json (and the other formats, in the usual order), so one
// directory can hold several projects (futrou.dev.json, futrou.prod.json).
// When no such file exists it falls back to the default config and the config
// is renamed to the project; a project file that names no project of its own
// is named after the project too. With an empty project the default
// discovery applies: futrou.* and then a Docker Compose file.
func (cfg *Config) LoadProject(dir, file, project string) (string, error) {
	if project != "" {
		if err := ValidateProjectFileName(project); err != nil {
			return "", err
		}
	}
	path, projectFile, err := findProjectConfig(dir, file, project)
	if err != nil {
		return "", err
	}
	loaded := &Config{}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		err = loaded.LoadJSON(path)
	case ".js", ".cjs", ".mjs", ".ts":
		err = loaded.LoadJS(path)
	case ".yaml", ".yml":
		if IsComposeFile(path) {
			err = loaded.LoadCompose(path)
		} else {
			err = loaded.LoadYAML(path)
		}
	default:
		err = fmt.Errorf("unsupported config file %s", path)
	}
	if err != nil {
		return "", err
	}
	if project != "" && (!projectFile || loaded.Project == "") {
		loaded.Project = project
	}
	*cfg = *loaded
	return path, nil
}

// ValidateProjectFileName rejects project names that cannot be part of a
// config file name.
func ValidateProjectFileName(project string) error {
	if project == "" || strings.ContainsAny(project, `/\`) || strings.Contains(project, "..") || strings.HasPrefix(project, ".") {
		return fmt.Errorf("invalid project name %q for a config file name", project)
	}
	return nil
}

// LoadJSON replaces cfg with the configuration in the JSON file at path,
// validating it first.
func (cfg *Config) LoadJSON(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return cfg.loadDocument(path, raw)
}

// LoadJS replaces cfg with the configuration exported by the .js, .cjs, .mjs
// or .ts file at path, which is evaluated with node, bun or deno.
func (cfg *Config) LoadJS(path string) error {
	raw, err := evaluateJSConfig(path)
	if err != nil {
		return err
	}
	return cfg.loadDocument(path, raw)
}

// LoadYAML replaces cfg with the configuration in the YAML file at path. The
// document is converted to its JSON equivalent and then validated and loaded
// exactly like a JSON config.
func (cfg *Config) LoadYAML(path string) error {
	raw, err := yamlFileToJSON(path)
	if err != nil {
		return err
	}
	return cfg.loadDocument(path, raw)
}

// keyedCollections are the collections that can be written as an object keyed
// by the resource's name instead of a list, like services in Docker Compose.
// The value is the field the key fills in: the name, or the domain for
// resources that are identified by their domain.
var keyedCollections = map[string]string{
	"networks":   "name",
	"storages":   "name",
	"serverlets": "name",
	"crons":      "name",
	"statico":    "name",
	"proxies":    "domain",
	"dns":        "domain",
}

// normalizeKeyedCollections turns every collection written as an object into
// the equivalent list: each key becomes the item's name (or domain), unless
// the item sets it itself (a dns zone also gets its name). Items are ordered by key. Anything that is not an
// object is left for the validators to reject.
func normalizeKeyedCollections(object map[string]interface{}) {
	for collection, field := range keyedCollections {
		keyed, ok := object[collection].(map[string]interface{})
		if !ok {
			continue
		}
		keys := make([]string, 0, len(keyed))
		for key := range keyed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		items := make([]interface{}, 0, len(keys))
		for _, key := range keys {
			switch value := keyed[key].(type) {
			case nil:
				item := map[string]interface{}{field: key}
				if collection == "dns" {
					item["name"] = key
				}
				items = append(items, item)
			case map[string]interface{}:
				item := make(map[string]interface{}, len(value)+1)
				for k, v := range value {
					item[k] = v
				}
				if text, _ := item[field].(string); text == "" {
					item[field] = key
				}
				if collection == "dns" {
					// A zone has both a domain and a name; fill in the one that is missing.
					if text, _ := item["name"].(string); text == "" {
						item["name"] = item["domain"]
					}
				}
				items = append(items, item)
			default:
				items = append(items, value) // not an object: the validators report it
			}
		}
		object[collection] = items
	}
}

// loadDocument validates a JSON document and decodes it into cfg. path is
// only used in error messages.
func (cfg *Config) loadDocument(path string, raw []byte) error {
	var object map[string]interface{}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&object); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	normalizeStaticoInclude(object, includeOrders(path, raw))
	normalizeKeyedCollections(object)
	// Validators work with float64; normalize JSON numbers after preserving the
	// strict decode above, so 1.2 cannot silently become an integer.
	plain, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("encoding %s: %w", path, err)
	}
	if err := json.Unmarshal(plain, &object); err != nil {
		return err
	}
	if _, validationErrors, warnings := validators.ConfigValidator.ValidateWithWarnings(object); len(validationErrors) > 0 {
		return formatValidationErrors(path, validationErrors)
	} else {
		emitValidationWarnings(warnings)
	}
	var loaded Config
	if err := json.Unmarshal(plain, &loaded); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	*cfg = loaded
	return nil
}

// includeOrders reads the order of the keys of every statico "include" object
// straight from the file: decoding into a map forgets it, and the order is what
// the rules mean. A website is identified by its key (keyed form) or "#<index>"
// (list form). YAML is read from the file itself, JSON and JS from raw.
func includeOrders(path string, raw []byte) map[string][]string {
	if ext := strings.ToLower(filepath.Ext(path)); ext == ".yaml" || ext == ".yml" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		return yamlIncludeOrders(data)
	}
	return jsonIncludeOrders(raw)
}

func jsonIncludeOrders(raw []byte) map[string][]string {
	orders := map[string][]string{}
	var top map[string]json.RawMessage
	if json.Unmarshal(raw, &top) != nil {
		return orders
	}
	section := bytes.TrimSpace(top["statico"])
	includeKeys := func(item json.RawMessage) []string {
		var fields map[string]json.RawMessage
		if json.Unmarshal(item, &fields) != nil {
			return nil
		}
		keys, _ := utils.OrderedKeys(bytes.TrimSpace(fields["include"]))
		return keys
	}
	switch {
	case len(section) > 0 && section[0] == '{':
		var items map[string]json.RawMessage
		if json.Unmarshal(section, &items) == nil {
			for key, item := range items {
				orders[key] = includeKeys(item)
			}
		}
	case len(section) > 0 && section[0] == '[':
		var items []json.RawMessage
		if json.Unmarshal(section, &items) == nil {
			for i, item := range items {
				orders[fmt.Sprintf("#%d", i)] = includeKeys(item)
			}
		}
	}
	return orders
}

func yamlIncludeOrders(data []byte) map[string][]string {
	orders := map[string][]string{}
	var document yaml.Node
	if yaml.Unmarshal(data, &document) != nil || len(document.Content) == 0 {
		return orders
	}
	field := func(mapping *yaml.Node, name string) *yaml.Node {
		if mapping == nil || mapping.Kind != yaml.MappingNode {
			return nil
		}
		for i := 0; i+1 < len(mapping.Content); i += 2 {
			if mapping.Content[i].Value == name {
				return mapping.Content[i+1]
			}
		}
		return nil
	}
	includeKeys := func(item *yaml.Node) []string {
		include := field(item, "include")
		if include == nil || include.Kind != yaml.MappingNode {
			return nil
		}
		var keys []string
		for i := 0; i+1 < len(include.Content); i += 2 {
			keys = append(keys, include.Content[i].Value)
		}
		return keys
	}
	section := field(document.Content[0], "statico")
	if section == nil {
		return orders
	}
	switch section.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(section.Content); i += 2 {
			orders[section.Content[i].Value] = includeKeys(section.Content[i+1])
		}
	case yaml.SequenceNode:
		for i, item := range section.Content {
			orders[fmt.Sprintf("#%d", i)] = includeKeys(item)
		}
	}
	return orders
}

// normalizeStaticoInclude turns every include written as an object into the
// equivalent array of one-key objects, in the order of the file, so the order
// survives the unordered maps the document goes through. A key the order lacks
// (it should not happen) goes last, sorted.
func normalizeStaticoInclude(object map[string]interface{}, orders map[string][]string) {
	convert := func(id string, item interface{}) {
		site, ok := item.(map[string]interface{})
		if !ok {
			return
		}
		include, ok := site["include"].(map[string]interface{})
		if !ok {
			return
		}
		var keys []string
		seen := map[string]bool{}
		for _, key := range orders[id] {
			if _, present := include[key]; present && !seen[key] {
				keys = append(keys, key)
				seen[key] = true
			}
		}
		var rest []string
		for key := range include {
			if !seen[key] {
				rest = append(rest, key)
			}
		}
		sort.Strings(rest)
		rules := make([]interface{}, 0, len(include))
		for _, key := range append(keys, rest...) {
			rules = append(rules, map[string]interface{}{key: include[key]})
		}
		site["include"] = rules
	}
	switch section := object["statico"].(type) {
	case map[string]interface{}:
		for key, item := range section {
			convert(key, item)
		}
	case []interface{}:
		for i, item := range section {
			convert(fmt.Sprintf("#%d", i), item)
		}
	}
}

// Save writes cfg back to dir/file in the format of the file: JSON for
// .json (the default futrou.json when file is empty), YAML for .yaml/.yml.
// JavaScript and TypeScript configs are code and cannot be written back.
func (cfg *Config) Save(dir, file string) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("configuration is required")
	}
	if file == "" {
		file = filepath.Join(dir, "futrou.json")
	} else if !filepath.IsAbs(file) {
		file = filepath.Join(dir, file)
	}
	var err error
	switch strings.ToLower(filepath.Ext(file)) {
	case ".js", ".cjs", ".mjs", ".ts":
		err = cfg.SaveJS(file)
	case ".yaml", ".yml":
		if IsComposeFile(file) {
			err = fmt.Errorf("cannot save %s: a Docker Compose file is converted on load and never rewritten", file)
		} else {
			err = cfg.SaveYAML(file)
		}
	default:
		err = cfg.SaveJSON(file)
	}
	if err != nil {
		return "", err
	}
	return file, nil
}

// CanSave reports whether Save can write a config in the format of path.
func CanSave(path string) bool {
	if IsComposeFile(path) {
		return false
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".js", ".cjs", ".mjs", ".ts":
		return false
	}
	return true
}

// SaveJSON validates cfg and writes it to path as indented JSON.
func (cfg *Config) SaveJSON(path string) error {
	if err := cfg.validateForSave(path); err != nil {
		return err
	}
	data, err := cfg.ToJSON()
	if err != nil {
		return fmt.Errorf("encoding %s: %w", path, err)
	}
	return writeConfigFile(path, append(data, '\n'))
}

// SaveYAML validates cfg and writes it to path as block-style YAML, with the
// keys in the same order as the JSON form.
func (cfg *Config) SaveYAML(path string) error {
	if err := cfg.validateForSave(path); err != nil {
		return err
	}
	data, err := cfg.ToYAML()
	if err != nil {
		return fmt.Errorf("encoding %s: %w", path, err)
	}
	return writeConfigFile(path, data)
}

// SaveJS is not supported: JavaScript and TypeScript configs are code, so
// rewriting one from data would discard the code.
func (cfg *Config) SaveJS(path string) error {
	return fmt.Errorf("cannot save %s: writing JavaScript/TypeScript configs is not supported; use futrou.json or futrou.yaml", path)
}

func (cfg *Config) validateForSave(path string) error {
	object, err := toObject(cfg)
	if err != nil {
		return err
	}
	if _, validationErrors, warnings := validators.ConfigValidator.ValidateWithWarnings(object); len(validationErrors) > 0 {
		return formatValidationErrors(path, validationErrors)
	} else {
		emitValidationWarnings(warnings)
	}
	return nil
}

func writeConfigFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// ToYAML serializes this project config as block-style YAML.
func (cfg *Config) ToYAML() ([]byte, error) {
	data, err := cfg.ToJSON()
	if err != nil {
		return nil, err
	}
	// Decoding the JSON into a yaml.Node keeps the key order; clearing the
	// styles turns its flow style into block style.
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, err
	}
	var plain func(*yaml.Node)
	plain = func(node *yaml.Node) {
		node.Style = 0
		for _, child := range node.Content {
			plain(child)
		}
	}
	plain(&document)
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// LoadConfig returns the configuration found by Config.Load and its path.
func LoadConfig(dir, file string) (*Config, string, error) {
	return LoadProjectConfig(dir, file, "")
}

// LoadProjectConfig returns the configuration found by Config.LoadProject and
// its path.
func LoadProjectConfig(dir, file, project string) (*Config, string, error) {
	cfg := &Config{}
	path, err := cfg.LoadProject(dir, file, project)
	if err != nil {
		return nil, "", err
	}
	// the commands resolve relative paths in the config (include) against its folder
	if abs, err := filepath.Abs(path); err == nil {
		cfg.File = abs
	}
	return cfg, path, nil
}

// SaveConfig writes cfg with Config.Save.
func SaveConfig(dir, file string, cfg *Config) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("configuration is required")
	}
	return cfg.Save(dir, file)
}

// findProjectConfig resolves the file to load. It reports whether the file is
// specific to the project (futrou.<project>.*), as opposed to the default
// config used as a fallback.
func findProjectConfig(dir, file, project string) (path string, projectFile bool, err error) {
	if file == "" && project != "" {
		for _, name := range constants.ProjectConfigFilesFor(project) {
			candidate := filepath.Join(dir, name)
			if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
				return candidate, true, nil
			}
		}
	}
	path, err = findConfig(dir, file)
	return path, false, err
}

func findConfig(dir, file string) (string, error) {
	if file != "" {
		if !filepath.IsAbs(file) {
			file = filepath.Join(dir, file)
		}
		if _, err := os.Stat(file); err != nil {
			return "", fmt.Errorf("config file %s: %w", file, err)
		}
		return file, nil
	}
	for _, name := range constants.ProjectConfigFiles {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}
	return "", fmt.Errorf("%w (looked for: %s)", ErrNoFutrouConfig, strings.Join(constants.ProjectConfigFiles, ", "))
}

// evaluateJSConfig runs a .js, .cjs, .mjs or .ts config and returns the
// JSON document it exports.
func evaluateJSConfig(path string) ([]byte, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".js" && ext != ".cjs" && ext != ".mjs" && ext != ".ts" {
		return nil, fmt.Errorf("unsupported config file %s", path)
	}
	runtime, err := findRuntime(ext)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolving config path: %w", err)
	}
	fileURL := (&url.URL{Scheme: "file", Path: absolutePath}).String()
	// Dynamic import works for ESM and CommonJS. The config may default-export
	// an object or a sync/async function; Promise.resolve handles both forms.
	var script string
	var args []string
	if runtime.name == "deno" {
		script = `try { let m = await import(Deno.args[0]); let v = m.default; if (v === undefined) v = m; if (typeof v === 'function') v = await v(); if (v === null || typeof v !== 'object' || Array.isArray(v)) throw new Error('default export must be an object or function returning an object'); console.log(JSON.stringify(v)); } catch (e) { console.error(e.stack || e); Deno.exit(1); }`
		args = []string{"eval", "--quiet", script, fileURL}
	} else {
		script = `import(process.argv[1]).then(async m=>{let v=m.default; if(v===undefined) v=m; if(typeof v==='function') v=await v(); if(v===null||typeof v!=='object'||Array.isArray(v)) throw new Error('default export must be an object or function returning an object'); process.stdout.write(JSON.stringify(v));}).catch(e=>{console.error(e.stack||e);process.exit(1)})`
		args = []string{"-e", script, fileURL}
	}
	cmd := exec.CommandContext(ctx, runtime.path, args...)
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("evaluating timed out")
	}
	if err != nil {
		return nil, fmt.Errorf("evaluating with %s: %w", runtime.name, err)
	}
	return out, nil
}

// yamlFileToJSON parses a futrou.yaml/futrou.yml file and returns the JSON
// document with the same content, so it is validated and loaded exactly like
// futrou.json.
func yamlFileToJSON(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	var value interface{}
	if err := decoder.Decode(&value); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("parsing %s: file is empty", path)
		}
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	var extra interface{}
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parsing %s: multiple YAML documents are not supported", path)
	}
	converted, err := yamlToJSONValue(value)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if _, ok := converted.(map[string]interface{}); !ok {
		return nil, fmt.Errorf("parsing %s: the top level must be a mapping", path)
	}
	data, err := json.Marshal(converted)
	if err != nil {
		return nil, fmt.Errorf("converting %s to JSON: %w", path, err)
	}
	return data, nil
}

// yamlToJSONValue makes a decoded YAML value JSON-compatible: mapping keys
// must be strings, and timestamps become RFC 3339 strings.
func yamlToJSONValue(value interface{}) (interface{}, error) {
	switch v := value.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(v))
		for key, item := range v {
			converted, err := yamlToJSONValue(item)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", key, err)
			}
			out[key] = converted
		}
		return out, nil
	case map[interface{}]interface{}:
		for key := range v {
			return nil, fmt.Errorf("mapping key %v must be a string", key)
		}
		return map[string]interface{}{}, nil
	case []interface{}:
		out := make([]interface{}, len(v))
		for i, item := range v {
			converted, err := yamlToJSONValue(item)
			if err != nil {
				return nil, fmt.Errorf("item %d: %w", i, err)
			}
			out[i] = converted
		}
		return out, nil
	case time.Time:
		return v.Format(time.RFC3339), nil
	default:
		return v, nil
	}
}

type jsRuntime struct {
	name string
	path string
}

func findRuntime(extension string) (jsRuntime, error) {
	if extension == ".ts" {
		if bun, err := exec.LookPath("bun"); err == nil {
			return jsRuntime{name: "bun", path: bun}, nil
		}
		if deno, err := exec.LookPath("deno"); err == nil {
			return jsRuntime{name: "deno", path: deno}, nil
		}
		return jsRuntime{}, fmt.Errorf("requires bun or deno to evaluate TypeScript config (neither found in PATH)")
	}
	if node, err := exec.LookPath("node"); err == nil {
		return jsRuntime{name: "node", path: node}, nil
	}
	if bun, err := exec.LookPath("bun"); err == nil {
		return jsRuntime{name: "bun", path: bun}, nil
	}
	if deno, err := exec.LookPath("deno"); err == nil {
		return jsRuntime{name: "deno", path: deno}, nil
	}
	return jsRuntime{}, fmt.Errorf("requires node, bun, or deno to evaluate executable config (none found in PATH)")
}

func toObject(cfg *Config) (map[string]interface{}, error) {
	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	var object map[string]interface{}
	if err := json.Unmarshal(data, &object); err != nil {
		return nil, err
	}
	return object, nil
}

// formatValidationErrors retains every error returned by ConfigValidator and
// sorts fields so CLI output is stable and easy to act on.
func formatValidationErrors(path string, validationErrors map[string]string) error {
	fields := make([]string, 0, len(validationErrors))
	for field := range validationErrors {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	lines := make([]string, 0, len(fields))
	for _, field := range fields {
		lines = append(lines, fmt.Sprintf("  %s: %s", field, validationErrors[field]))
	}
	return fmt.Errorf("validating %s:\n%s", path, strings.Join(lines, "\n"))
}

func emitValidationWarnings(warnings []string) {
	for _, warning := range warnings {
		logger.Warn("%s", warning)
	}
}
