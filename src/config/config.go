package config

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
	"futrou-cli/src/validators"
)

// Config is the declarative project configuration stored in futrou.json
// (or exported by futrou.js/cjs/mjs/ts). It is built from the generated API
// resource types, with deployment-only fields added where the API response
// models do not expose them.
type Config struct {
	Schema     string            `json:"$schema,omitempty"`
	Workspace  string            `json:"workspace,omitempty"`
	Project    string            `json:"project,omitempty"`
	Serverlets []ServerletConfig `json:"serverlets,omitempty"`
	DNS        []DNSConfig       `json:"dns,omitempty"`
	Proxies    []ProxyConfig     `json:"proxies,omitempty"`
	Volumes    []VolumeConfig    `json:"volumes,omitempty"`
	Crons      []CronConfig      `json:"crons,omitempty"`
	Locks      map[string]string `json:"locks,omitempty"`
}

// ServerletConfig exposes the Serverlet API model plus creation/deployment
// fields that are not part of a Serverlet response.
type ServerletConfig struct {
	api.Serverlet
	ServerletPlanId string                 `json:"serverletPlanId,omitempty"`
	WorkspaceId     string                 `json:"workspaceId,omitempty"`
	ProjectId       string                 `json:"projectId,omitempty"`
	Env             map[string]string      `json:"env,omitempty"`
	Volumes         []ServerletVolumeMount `json:"volumes,omitempty"`
	Ports           []ServerletPort        `json:"ports,omitempty"`
	Scaling         *ServerletScaling      `json:"scaling,omitempty"`
}

// MarshalJSON keeps declarative serverlet files stable and human-readable:
// identity and image are emitted before sizing and generated API fields.
func (s ServerletConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Name            string                 `json:"name,omitempty"`
		Image           string                 `json:"image,omitempty"`
		ServerletPlanId string                 `json:"serverletPlanId,omitempty"`
		WorkspaceId     string                 `json:"workspaceId,omitempty"`
		ProjectId       string                 `json:"projectId,omitempty"`
		Ram             float64                `json:"ram,omitempty"`
		Cpu             float64                `json:"cpu,omitempty"`
		MinInstances    float64                `json:"minInstances"`
		MaxInstances    float64                `json:"maxInstances"`
		Env             map[string]string      `json:"env,omitempty"`
		Volumes         []ServerletVolumeMount `json:"volumes,omitempty"`
		Ports           []ServerletPort        `json:"ports,omitempty"`
		Scaling         *ServerletScaling      `json:"scaling,omitempty"`
		NetworkId       string                 `json:"networkId,omitempty"`
		Runtime         string                 `json:"runtime,omitempty"`
	}{s.Name, s.Image, s.ServerletPlanId, s.WorkspaceId, s.ProjectId, s.Ram, s.Cpu, s.MinInstances, s.MaxInstances, s.Env, s.Volumes, s.Ports, s.Scaling, s.NetworkId, s.Runtime})
}

type ServerletVolumeMount struct {
	VolumeId  string `json:"volumeId,omitempty"`
	MountPath string `json:"mountPath,omitempty"`
	ReadOnly  bool   `json:"readOnly,omitempty"`
}

type ServerletPort struct {
	Port     int    `json:"port"`
	Protocol string `json:"protocol,omitempty"`
}

type ServerletScaling struct {
	CpuPercent   int `json:"cpuPercent,omitempty"`
	RamPercent   int `json:"ramPercent,omitempty"`
	UpCooldown   int `json:"upCooldown,omitempty"`
	DownCooldown int `json:"downCooldown,omitempty"`
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

// ProxyConfig, VolumeConfig, and CronConfig are the generated API models.
// They need no project-config-specific adjustment, so aliases retain every
// API field without duplicating the generated definitions.
type ProxyConfig api.Proxy

const (
	defaultProxyPort     = 80
	defaultProxyType     = "http"
	defaultProxyStrategy = "round-robin"
)

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
	typeName := p.Type
	if typeName == defaultProxyType {
		typeName = ""
	}
	strategy := p.Strategy
	if strategy == defaultProxyStrategy {
		strategy = ""
	}
	return json.Marshal(struct {
		Domain          string  `json:"domain,omitempty"`
		Type            string  `json:"type,omitempty"`
		Target          string  `json:"target,omitempty"`
		Port            float64 `json:"port,omitempty"`
		Compress        *bool   `json:"compress,omitempty"`
		EnforceHttps    *bool   `json:"enforceHttps,omitempty"`
		FollowRedirects *bool   `json:"followRedirects,omitempty"`
		PreserveHeaders *bool   `json:"preserveHeaders,omitempty"`
		PreserveHost    *bool   `json:"preserveHost,omitempty"`
		PreservePath    *bool   `json:"preservePath,omitempty"`
		PreserveQuery   *bool   `json:"preserveQuery,omitempty"`
		VerifyTls       *bool   `json:"verifyTls,omitempty"`
		Strategy        string  `json:"strategy,omitempty"`
	}{p.Domain, typeName, p.Target, port, differentBool(p.Compress, false), differentBool(p.EnforceHttps, false), differentBool(p.FollowRedirects, false), differentBool(p.PreserveHeaders, true), differentBool(p.PreserveHost, true), differentBool(p.PreservePath, true), differentBool(p.PreserveQuery, true), differentBool(p.VerifyTls, true), strategy})
}

type VolumeConfig = api.Volume
type CronConfig = api.Cron

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
	volumes, err := pullCollection(client, "/v2/volumes"+q)
	if err != nil {
		return err
	}
	crons, err := pullCollection(client, "/v2/crons"+q)
	if err != nil {
		return err
	}
	locks := resourceLocks(project, workspace, serverlets, dns, proxies, volumes, crons)
	data, _ := json.Marshal(map[string]interface{}{"$schema": constants.ProjectConfigSchemaURL, "workspace": workspace, "project": project.Name, "serverlets": portableItems(serverlets), "dns": portableItems(dns), "proxies": portableItems(proxies), "volumes": portableItems(volumes), "crons": portableItems(crons), "locks": locks})
	// JSON unmarshalling merges maps into an existing struct. Clear it first so
	// locks and resources are a complete snapshot, never a partial merge.
	*cfg = Config{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("reading pulled configuration: %w", err)
	}
	if cfg.Project == "" {
		cfg.Project = project.Id
	}
	return cfg.Validate()
}

func resourceLocks(project *api.Project, workspace string, serverlets, dns, proxies, volumes, crons []map[string]interface{}) map[string]string {
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
	for _, item := range volumes {
		add("volumes."+stringValue(item["name"]), item)
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

// Load replaces cfg with the configuration read from dir/file.
func (cfg *Config) Load(dir, file string) (string, error) {
	loaded, path, err := LoadConfig(dir, file)
	if err != nil {
		return "", err
	}
	*cfg = *loaded
	return path, nil
}

// Save writes cfg to dir/file.
func (cfg *Config) Save(dir, file string) (string, error) { return SaveConfig(dir, file, cfg) }

// ErrNoFutrouConfig reports that no supported project config exists in the
// selected directory. Callers can use it to provide an empty configuration.
var ErrNoFutrouConfig = errors.New("no Futrou config found")

// LoadConfig loads a file, or searches dir in the documented priority
// order when file is empty. JSON always wins over executable configuration.
func LoadConfig(dir, file string) (*Config, string, error) {
	path, err := findConfig(dir, file)
	if err != nil {
		return nil, "", err
	}
	raw, err := readProjectConfig(path)
	if err != nil {
		return nil, "", err
	}
	var object map[string]interface{}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&object); err != nil {
		return nil, "", fmt.Errorf("parsing %s: %w", path, err)
	}
	// Validators work with float64; normalize JSON numbers after preserving the
	// strict decode above, so 1.2 cannot silently become an integer.
	plain, err := json.Marshal(object)
	if err != nil {
		return nil, "", fmt.Errorf("encoding %s: %w", path, err)
	}
	if err := json.Unmarshal(plain, &object); err != nil {
		return nil, "", err
	}
	if _, validationErrors, warnings := validators.ConfigValidator.ValidateWithWarnings(object); len(validationErrors) > 0 {
		return nil, "", formatValidationErrors(path, validationErrors)
	} else {
		emitValidationWarnings(warnings)
	}
	var cfg Config
	if err := json.Unmarshal(plain, &cfg); err != nil {
		return nil, "", fmt.Errorf("parsing %s: %w", path, err)
	}
	return &cfg, path, nil
}

// Save writes validated configuration as pretty JSON. An empty file name uses
// <dir>/futrou.json, never overwriting an executable config by accident.
func SaveConfig(dir, file string, cfg *Config) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("configuration is required")
	}
	if file == "" {
		file = filepath.Join(dir, "futrou.json")
	} else if !filepath.IsAbs(file) {
		file = filepath.Join(dir, file)
	}
	object, err := toObject(cfg)
	if err != nil {
		return "", err
	}
	if _, validationErrors, warnings := validators.ConfigValidator.ValidateWithWarnings(object); len(validationErrors) > 0 {
		return "", formatValidationErrors(file, validationErrors)
	} else {
		emitValidationWarnings(warnings)
	}
	data, err := cfg.ToJSON()
	if err != nil {
		return "", fmt.Errorf("encoding %s: %w", file, err)
	}
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		return "", fmt.Errorf("creating config directory: %w", err)
	}
	if err := os.WriteFile(file, append(data, '\n'), 0644); err != nil {
		return "", fmt.Errorf("writing %s: %w", file, err)
	}
	return file, nil
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

func readProjectConfig(path string) ([]byte, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".json" {
		return os.ReadFile(path)
	}
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
