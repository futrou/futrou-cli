package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func noEnv(string) (string, bool) { return "", false }

// convert runs ConvertCompose and returns the object and the warnings.
func convert(t *testing.T, doc string, env map[string]string) (map[string]interface{}, []string) {
	t.Helper()
	var warnings []string
	lookup := func(name string) (string, bool) { v, ok := env[name]; return v, ok }
	out, err := ConvertCompose([]byte(doc), lookup, func(m string) { warnings = append(warnings, m) })
	if err != nil {
		t.Fatal(err)
	}
	return out, warnings
}

func serverlet(t *testing.T, out map[string]interface{}, name string) map[string]interface{} {
	t.Helper()
	for _, item := range out["serverlets"].([]interface{}) {
		if s := item.(map[string]interface{}); s["name"] == name {
			return s
		}
	}
	t.Fatalf("no serverlet %q in %v", name, out)
	return nil
}

func hasWarning(warnings []string, parts ...string) bool {
	for _, w := range warnings {
		ok := true
		for _, part := range parts {
			ok = ok && strings.Contains(w, part)
		}
		if ok {
			return true
		}
	}
	return false
}

func TestIsComposeFile(t *testing.T) {
	for path, want := range map[string]bool{
		"compose.yaml": true, "compose.yml": true, "docker-compose.yaml": true, "docker-compose.yml": true,
		"/x/Docker-Compose.YML": true, "futrou.yaml": false, "compose.json": false, "docker-compose.override.yml": false,
	} {
		if got := IsComposeFile(path); got != want {
			t.Errorf("IsComposeFile(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestConvertComposeServiceBasics(t *testing.T) {
	out, warnings := convert(t, `
name: shop
services:
  web:
    image: nginx:1
    entrypoint: ["node", "server.js"]
    command: ["--port", "8080", "hello world"]
    working_dir: /app
    user: node
    environment:
      A: one
      B: 2
      C: true
    ports:
      - "80:8080"
      - "127.0.0.1:9000:9000/udp"
      - 3000
      - target: 5432
        published: 15432
        protocol: tcp
    stop_grace_period: 1m30s
    deploy:
      replicas: 3
    x-futrou:
      serverletPlanId: plan-1
      regionId: reg-1
`, nil)
	if out["project"] != "shop" {
		t.Errorf("project = %v", out["project"])
	}
	web := serverlet(t, out, "web")
	want := map[string]interface{}{
		"image": "nginx:1", "entrypoint": `node server.js --port 8080 "hello world"`, "cwd": "/app", "user": "node",
		"stopTimeout": 90.0, "minInstances": 3, "maxInstances": 3, "serverletPlanId": "plan-1", "regionId": "reg-1",
	}
	for key, value := range want {
		if !reflect.DeepEqual(web[key], value) {
			t.Errorf("%s = %#v, want %#v", key, web[key], value)
		}
	}
	if !reflect.DeepEqual(web["env"], map[string]interface{}{"A": "one", "B": "2", "C": "true"}) {
		t.Errorf("env = %#v", web["env"])
	}
	ports := web["ports"].([]interface{})
	var got []string
	for _, p := range ports {
		m := p.(map[string]interface{})
		proto, _ := m["protocol"].(string)
		got = append(got, strings.TrimSpace(strings.Join([]string{scalarString(m["port"]), proto}, "/")))
	}
	if strings.Join(got, ",") != "8080/,9000/udp,3000/,5432/" {
		t.Errorf("ports = %v", got)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
}

func TestConvertComposeEntrypointAndCommandAlone(t *testing.T) {
	out, _ := convert(t, "services:\n  a:\n    image: x\n    command: run it\n  b:\n    image: x\n    entrypoint: /start.sh\n  c:\n    image: x\n", nil)
	if serverlet(t, out, "a")["entrypoint"] != "run it" || serverlet(t, out, "b")["entrypoint"] != "/start.sh" {
		t.Errorf("unexpected entrypoints: %v", out["serverlets"])
	}
	if _, present := serverlet(t, out, "c")["entrypoint"]; present {
		t.Error("a service without a command keeps the image's start command")
	}
}

func TestConvertComposeEnvironmentList(t *testing.T) {
	out, warnings := convert(t, "services:\n  a:\n    image: x\n    environment:\n      - PLAIN=1\n      - EMPTY=\n      - FROM_HOST\n      - MISSING\n", map[string]string{"FROM_HOST": "yes"})
	env := serverlet(t, out, "a")["env"].(map[string]interface{})
	if env["PLAIN"] != "1" || env["EMPTY"] != "" {
		t.Errorf("env = %v", env)
	}
	if _, present := env["MISSING"]; present || !hasWarning(warnings, "MISSING") {
		t.Errorf("an unset pass-through variable is skipped with a warning: %v %v", env, warnings)
	}
}

func TestConvertComposeDependsOn(t *testing.T) {
	out, warnings := convert(t, `
services:
  db: {image: x}
  cache: {image: x}
  job: {image: x}
  web:
    image: x
    depends_on:
      db: {condition: service_healthy}
      cache: {condition: service_started}
      job: {condition: service_completed_successfully}
      ghost: {condition: service_started}
  list:
    image: x
    depends_on: [db, cache]
`, nil)
	if got := serverlet(t, out, "web")["needs"]; !reflect.DeepEqual(got, []interface{}{"cache", "db:healthy", "job"}) {
		t.Errorf("needs = %#v", got)
	}
	if got := serverlet(t, out, "list")["needs"]; !reflect.DeepEqual(got, []interface{}{"db", "cache"}) {
		t.Errorf("list needs = %#v", got)
	}
	if !hasWarning(warnings, "service_completed_successfully") || !hasWarning(warnings, "ghost", "not a service") {
		t.Errorf("warnings = %v", warnings)
	}
}

func TestConvertComposeHealthcheck(t *testing.T) {
	for name, tc := range map[string]struct {
		test string
		want string
	}{
		"CMD":       {`["CMD", "curl", "-f", "http://x"]`, "curl -f http://x"},
		"CMD-SHELL": {`["CMD-SHELL", "pg_isready -U postgres"]`, "pg_isready -U postgres"},
		"string":    {`"pg_isready"`, "pg_isready"},
	} {
		t.Run(name, func(t *testing.T) {
			out, warnings := convert(t, "services:\n  a:\n    image: x\n    healthcheck:\n      test: "+tc.test+"\n      interval: 10s\n      timeout: 500ms\n      retries: 4\n      start_period: 2m\n", nil)
			a := serverlet(t, out, "a")
			if a["healthcheck"] != tc.want || a["healthcheckInterval"] != 10.0 || a["healthcheckTimeout"] != 0.5 || a["healthcheckRetries"] != 4 || a["startTimeout"] != 120.0 {
				t.Errorf("healthcheck settings = %v", a)
			}
			if len(warnings) != 1 || !hasWarning(warnings, "serverletPlanId") {
				t.Errorf("only the missing plan is reported, got %v", warnings)
			}
		})
	}
	out, _ := convert(t, "services:\n  a:\n    image: x\n    healthcheck:\n      test: [NONE]\n  b:\n    image: x\n    healthcheck:\n      disable: true\n      test: x\n", nil)
	for _, name := range []string{"a", "b"} {
		if _, present := serverlet(t, out, name)["healthcheck"]; present {
			t.Errorf("%s: a disabled healthcheck must not be converted", name)
		}
	}
	_, warnings := convert(t, "services:\n  a:\n    image: x\n    healthcheck:\n      test: x\n      interval: soon\n      start_interval: 5s\n      retries: many\n", nil)
	if !hasWarning(warnings, "interval", "soon") || !hasWarning(warnings, "start_interval") || !hasWarning(warnings, "retries") {
		t.Errorf("warnings = %v", warnings)
	}
}

func TestConvertComposeNetworksAndVolumes(t *testing.T) {
	out, warnings := convert(t, `
services:
  web:
    image: x
    networks: [default, back]
    volumes:
      - data:/data
      - type: volume
        source: data
        target: /more
      - ./src:/src
      - /abs:/abs
      - /anonymous
      - type: tmpfs
        target: /tmp
      - undeclared:/u
networks:
  back:
    internal: true
    driver: overlay
    ipam:
      config:
        - subnet: 10.1.0.0/24
        - subnet: fd00::/64
  outside:
    external: true
volumes:
  data:
    x-futrou: {storagePlanId: sp-1}
  other: {}
  shared:
    external: true
`, nil)
	web := serverlet(t, out, "web")
	if !reflect.DeepEqual(web["networks"], []interface{}{"back"}) {
		t.Errorf("networks = %#v (the implicit default network is dropped)", web["networks"])
	}
	if !reflect.DeepEqual(web["mounts"], map[string]interface{}{"/data": "data", "/more": "data"}) {
		t.Errorf("mounts = %#v", web["mounts"])
	}
	networks := out["networks"].([]interface{})
	if len(networks) != 1 {
		t.Fatalf("networks = %#v (external networks are not created)", networks)
	}
	back := networks[0].(map[string]interface{})
	if back["name"] != "back" || back["internal"] != true || back["ipv4"] != "10.1.0.0/24" || back["ipv6"] != "fd00::/64" {
		t.Errorf("network = %#v", back)
	}
	storages := out["storages"].([]interface{})
	if len(storages) != 2 || storages[0].(map[string]interface{})["storagePlanId"] != "sp-1" {
		t.Errorf("storages = %#v", storages)
	}
	for _, want := range [][]string{
		{"bind mount", "./src"}, {"bind mount", "/abs"}, {"anonymous volume"}, {"tmpfs"}, {"undeclared", "not a declared volume"},
		{"driver", "overlay"}, {"networks.outside", "external"}, {"volumes.shared", "external"}, {"volumes.other", "storagePlanId"},
	} {
		if !hasWarning(warnings, want...) {
			t.Errorf("missing warning %v in %v", want, warnings)
		}
	}
}

func TestConvertComposeWarnsAboutUnsupportedOptions(t *testing.T) {
	_, warnings := convert(t, `
version: "3.9"
name: x
services:
  web:
    image: nginx
    restart: always
    container_name: web
    labels: {a: b}
    env_file: .env
    network_mode: host
    deploy:
      replicas: 1
      resources: {limits: {cpus: "1"}}
    x-custom: ignored
  builder:
    build: .
secrets: {s: {file: ./s}}
configs: {c: {file: ./c}}
x-anything: ignored
`, nil)
	for _, want := range [][]string{
		{"services.web.restart"}, {"services.web.container_name"}, {"services.web.labels"}, {"services.web.env_file"},
		{"network_mode"}, {"deploy.resources"}, {"services.builder", "build"}, {"secrets"}, {"configs"}, {"serverletPlanId"},
	} {
		if !hasWarning(warnings, want...) {
			t.Errorf("missing warning %v in:\n%s", want, strings.Join(warnings, "\n"))
		}
	}
	for _, unwanted := range []string{"version", "x-custom", "x-anything", "services.web.image", "services.web.deploy.replicas"} {
		if hasWarning(warnings, unwanted) {
			t.Errorf("%q must not be reported: %v", unwanted, warnings)
		}
	}
}

func TestConvertComposeInterpolation(t *testing.T) {
	out, warnings := convert(t, `
services:
  web:
    image: "${REGISTRY:-docker.io}/web:${TAG}"
    environment:
      A: ${SET}
      B: ${EMPTY:-fallback}
      C: ${UNSET-dash}
      D: $$literal
      E: $BARE
      F: ${MISSING}
`, map[string]string{"TAG": "v1", "SET": "set", "EMPTY": "", "BARE": "bare"})
	web := serverlet(t, out, "web")
	if web["image"] != "docker.io/web:v1" {
		t.Errorf("image = %v", web["image"])
	}
	want := map[string]interface{}{"A": "set", "B": "fallback", "C": "dash", "D": "$literal", "E": "bare", "F": ""}
	if !reflect.DeepEqual(web["env"], map[string]interface{}{"A": "set", "B": "fallback", "C": "dash", "D": "$literal", "E": "bare", "F": ""}) {
		t.Errorf("env = %#v, want %#v", web["env"], want)
	}
	if !hasWarning(warnings, "MISSING") {
		t.Errorf("an unset variable is reported: %v", warnings)
	}
}

func TestConvertComposeXFutrouTopLevelAndMerge(t *testing.T) {
	out, _ := convert(t, "x-futrou:\n  workspace: acme\n  project: override\nname: shop\nservices:\n  a:\n    image: x\n    x-futrou: {serverletPlanId: p}\n", nil)
	if out["workspace"] != "acme" || out["project"] != "override" {
		t.Errorf("x-futrou must be merged over the converted values: %v", out)
	}
}

func TestConvertComposeErrors(t *testing.T) {
	for name, doc := range map[string]string{
		"empty":      "",
		"syntax":     "services: [unclosed",
		"multi docs": "services: {}\n---\nservices: {}\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ConvertCompose([]byte(doc), noEnv, func(string) {}); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
	_, warnings := convert(t, "name: x\n", nil)
	if !hasWarning(warnings, "no services") {
		t.Errorf("a file without services is reported: %v", warnings)
	}
}

func TestConvertComposeYAMLAnchorsAndMergeKeys(t *testing.T) {
	out, _ := convert(t, "x-base: &base\n  image: nginx\n  environment: {A: b}\nservices:\n  one:\n    <<: *base\n  two:\n    <<: *base\n    image: other\n", nil)
	if serverlet(t, out, "one")["image"] != "nginx" || serverlet(t, out, "two")["image"] != "other" {
		t.Errorf("anchors and merge keys must work: %v", out["serverlets"])
	}
}

func TestLoadComposeFileEndToEnd(t *testing.T) {
	for _, name := range []string{"docker-compose.yml", "docker-compose.yaml", "compose.yaml", "compose.yml"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, name, "name: shop\nservices:\n  db:\n    image: postgres\n  web:\n    image: nginx\n    depends_on: [db]\n    restart: always\n")
			var warnings []string
			previous := composeWarn
			composeWarn = func(m string) { warnings = append(warnings, m) }
			defer func() { composeWarn = previous }()

			cfg, path, err := LoadConfig(dir, "")
			if err != nil {
				t.Fatal(err)
			}
			if filepath.Base(path) != name || cfg.Project != "shop" || len(cfg.Serverlets) != 2 {
				t.Fatalf("unexpected config %#v from %s", cfg, path)
			}
			if cfg.Serverlets[1].Needs[0] != "db" {
				t.Errorf("needs = %v", cfg.Serverlets[1].Needs)
			}
			if !hasWarning(warnings, name, "restart") {
				t.Errorf("warnings must name the file and the option: %v", warnings)
			}
		})
	}
}

func TestComposeIsTheLastDiscoveryChoice(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "docker-compose.yml", "services:\n  a:\n    image: x\n")
	writeFile(t, dir, "compose.yaml", "services:\n  b:\n    image: x\n")
	path, err := findConfig(dir, "")
	if err != nil || filepath.Base(path) != "compose.yaml" {
		t.Fatalf("compose.yaml is preferred over docker-compose.yml: %s, %v", path, err)
	}
	writeFile(t, dir, "futrou.yml", "project: native\n")
	cfg, path, err := LoadConfig(dir, "")
	if err != nil || filepath.Base(path) != "futrou.yml" || cfg.Project != "native" {
		t.Fatalf("a futrou config beats Docker Compose: %s, %v", path, err)
	}
}

func TestComposeFilesAreNeverSaved(t *testing.T) {
	if CanSave("docker-compose.yml") || CanSave("compose.yaml") || !CanSave("futrou.yaml") {
		t.Error("CanSave must refuse Compose files only")
	}
	dir := t.TempDir()
	path := writeFile(t, dir, "docker-compose.yml", "services: {}\n")
	if _, err := sampleConfig(t).Save(dir, "docker-compose.yml"); err == nil {
		t.Fatal("saving to a Compose file must fail")
	}
	if data, _ := os.ReadFile(path); string(data) != "services: {}\n" {
		t.Fatalf("the Compose file must not change: %q", data)
	}
}

func TestComposeExampleConvertsWithoutWarnings(t *testing.T) {
	var warnings []string
	previous := composeWarn
	composeWarn = func(m string) { warnings = append(warnings, m) }
	defer func() { composeWarn = previous }()

	cfg, _, err := LoadConfig("../../examples", "docker-compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Errorf("the example uses only supported options, got: %v", warnings)
	}
	if cfg.Project != "website" || cfg.Workspace != "acme" || cfg.Region != "<region-name-or-id>" || len(cfg.Serverlets) != 2 || len(cfg.Networks) != 1 || len(cfg.Storages) != 1 {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	web := cfg.Serverlets[1]
	if web.Name != "web" || web.Entrypoint != "node server.js" || web.Needs[0] != "db:healthy" || web.MinInstances != 2 || web.StartTimeout != 60 || web.Mounts["/data"] != "uploads" {
		t.Fatalf("unexpected web serverlet: %#v", web)
	}
	if db := cfg.Serverlets[0]; db.Env["POSTGRES_PASSWORD"] != "change-me" {
		t.Fatalf("variables must be expanded: %#v", db.Env)
	}
}
