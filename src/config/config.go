package config

import (
	"context"
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

// ProxyConfig, VolumeConfig, and CronConfig are the generated API models.
// They need no project-config-specific adjustment, so aliases retain every
// API field without duplicating the generated definitions.
type ProxyConfig = api.Proxy
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
	return schema
}

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
