package config

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

// composeFixtures are the Compose files the tests convert, by name.
var composeFixtures = map[string]string{
	"wordpress.yml":   composeWordPress,
	"web-stack.yaml":  composeWebStack,
	"monitoring.yml":  composeMonitoring,
	"minimal.yml":     composeMinimal,
	"dns-and-udp.yml": composeDNSAndUDP,
}

// convertFile converts the named fixture with the given environment.
func convertFile(t *testing.T, name string, env map[string]string) (map[string]interface{}, []string) {
	t.Helper()
	doc, ok := composeFixtures[name]
	if !ok {
		t.Fatalf("unknown fixture %q", name)
	}
	return convert(t, doc, env)
}

func serverletNames(out map[string]interface{}) []string {
	var names []string
	for _, item := range out["serverlets"].([]interface{}) {
		names = append(names, item.(map[string]interface{})["name"].(string))
	}
	sort.Strings(names)
	return names
}

func portList(s map[string]interface{}) string {
	var ports []string
	for _, item := range s["ports"].([]interface{}) {
		p := item.(map[string]interface{})
		text := scalarString(p["port"])
		if proto, ok := p["protocol"].(string); ok {
			text += "/" + proto
		}
		ports = append(ports, text)
	}
	return strings.Join(ports, ",")
}

func names(items interface{}) []string {
	var out []string
	list, _ := items.([]interface{})
	for _, item := range list {
		out = append(out, item.(map[string]interface{})["name"].(string))
	}
	sort.Strings(out)
	return out
}

func TestConvertRealWorldWordPress(t *testing.T) {
	out, warnings := convertFile(t, "wordpress.yml", map[string]string{"MYSQL_PASSWORD": "from-env"})

	if !reflect.DeepEqual(serverletNames(out), []string{"db", "wordpress"}) {
		t.Fatalf("serverlets = %v", serverletNames(out))
	}
	db, wp := serverlet(t, out, "db"), serverlet(t, out, "wordpress")
	if db["image"] != "mysql:8.0" || db["entrypoint"] != "--default-authentication-plugin=mysql_native_password" {
		t.Errorf("db = %v", db)
	}
	if db["healthcheck"] != "mysqladmin ping -h localhost" || db["healthcheckRetries"] != 5 || db["healthcheckInterval"] != 10.0 {
		t.Errorf("db healthcheck = %v", db)
	}
	if !reflect.DeepEqual(db["mounts"], map[string]interface{}{"/var/lib/mysql": "db_data"}) {
		t.Errorf("db mounts = %v", db["mounts"])
	}
	env := wp["env"].(map[string]interface{})
	if env["WORDPRESS_DB_PASSWORD"] != "from-env" || env["WORDPRESS_DB_HOST"] != "db" {
		t.Errorf("wordpress env = %v", env)
	}
	if env := db["env"].(map[string]interface{}); env["MYSQL_ROOT_PASSWORD"] != "rootpass" || env["MYSQL_PASSWORD"] != "from-env" {
		t.Errorf("db env = %v", env)
	}
	if portList(wp) != "80" || !reflect.DeepEqual(wp["needs"], []interface{}{"db:healthy"}) || !reflect.DeepEqual(wp["networks"], []interface{}{"wp"}) {
		t.Errorf("wordpress = %v", wp)
	}
	if !reflect.DeepEqual(wp["mounts"], map[string]interface{}{"/var/www/html/wp-content": "wp_content"}) {
		t.Errorf("wordpress mounts = %v", wp["mounts"])
	}
	if !reflect.DeepEqual(names(out["storages"]), []string{"db_data", "wp_content"}) || !reflect.DeepEqual(names(out["networks"]), []string{"wp"}) {
		t.Errorf("storages = %v, networks = %v", out["storages"], out["networks"])
	}
	for _, want := range [][]string{
		{"services.db.restart"}, {"services.db.expose"}, {"services.wordpress.restart"},
		{"bind mount", "php.ini"}, {"volumes.db_data", "storagePlanId"}, {"services.db", "serverletPlanId"},
	} {
		if !hasWarning(warnings, want...) {
			t.Errorf("missing warning %v in:\n%s", want, strings.Join(warnings, "\n"))
		}
	}
	if hasWarning(warnings, "version") || hasWarning(warnings, "networks.wp") {
		t.Errorf("version and a bridge network are fine: %v", warnings)
	}
}

func TestConvertRealWorldWebStack(t *testing.T) {
	out, warnings := convertFile(t, "web-stack.yaml", nil)

	if out["project"] != "web-stack" {
		t.Errorf("project = %v", out["project"])
	}
	if !reflect.DeepEqual(serverletNames(out), []string{"api", "postgres", "redis", "web"}) {
		t.Fatalf("serverlets = %v", serverletNames(out))
	}
	api := serverlet(t, out, "api")
	checks := map[string]interface{}{
		"image": "ghcr.io/acme/api:latest", "cwd": "/srv/api", "user": "1000:1000", "entrypoint": "node dist/main.js",
		"minInstances": 3, "maxInstances": 3, "stopTimeout": 20.0,
	}
	for key, want := range checks {
		if !reflect.DeepEqual(api[key], want) {
			t.Errorf("api.%s = %#v, want %#v", key, api[key], want)
		}
	}
	if !reflect.DeepEqual(api["needs"], []interface{}{"postgres:healthy", "redis"}) || portList(api) != "3000" {
		t.Errorf("api needs/ports = %v / %s", api["needs"], portList(api))
	}
	if !reflect.DeepEqual(api["networks"], []interface{}{"backend", "frontend"}) {
		t.Errorf("api networks = %v", api["networks"])
	}
	pg := serverlet(t, out, "postgres")
	if pg["healthcheck"] != "pg_isready -U app -d app" || pg["startTimeout"] != 20.0 {
		t.Errorf("postgres = %v", pg)
	}
	if env := pg["env"].(map[string]interface{}); env["POSTGRES_DB"] != "app" || len(env) != 3 {
		t.Errorf("postgres env = %v", env)
	}
	if redis := serverlet(t, out, "redis"); redis["entrypoint"] != "redis-server --appendonly yes" || portList(redis) != "6379" {
		t.Errorf("redis = %v", redis)
	}
	web := serverlet(t, out, "web")
	if portList(web) != "80,443" || !reflect.DeepEqual(web["needs"], []interface{}{"api"}) || !reflect.DeepEqual(web["mounts"], map[string]interface{}{"/etc/nginx/certs": "certs"}) {
		t.Errorf("web = %v", web)
	}

	networks := map[string]map[string]interface{}{}
	for _, item := range out["networks"].([]interface{}) {
		n := item.(map[string]interface{})
		networks[n["name"].(string)] = n
	}
	if networks["backend"]["internal"] != true || networks["frontend"]["ipv4"] != "172.28.0.0/16" {
		t.Errorf("networks = %v", networks)
	}
	if !reflect.DeepEqual(names(out["storages"]), []string{"certs", "pgdata"}) {
		t.Errorf("storages = %v", out["storages"])
	}
	for _, want := range [][]string{
		{"services.api.build"}, {"services.api.env_file"}, {"services.api.deploy.resources"}, {"services.api.restart"},
		{"services.api.logging"}, {"bind mount", "nginx.conf"},
	} {
		if !hasWarning(warnings, want...) {
			t.Errorf("missing warning %v in:\n%s", want, strings.Join(warnings, "\n"))
		}
	}
	// api has an image next to build: the image is used, so no "no image" warning.
	if hasWarning(warnings, "services.api has no image") {
		t.Errorf("api has an image: %v", warnings)
	}
}

func TestConvertRealWorldMonitoring(t *testing.T) {
	out, warnings := convertFile(t, "monitoring.yml", nil)

	if !reflect.DeepEqual(serverletNames(out), []string{"grafana", "node-exporter", "prometheus"}) {
		t.Fatalf("serverlets = %v", serverletNames(out))
	}
	prom := serverlet(t, out, "prometheus")
	want := `--config.file=/etc/prometheus/prometheus.yml --storage.tsdb.retention.time=15d "--web.external-url=http://localhost:9090/my prom"`
	if prom["entrypoint"] != want {
		t.Errorf("prometheus entrypoint = %q, want %q", prom["entrypoint"], want)
	}
	if !reflect.DeepEqual(prom["mounts"], map[string]interface{}{"/prometheus": "prom_data"}) || portList(prom) != "9090" {
		t.Errorf("prometheus = %v", prom)
	}
	grafana := serverlet(t, out, "grafana")
	if grafana["healthcheck"] != "wget -qO- http://localhost:3000/api/health || exit 1" || grafana["healthcheckInterval"] != 90.0 || portList(grafana) != "3000" {
		t.Errorf("grafana = %v", grafana)
	}
	if !reflect.DeepEqual(grafana["needs"], []interface{}{"prometheus"}) {
		t.Errorf("grafana needs = %v", grafana["needs"])
	}
	for _, want := range [][]string{
		{"services.prometheus.container_name"}, {"services.prometheus.labels"}, {"services.prometheus.extra_hosts"},
		{"services.grafana.secrets"}, {"services.grafana.cap_add"}, {"services.grafana.ulimits"},
		{"services.node-exporter.network_mode"}, {"services.node-exporter.pid"}, {"services.node-exporter.privileged"},
		{"services.node-exporter.profiles"}, {"secrets"}, {"volumes.prom_data.driver"}, {"bind mount", "prometheus.yml"},
	} {
		if !hasWarning(warnings, want...) {
			t.Errorf("missing warning %v in:\n%s", want, strings.Join(warnings, "\n"))
		}
	}
}

func TestConvertRealWorldMinimal(t *testing.T) {
	out, warnings := convertFile(t, "minimal.yml", nil)
	hello := serverlet(t, out, "hello")
	if hello["image"] != "hello-world" || len(hello) != 2 { // name + image
		t.Errorf("hello = %v", hello)
	}
	for _, key := range []string{"networks", "storages", "project"} {
		if _, present := out[key]; present {
			t.Errorf("a minimal file must not produce %s", key)
		}
	}
	if len(warnings) != 1 || !hasWarning(warnings, "serverletPlanId") {
		t.Errorf("only the missing plan is reported: %v", warnings)
	}
}

func TestConvertRealWorldPortsAndProtocols(t *testing.T) {
	out, warnings := convertFile(t, "dns-and-udp.yml", nil)
	dns, game := serverlet(t, out, "dns"), serverlet(t, out, "game")
	if portList(dns) != "53/udp,53,9153" || dns["entrypoint"] != "-conf /Corefile" || dns["minInstances"] != 2 {
		t.Errorf("dns = %v (ports %s)", dns, portList(dns))
	}
	if portList(game) != "25565,19132/udp" {
		t.Errorf("game ports = %s", portList(game))
	}
	for _, want := range [][]string{
		{"services.dns.deploy.restart_policy"}, {"30000-30010", "port ranges"}, {"services.game.stdin_open"}, {"services.game.tty"},
	} {
		if !hasWarning(warnings, want...) {
			t.Errorf("missing warning %v in:\n%s", want, strings.Join(warnings, "\n"))
		}
	}
	if !reflect.DeepEqual(names(out["storages"]), []string{"mc"}) {
		t.Errorf("storages = %v", out["storages"])
	}
}

// TestConvertRealWorldFilesLoadAndValidate runs every fixture through the
// full loader: the converted document must pass the futrou.json validators.
func TestConvertRealWorldFilesLoadAndValidate(t *testing.T) {
	previous := composeWarn
	composeWarn = func(string) {}
	defer func() { composeWarn = previous }()

	for name, doc := range composeFixtures {
		t.Run(name, func(t *testing.T) {
			path := writeFile(t, t.TempDir(), "docker-compose.yml", doc)
			cfg := &Config{}
			if err := cfg.LoadCompose(path); err != nil {
				t.Fatalf("the converted config must validate: %v", err)
			}
			if len(cfg.Serverlets) == 0 {
				t.Fatal("no serverlets were converted")
			}
			// And it must survive the JSON and YAML serializers used by deploy and pull.
			if _, err := cfg.ToJSON(); err != nil {
				t.Fatal(err)
			}
			if _, err := cfg.ToYAML(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestConvertRealWorldIsDeterministic(t *testing.T) {
	first, _ := convertFile(t, "web-stack.yaml", nil)
	for i := 0; i < 20; i++ {
		again, _ := convertFile(t, "web-stack.yaml", nil)
		if !reflect.DeepEqual(first, again) {
			t.Fatal("the same file must always convert to the same document")
		}
	}
}

// composeWordPress is a typical wordpress Compose file.
const composeWordPress = `version: "3.8"

services:
  db:
    image: mysql:8.0
    command: '--default-authentication-plugin=mysql_native_password'
    restart: always
    volumes:
      - db_data:/var/lib/mysql
    environment:
      MYSQL_ROOT_PASSWORD: ${MYSQL_ROOT_PASSWORD:-rootpass}
      MYSQL_DATABASE: wordpress
      MYSQL_USER: wordpress
      MYSQL_PASSWORD: ${MYSQL_PASSWORD:-wppass}
    expose:
      - 3306
      - 33060
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "localhost"]
      interval: 10s
      timeout: 5s
      retries: 5
    networks:
      - wp

  wordpress:
    image: wordpress:6-apache
    restart: always
    ports:
      - "8080:80"
    environment:
      WORDPRESS_DB_HOST: db
      WORDPRESS_DB_USER: wordpress
      WORDPRESS_DB_PASSWORD: ${MYSQL_PASSWORD:-wppass}
      WORDPRESS_DB_NAME: wordpress
    volumes:
      - wp_content:/var/www/html/wp-content
      - ./php.ini:/usr/local/etc/php/conf.d/custom.ini:ro
    depends_on:
      db:
        condition: service_healthy
    networks:
      - wp

volumes:
  db_data:
  wp_content:

networks:
  wp:
    driver: bridge
`

// composeWebStack is a typical web-stack Compose file.
const composeWebStack = `name: web-stack

x-common: &common
  restart: unless-stopped
  logging:
    driver: json-file
    options:
      max-size: "10m"

services:
  postgres:
    <<: *common
    image: postgres:17-alpine
    environment:
      - POSTGRES_USER=app
      - POSTGRES_PASSWORD=secret
      - POSTGRES_DB=app
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U app -d app"]
      interval: 5s
      timeout: 3s
      retries: 10
      start_period: 20s
    networks: [backend]

  redis:
    <<: *common
    image: redis:7
    command: ["redis-server", "--appendonly", "yes"]
    ports:
      - 6379
    networks: [backend]

  api:
    <<: *common
    build:
      context: ./api
      dockerfile: Dockerfile
    image: ghcr.io/acme/api:latest
    working_dir: /srv/api
    user: "1000:1000"
    command: node dist/main.js
    environment:
      DATABASE_URL: postgres://app:secret@postgres:5432/app
      REDIS_URL: redis://redis:6379
      NODE_ENV: production
    env_file:
      - .env
    ports:
      - "127.0.0.1:3000:3000"
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_started
    deploy:
      replicas: 3
      resources:
        limits:
          cpus: "0.5"
          memory: 512M
    stop_grace_period: 20s
    networks: [backend, frontend]

  web:
    <<: *common
    image: nginx:1.27
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf:ro
      - certs:/etc/nginx/certs
    depends_on: [api]
    networks: [frontend]

volumes:
  pgdata:
  certs:

networks:
  backend:
    internal: true
  frontend:
    ipam:
      config:
        - subnet: 172.28.0.0/16
`

// composeMonitoring is a typical monitoring Compose file.
const composeMonitoring = `services:
  prometheus:
    image: prom/prometheus:v2.55.0
    container_name: prometheus
    command:
      - --config.file=/etc/prometheus/prometheus.yml
      - --storage.tsdb.retention.time=15d
      - "--web.external-url=http://localhost:9090/my prom"
    ports: ["9090:9090"]
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
      - prom_data:/prometheus
    labels:
      com.example.team: platform
    extra_hosts:
      - "host.docker.internal:host-gateway"

  grafana:
    image: grafana/grafana:11.3.0
    ports: ["3000:3000/tcp"]
    environment:
      GF_SECURITY_ADMIN_PASSWORD__FILE: /run/secrets/grafana_password
    secrets:
      - grafana_password
    depends_on: [prometheus]
    healthcheck:
      test: wget -qO- http://localhost:3000/api/health || exit 1
      interval: 1m30s
      timeout: 10s
    cap_add: [SYS_PTRACE]
    ulimits:
      nofile: {soft: 65536, hard: 65536}

  node-exporter:
    image: prom/node-exporter
    network_mode: host
    pid: host
    privileged: true
    profiles: [debug]

secrets:
  grafana_password:
    file: ./grafana_password.txt

volumes:
  prom_data:
    driver: local
`

// composeMinimal is a typical minimal Compose file.
const composeMinimal = `services:
  hello:
    image: hello-world
`

// composeDNSAndUDP is a typical dns-and-udp Compose file.
const composeDNSAndUDP = `services:
  dns:
    image: coredns/coredns:1.11.3
    ports:
      - "53:53/udp"
      - "53:53/tcp"
      - "9153:9153"
    command: -conf /Corefile
    deploy:
      replicas: 2
      restart_policy:
        condition: on-failure
  game:
    image: itzg/minecraft-server
    ports:
      - target: 25565
        published: 25565
        protocol: tcp
      - "19132:19132/udp"
      - "30000-30010:30000-30010"
    environment:
      EULA: "TRUE"
      MEMORY: 2G
    stdin_open: true
    tty: true
    volumes:
      - mc:/data
volumes:
  mc: {}
`
