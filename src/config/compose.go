package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"futrou-cli/src/constants"
	"futrou-cli/src/logger"

	"gopkg.in/yaml.v3"
)

// IsComposeFile reports whether path names a Docker Compose file.
func IsComposeFile(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	for _, name := range constants.ComposeFiles {
		if base == name {
			return true
		}
	}
	return false
}

// composeWarn reports a Compose option that has no Futrou equivalent.
var composeWarn = func(message string) { logger.Warn("%s", message) }

// LoadCompose replaces cfg with the Docker Compose file at path, converted to
// the equivalent futrou.json document and then validated and loaded like it.
// Everything that cannot be converted is logged as a warning and skipped.
func (cfg *Config) LoadCompose(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	warn := func(message string) { composeWarn(filepath.Base(path) + ": " + message) }
	object, err := ConvertCompose(raw, os.LookupEnv, warn)
	if err != nil {
		return fmt.Errorf("converting %s: %w", path, err)
	}
	document, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("converting %s: %w", path, err)
	}
	return cfg.loadDocument(path, document)
}

// composeServiceKeys are the service options ConvertCompose understands.
var composeServiceKeys = map[string]bool{
	"image": true, "command": true, "entrypoint": true, "environment": true, "ports": true,
	"networks": true, "volumes": true, "depends_on": true, "healthcheck": true,
	"working_dir": true, "user": true, "deploy": true, "stop_grace_period": true,
}

// ConvertCompose converts a Docker Compose document to the equivalent
// futrou.json object. Services become serverlets, networks and named volumes
// become networks and storages. Options without a Futrou equivalent are
// reported through warn and left out. lookup supplies ${VARIABLES}.
//
// The Futrou-only settings Compose cannot express (serverletPlanId, regionId,
// workspace, project, storagePlanId, ...) go in `x-futrou` mappings, at the
// top level and on a service or volume; they are merged into the result.
func ConvertCompose(raw []byte, lookup func(string) (string, bool), warn func(string)) (map[string]interface{}, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	var document map[string]interface{}
	if err := decoder.Decode(&document); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("the file is empty")
		}
		return nil, err
	}
	var extra interface{}
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("multiple YAML documents are not supported")
	}
	value, err := yamlToJSONValue(interpolateValue(document, lookup, warn))
	if err != nil {
		return nil, err
	}
	document, _ = value.(map[string]interface{})

	c := &composeConverter{warn: warn}
	return c.convert(document)
}

type composeConverter struct {
	warn func(string)
}

func (c *composeConverter) warnf(format string, args ...interface{}) {
	c.warn(fmt.Sprintf(format, args...))
}

func (c *composeConverter) convert(document map[string]interface{}) (map[string]interface{}, error) {
	out := map[string]interface{}{}
	for key, value := range document {
		switch {
		case key == "services" || key == "networks" || key == "volumes" || key == "x-futrou":
		case key == "version":
			// obsolete in the Compose spec; nothing to convert
		case key == "name":
			if name, ok := value.(string); ok && name != "" {
				out["project"] = name
			}
		case strings.HasPrefix(key, "x-"):
		default:
			c.warnf("%s is not supported and was ignored", key)
		}
	}

	services, _ := document["services"].(map[string]interface{})
	if len(services) == 0 {
		c.warnf("there are no services to convert")
	}
	volumes, _ := document["volumes"].(map[string]interface{})

	var serverlets []interface{}
	usedNetworks := map[string]bool{}
	for _, name := range sortedKeys(services) {
		serverlet := c.service(name, services[name], volumes, services, usedNetworks)
		if serverlet != nil {
			serverlets = append(serverlets, serverlet)
		}
	}
	if len(serverlets) > 0 {
		out["serverlets"] = serverlets
	}
	if networks := c.networks(document["networks"]); len(networks) > 0 {
		out["networks"] = networks
	}
	if storages := c.storages(volumes); len(storages) > 0 {
		out["storages"] = storages
	}

	if extension, ok := document["x-futrou"].(map[string]interface{}); ok {
		for key, value := range extension {
			out[key] = value
		}
	}
	return out, nil
}

func (c *composeConverter) service(name string, raw interface{}, volumes, services map[string]interface{}, usedNetworks map[string]bool) map[string]interface{} {
	where := "services." + name
	spec, ok := raw.(map[string]interface{})
	if !ok {
		c.warnf("%s is not a mapping and was ignored", where)
		return nil
	}
	serverlet := map[string]interface{}{"name": name}

	for _, key := range sortedKeys(spec) {
		if !composeServiceKeys[key] && !strings.HasPrefix(key, "x-") {
			c.warnf("%s.%s is not supported and was ignored", where, key)
		}
	}

	if image, ok := spec["image"].(string); ok && image != "" {
		serverlet["image"] = image
	} else if _, hasBuild := spec["build"]; hasBuild {
		c.warnf("%s has no image: building from source is not supported, push an image and set image", where)
	} else {
		c.warnf("%s has no image", where)
	}

	if command := c.commandLine(where, spec["entrypoint"], spec["command"]); command != "" {
		serverlet["entrypoint"] = command
	}
	if env := c.environment(where, spec["environment"]); len(env) > 0 {
		serverlet["env"] = env
	}
	if ports := c.ports(where, spec["ports"]); len(ports) > 0 {
		serverlet["ports"] = ports
	}
	if networks := c.serviceNetworks(where, spec["networks"], spec["network_mode"]); len(networks) > 0 {
		serverlet["networks"] = networks
	}
	if mounts := c.mounts(where, spec["volumes"], volumes); len(mounts) > 0 {
		serverlet["mounts"] = mounts
	}
	if needs := c.needs(where, spec["depends_on"], services); len(needs) > 0 {
		serverlet["needs"] = needs
	}
	c.healthcheck(where, spec["healthcheck"], serverlet)
	if dir, ok := spec["working_dir"].(string); ok && dir != "" {
		serverlet["cwd"] = dir
	}
	if user := scalarString(spec["user"]); user != "" {
		serverlet["user"] = user
	}
	if grace, present := spec["stop_grace_period"]; present {
		if seconds, ok := c.seconds(where+".stop_grace_period", grace); ok {
			serverlet["stopTimeout"] = seconds
		}
	}
	c.deploy(where, spec["deploy"], serverlet)

	if extension, ok := spec["x-futrou"].(map[string]interface{}); ok {
		for key, value := range extension {
			serverlet[key] = value
		}
	}
	if serverlet["serverletPlanId"] == nil {
		c.warnf("%s: Compose has no plan; set serverletPlanId under x-futrou", where)
	}
	return serverlet
}

// commandLine joins a Compose entrypoint and command into the single start
// command Futrou takes. Lists are joined with spaces; items with spaces are
// quoted.
func (c *composeConverter) commandLine(where string, entrypoint, command interface{}) string {
	var parts []string
	for _, value := range []interface{}{entrypoint, command} {
		switch v := value.(type) {
		case nil:
		case string:
			if strings.TrimSpace(v) != "" {
				parts = append(parts, strings.TrimSpace(v))
			}
		case []interface{}:
			for _, item := range v {
				text := scalarString(item)
				if strings.ContainsAny(text, " \t") {
					text = strconv.Quote(text)
				}
				parts = append(parts, text)
			}
		default:
			c.warnf("%s: entrypoint and command must be a string or a list", where)
		}
	}
	return strings.Join(parts, " ")
}

func (c *composeConverter) environment(where string, raw interface{}) map[string]interface{} {
	env := map[string]interface{}{}
	set := func(key string, value interface{}, present bool) {
		if present && value != nil {
			env[key] = scalarString(value)
			return
		}
		if host, ok := os.LookupEnv(key); ok {
			env[key] = host
			return
		}
		c.warnf("%s.environment.%s has no value and is not set in the environment, so it was ignored", where, key)
	}
	switch v := raw.(type) {
	case nil:
	case map[string]interface{}:
		for _, key := range sortedKeys(v) {
			set(key, v[key], true)
		}
	case []interface{}:
		for _, item := range v {
			text := scalarString(item)
			if key, value, found := strings.Cut(text, "="); found {
				env[key] = value
			} else {
				set(text, nil, false)
			}
		}
	default:
		c.warnf("%s.environment must be a mapping or a list", where)
	}
	return env
}

var composePortPattern = regexp.MustCompile(`^\d+$`)

func (c *composeConverter) ports(where string, raw interface{}) []interface{} {
	list, _ := raw.([]interface{})
	var ports []interface{}
	seen := map[string]bool{}
	add := func(port, protocol string) {
		key := port + "/" + protocol
		if seen[key] {
			return
		}
		seen[key] = true
		number, _ := strconv.Atoi(port)
		entry := map[string]interface{}{"port": number}
		if protocol != "" && protocol != "tcp" {
			entry["protocol"] = protocol
		}
		ports = append(ports, entry)
	}
	for _, item := range list {
		switch v := item.(type) {
		case map[string]interface{}:
			target := scalarString(v["target"])
			if !composePortPattern.MatchString(target) {
				c.warnf("%s.ports: %v has no valid target port and was ignored", where, v)
				continue
			}
			protocol, _ := v["protocol"].(string)
			add(target, strings.ToLower(protocol))
		default:
			text := scalarString(item)
			protocol := ""
			if base, proto, found := strings.Cut(text, "/"); found {
				text, protocol = base, strings.ToLower(proto)
			}
			parts := strings.Split(text, ":")
			target := parts[len(parts)-1]
			if !composePortPattern.MatchString(target) {
				c.warnf("%s.ports: %q (port ranges and names) is not supported and was ignored", where, scalarString(item))
				continue
			}
			add(target, protocol)
		}
	}
	return ports
}

func (c *composeConverter) serviceNetworks(where string, raw, mode interface{}) []interface{} {
	if mode != nil {
		c.warnf("%s.network_mode is not supported and was ignored", where)
	}
	var names []string
	switch v := raw.(type) {
	case nil:
	case []interface{}:
		for _, item := range v {
			names = append(names, scalarString(item))
		}
	case map[string]interface{}:
		names = sortedKeys(v)
	default:
		c.warnf("%s.networks must be a list or a mapping", where)
	}
	var out []interface{}
	for _, name := range names {
		if name == "default" {
			continue // Futrou has no implicit default network
		}
		out = append(out, name)
	}
	return out
}

func (c *composeConverter) networks(raw interface{}) []interface{} {
	declared, _ := raw.(map[string]interface{})
	var out []interface{}
	for _, name := range sortedKeys(declared) {
		spec, _ := declared[name].(map[string]interface{})
		where := "networks." + name
		if external, _ := spec["external"].(bool); external {
			c.warnf("%s is external and was not created; make sure a network named %q exists", where, name)
			continue
		}
		network := map[string]interface{}{"name": name}
		if driver, _ := spec["driver"].(string); driver != "" && driver != "bridge" {
			c.warnf("%s.driver %q is not supported and was ignored", where, driver)
		}
		if internal, ok := spec["internal"].(bool); ok && internal {
			network["internal"] = true
		}
		if ipam, ok := spec["ipam"].(map[string]interface{}); ok {
			configs, _ := ipam["config"].([]interface{})
			for _, entry := range configs {
				subnet, _ := entry.(map[string]interface{})["subnet"].(string)
				switch {
				case subnet == "":
				case strings.Contains(subnet, ":"):
					network["ipv6"] = subnet
				default:
					network["ipv4"] = subnet
				}
			}
		}
		for _, key := range sortedKeys(spec) {
			switch key {
			case "driver", "internal", "ipam", "external":
			default:
				if !strings.HasPrefix(key, "x-") {
					c.warnf("%s.%s is not supported and was ignored", where, key)
				}
			}
		}
		if extension, ok := spec["x-futrou"].(map[string]interface{}); ok {
			for key, value := range extension {
				network[key] = value
			}
		}
		out = append(out, network)
	}
	return out
}

// mounts converts service volumes. Named volumes become storage mounts (mount
// path -> storage name); bind mounts, tmpfs and anonymous volumes have no
// equivalent.
func (c *composeConverter) mounts(where string, raw interface{}, volumes map[string]interface{}) map[string]interface{} {
	list, _ := raw.([]interface{})
	mounts := map[string]interface{}{}
	for _, item := range list {
		var source, target, kind string
		switch v := item.(type) {
		case map[string]interface{}:
			source, target, kind = scalarString(v["source"]), scalarString(v["target"]), scalarString(v["type"])
		default:
			parts := strings.Split(scalarString(item), ":")
			switch len(parts) {
			case 1:
				target = parts[0]
			default:
				source, target = parts[0], parts[1]
			}
		}
		_, declared := volumes[source]
		switch {
		case kind == "tmpfs":
			c.warnf("%s.volumes: tmpfs mounts are not supported (%s)", where, target)
		case target == "" || source == "":
			c.warnf("%s.volumes: the anonymous volume %q is not supported and was ignored", where, target)
		case kind == "bind" || strings.HasPrefix(source, ".") || strings.HasPrefix(source, "/") || strings.HasPrefix(source, "~"):
			c.warnf("%s.volumes: the bind mount %s:%s is not supported; upload the files to a storage with `futrou storage file upload`", where, source, target)
		case declared || kind == "volume":
			mounts[target] = source
		default:
			c.warnf("%s.volumes: %q is not a declared volume and was ignored", where, source)
		}
	}
	return mounts
}

func (c *composeConverter) storages(raw map[string]interface{}) []interface{} {
	var out []interface{}
	for _, name := range sortedKeys(raw) {
		spec, _ := raw[name].(map[string]interface{})
		where := "volumes." + name
		if external, _ := spec["external"].(bool); external {
			c.warnf("%s is external and was not created; make sure a storage named %q exists", where, name)
			continue
		}
		storage := map[string]interface{}{"name": name}
		for _, key := range sortedKeys(spec) {
			if key != "x-futrou" && !strings.HasPrefix(key, "x-") {
				c.warnf("%s.%s is not supported and was ignored", where, key)
			}
		}
		if extension, ok := spec["x-futrou"].(map[string]interface{}); ok {
			for key, value := range extension {
				storage[key] = value
			}
		}
		if storage["storagePlanId"] == nil {
			c.warnf("%s: Compose has no storage plan; set storagePlanId under x-futrou", where)
		}
		out = append(out, storage)
	}
	return out
}

func (c *composeConverter) needs(where string, raw interface{}, services map[string]interface{}) []interface{} {
	var out []interface{}
	add := func(name, condition string) {
		if _, ok := services[name]; !ok {
			c.warnf("%s.depends_on: %q is not a service and was ignored", where, name)
			return
		}
		switch condition {
		case "", "service_started":
			out = append(out, name)
		case "service_healthy":
			out = append(out, name+":healthy")
		default:
			c.warnf("%s.depends_on.%s: the condition %q is not supported, waiting for it to start instead", where, name, condition)
			out = append(out, name)
		}
	}
	switch v := raw.(type) {
	case nil:
	case []interface{}:
		for _, item := range v {
			add(scalarString(item), "")
		}
	case map[string]interface{}:
		for _, name := range sortedKeys(v) {
			spec, _ := v[name].(map[string]interface{})
			add(name, scalarString(spec["condition"]))
		}
	default:
		c.warnf("%s.depends_on must be a list or a mapping", where)
	}
	return out
}

func (c *composeConverter) healthcheck(where string, raw interface{}, serverlet map[string]interface{}) {
	spec, ok := raw.(map[string]interface{})
	if !ok {
		return
	}
	if disabled, _ := spec["disable"].(bool); disabled {
		return
	}
	switch test := spec["test"].(type) {
	case string:
		serverlet["healthcheck"] = test
	case []interface{}:
		var words []string
		for _, item := range test {
			words = append(words, scalarString(item))
		}
		switch {
		case len(words) == 0 || words[0] == "NONE":
			return
		case words[0] == "CMD" || words[0] == "CMD-SHELL":
			serverlet["healthcheck"] = strings.Join(words[1:], " ")
		default:
			serverlet["healthcheck"] = strings.Join(words, " ")
		}
	}
	for key, field := range map[string]string{"interval": "healthcheckInterval", "timeout": "healthcheckTimeout", "start_period": "startTimeout"} {
		if value, present := spec[key]; present {
			if seconds, ok := c.seconds(where+".healthcheck."+key, value); ok {
				serverlet[field] = seconds
			}
		}
	}
	if retries, present := spec["retries"]; present {
		if n, err := strconv.Atoi(scalarString(retries)); err == nil {
			serverlet["healthcheckRetries"] = n
		} else {
			c.warnf("%s.healthcheck.retries %q is not a number and was ignored", where, scalarString(retries))
		}
	}
	for _, key := range sortedKeys(spec) {
		switch key {
		case "test", "interval", "timeout", "start_period", "retries", "disable":
		default:
			if !strings.HasPrefix(key, "x-") {
				c.warnf("%s.healthcheck.%s is not supported and was ignored", where, key)
			}
		}
	}
}

func (c *composeConverter) deploy(where string, raw interface{}, serverlet map[string]interface{}) {
	spec, ok := raw.(map[string]interface{})
	if !ok {
		return
	}
	for _, key := range sortedKeys(spec) {
		switch key {
		case "replicas":
			if n, err := strconv.Atoi(scalarString(spec[key])); err == nil && n >= 0 {
				serverlet["minInstances"] = n
				serverlet["maxInstances"] = n
			} else {
				c.warnf("%s.deploy.replicas %q is not a number and was ignored", where, scalarString(spec[key]))
			}
		default:
			if !strings.HasPrefix(key, "x-") {
				c.warnf("%s.deploy.%s is not supported and was ignored; size the serverlet with serverletPlanId", where, key)
			}
		}
	}
}

// seconds parses a Compose duration such as "30s" or "1m30s".
func (c *composeConverter) seconds(where string, value interface{}) (float64, bool) {
	text := scalarString(value)
	duration, err := time.ParseDuration(text)
	if err != nil {
		c.warnf("%s: %q is not a duration like 30s or 1m30s and was ignored", where, text)
		return 0, false
	}
	return duration.Seconds(), true
}

func scalarString(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return fmt.Sprint(v)
	}
}

func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

var composeVariable = regexp.MustCompile(`\$(\$|[A-Za-z_][A-Za-z0-9_]*|\{[^}]*\})`)

// interpolateValue expands ${VAR}, ${VAR:-default}, ${VAR-default} and $VAR in
// every string of a decoded Compose document; $$ is a literal dollar sign.
func interpolateValue(value interface{}, lookup func(string) (string, bool), warn func(string)) interface{} {
	switch v := value.(type) {
	case string:
		return composeVariable.ReplaceAllStringFunc(v, func(match string) string {
			name := match[1:]
			if name == "$" {
				return "$"
			}
			fallback, hasFallback := "", false
			if strings.HasPrefix(name, "{") {
				name = strings.Trim(name, "{}")
				if i := strings.Index(name, ":-"); i >= 0 {
					name, fallback, hasFallback = name[:i], name[i+2:], true
					if got, ok := lookup(name); ok && got != "" {
						return got
					}
					return fallback
				}
				if i := strings.Index(name, "-"); i >= 0 {
					name, fallback, hasFallback = name[:i], name[i+1:], true
				}
			}
			if got, ok := lookup(name); ok {
				return got
			}
			if hasFallback {
				return fallback
			}
			warn(fmt.Sprintf("the variable %s is not set; using an empty string", name))
			return ""
		})
	case map[string]interface{}:
		out := make(map[string]interface{}, len(v))
		for key, item := range v {
			out[key] = interpolateValue(item, lookup, warn)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(v))
		for i, item := range v {
			out[i] = interpolateValue(item, lookup, warn)
		}
		return out
	default:
		return value
	}
}
