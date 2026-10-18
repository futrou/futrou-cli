---
name: futrou-cli
description: Use when working with Futrou Cloud — deploying serverlets, managing proxies, DNS, storage, networks, projects, or calling the Futrou REST API directly.
---

# Futrou Cloud Skill

Futrou Cloud lets you run containerised workloads (serverlets), manage HTTP/TCP proxies, DNS zones, private networks, persistent storage, and projects — all from the CLI or REST API.

## Installation

### CLI (recommended)

```bash
# Linux / macOS
curl -fsSL https://futrou.com/install.sh | bash

# Windows (PowerShell)
irm https://futrou.com/install.ps1 | iex

# npm / npx
npm install -g futrou
npx futrou --help
```

### Authenticate

```bash
futrou login                      # opens the browser to sign in
futrou login --workspace acme     # also select the default workspace
futrou whoami                     # show who you are logged in as
futrou logout

# Or use env vars / flags instead of stored credentials
FUTROU_API_TOKEN=<token> futrou serverlet ls
futrou --api-key <token> serverlet ls
```

Credentials are stored in `~/.futrou/cli.json`. The env var `FUTROU_API_TOKEN` always takes precedence.

---

## Core concepts

| Resource | What it is |
|---|---|
| **Serverlet** | A containerised service — runs one or more instances of a Docker image |
| **Proxy** | HTTP/TCP reverse proxy with a domain, TLS, and load balancing |
| **Network** | Private network that serverlets join by name |
| **Storage** | Persistent storage mounted into a serverlet |
| **Server** | A machine of the workspace that runs serverlets; managed with `futrou server`, not part of `futrou.json` |
| **Cron** | A scheduled HTTP request or code job |
| **Region** | Where resources run; set once per project with the top-level `region` |
| **Project** | Logical grouping for serverlets, crons, and variables |
| **DNS** | DNS zone management |

---

## CLI reference

### Global flags

```bash
futrou --api-key <token>          # override stored token
futrou --api-url <url>            # override API base URL (default: https://api.futrou.com)
futrou --log-format json          # text or json (default: text); also FUTROU_LOG_FORMAT
futrou --log-level debug          # debug, info, warn, error; also FUTROU_LOG_LEVEL
```

### Auth

```bash
futrou login
futrou logout
futrou whoami
```

### Deploy (declarative)

Describe the project in `futrou.json`, `futrou.js`/`.cjs`/`.mjs`/`.ts` or `futrou.yaml`/`.yml` in your project root. The CLI uses the first one it finds, in that order; YAML is converted to the equivalent JSON and validated the same way. When the CLI updates the config (`config pull`, lock refresh after a deploy) it writes it back in the same format, JSON or YAML; JS/TS configs are code and are never rewritten. Resources are arrays; `networks` are created before the serverlets that join them. The same project in each format, with every supported option (also in `examples/`):

#### futrou.json

<!-- example:futrou.json:start -->

```json
{
  "$schema": "https://futrou.com/futrou.schema.json",
  "workspace": "acme",
  "project": "website",
  "region": "<region-name-or-id>",
  "networks": [
    {
      "name": "backend",
      "ipv4": "10.20.0.0/24",
      "ipv6": "fd00:20::/64",
      "internal": false
    }
  ],
  "storages": [
    {
      "name": "uploads",
      "storagePlanId": "<storage-plan-id>",
      "public": false
    }
  ],
  "serverlets": [
    {
      "name": "web",
      "image": "ghcr.io/acme/web:1.4.2",
      "serverletPlanId": "<serverlet-plan-id>",
      "regionId": "<region-id>",
      "env": {
        "NODE_ENV": "production",
        "PORT": "8080"
      },
      "mounts": {
        "/data": "<storage-id>"
      },
      "ports": [
        {
          "port": 8080,
          "protocol": "tcp"
        }
      ],
      "networks": [
        "backend"
      ],
      "entrypoint": "node server.js",
      "cwd": "/app",
      "user": "node",
      "needs": [
        "db:healthy"
      ],
      "healthcheck": "curl -f http://localhost:8080/health",
      "healthcheckInterval": 30,
      "healthcheckTimeout": 5,
      "healthcheckRetries": 3,
      "startTimeout": 60,
      "stopTimeout": 30,
      "minInstances": 1,
      "maxInstances": 3,
      "scaling": {
        "cpu": 70
      },
      "strategy": {
        "type": "rolling"
      }
    },
    {
      "name": "db",
      "image": "postgres:17",
      "serverletPlanId": "<serverlet-plan-id>",
      "env": {
        "POSTGRES_PASSWORD": "change-me"
      },
      "ports": [
        {
          "port": 5432
        }
      ],
      "networks": [
        "backend"
      ],
      "healthcheck": "pg_isready -U postgres",
      "healthcheckInterval": 10
    }
  ],
  "proxies": [
    {
      "domain": "www.example.com",
      "type": "http",
      "target": "web",
      "port": 8080,
      "strategy": "round-robin",
      "variant": "auto",
      "enforceHttps": true,
      "followRedirects": true,
      "preserveHeaders": true,
      "preserveHost": true,
      "preservePath": true,
      "preserveQuery": true,
      "verifyTls": true,
      "proxyPlanId": "<proxy-plan-id>"
    }
  ],
  "dns": [
    {
      "domain": "example.com",
      "name": "example.com",
      "ttl": 300,
      "priority": 0,
      "records": [
        {
          "name": "www",
          "type": "CNAME",
          "value": "proxy.futrou.com",
          "ttl": 300
        },
        {
          "name": "@",
          "type": "MX",
          "value": "mail.example.com",
          "ttl": 3600,
          "priority": 10
        }
      ]
    }
  ],
  "crons": [
    {
      "name": "cleanup",
      "type": "simple",
      "schedule": "30 14 * * *",
      "method": "POST",
      "url": "https://www.example.com/internal/cleanup",
      "headers": {
        "Authorization": "Bearer <token>"
      },
      "body": "{\"dryRun\":false}",
      "enabled": true,
      "cronPlanId": "<cron-plan-id>"
    }
  ]
}
```

<!-- example:futrou.json:end -->

#### futrou.yaml

<!-- example:futrou.yaml:start -->

```yaml
# yaml-language-server: $schema=https://futrou.com/futrou.schema.json
#
# futrou.yaml (or futrou.yml) is converted to the equivalent futrou.json and
# then validated and deployed exactly like it. Every option is shown below.
# Config files are looked up in this order: futrou.json, futrou.js, futrou.cjs,
# futrou.mjs, futrou.ts, futrou.yaml, futrou.yml.

$schema: https://futrou.com/futrou.schema.json
workspace: acme # workspace name or ID
project: website # project name or ID; the deploy target
region: <region-name-or-id> # default region (name or ID) of every resource below

networks:
  - name: backend
    ipv4: 10.20.0.0/24
    ipv6: fd00:20::/64
    internal: false # true = no outbound access

storages:
  - name: uploads
    storagePlanId: <storage-plan-id>
    public: false

serverlets:
  - name: web
    image: ghcr.io/acme/web:1.4.2
    serverletPlanId: <serverlet-plan-id>
    regionId: <region-id> # overrides the project region for this serverlet only
    env: # values must be strings, so quote numbers
      NODE_ENV: production
      PORT: "8080"
    mounts: # mount path -> storage ID
      /data: <storage-id>
    ports:
      - port: 8080 # 1-65535
        protocol: tcp # tcp, udp or http
    networks: [backend] # networks the serverlet joins
    entrypoint: node server.js # replaces the image's start command; an array is joined with spaces
    cwd: /app
    user: node
    needs: # serverlets to wait for: "name", "name:started" or "name:healthy"
      - db:healthy
    healthcheck: curl -f http://localhost:8080/health
    healthcheckInterval: 30 # seconds
    healthcheckTimeout: 5 # seconds
    healthcheckRetries: 3
    startTimeout: 60 # seconds
    stopTimeout: 30 # seconds
    minInstances: 1
    maxInstances: 3
    scaling:
      cpu: 70
    strategy:
      type: rolling

  - name: db
    image: postgres:17
    serverletPlanId: <serverlet-plan-id>
    env:
      POSTGRES_PASSWORD: change-me
    healthcheck: pg_isready -U postgres
    healthcheckInterval: 10
    ports:
      - port: 5432
    networks: [backend]

proxies:
  - domain: www.example.com
    type: http # http, tcp or udp
    target: web
    port: 8080
    strategy: round-robin # round-robin or primary-failover
    variant: auto # auto, www or non-www
    enforceHttps: true
    followRedirects: true
    preserveHeaders: true
    preserveHost: true
    preservePath: true
    preserveQuery: true
    verifyTls: true
    proxyPlanId: <proxy-plan-id>

dns:
  - domain: example.com
    name: example.com
    ttl: 300
    priority: 0
    records:
      - name: www
        type: CNAME
        value: proxy.futrou.com
        ttl: 300
      - name: "@"
        type: MX
        value: mail.example.com
        ttl: 3600
        priority: 10

crons:
  - name: cleanup
    type: simple # simple (HTTP request) or code
    schedule: "30 14 * * *" # cron expression
    method: POST
    url: https://www.example.com/internal/cleanup
    headers:
      Authorization: Bearer <token>
    body: '{"dryRun":false}'
    enabled: true
    cronPlanId: <cron-plan-id>
```

<!-- example:futrou.yaml:end -->

#### futrou.mjs

<!-- example:futrou.mjs:start -->

```js
// futrou.mjs — an ES module. The default export may be an object, or a
// function (sync or async) returning one. Because it is real code you can
// compute values, for example from environment variables.
const region = process.env.FUTROU_REGION ?? "<region-name-or-id>";

export default {
  "$schema": "https://futrou.com/futrou.schema.json",
  "workspace": "acme",
  "project": "website",
  "region": region,
  "networks": [
    {
      "name": "backend",
      "ipv4": "10.20.0.0/24",
      "ipv6": "fd00:20::/64",
      "internal": false
    }
  ],
  "storages": [
    {
      "name": "uploads",
      "storagePlanId": "<storage-plan-id>",
      "public": false
    }
  ],
  "serverlets": [
    {
      "name": "web",
      "image": "ghcr.io/acme/web:1.4.2",
      "serverletPlanId": "<serverlet-plan-id>",
      "regionId": "<region-id>",
      "env": {
        "NODE_ENV": "production",
        "PORT": "8080"
      },
      "mounts": {
        "/data": "<storage-id>"
      },
      "ports": [
        {
          "port": 8080,
          "protocol": "tcp"
        }
      ],
      "networks": [
        "backend"
      ],
      "entrypoint": "node server.js",
      "cwd": "/app",
      "user": "node",
      "needs": [
        "db:healthy"
      ],
      "healthcheck": "curl -f http://localhost:8080/health",
      "healthcheckInterval": 30,
      "healthcheckTimeout": 5,
      "healthcheckRetries": 3,
      "startTimeout": 60,
      "stopTimeout": 30,
      "minInstances": 1,
      "maxInstances": 3,
      "scaling": {
        "cpu": 70
      },
      "strategy": {
        "type": "rolling"
      }
    },
    {
      "name": "db",
      "image": "postgres:17",
      "serverletPlanId": "<serverlet-plan-id>",
      "env": {
        "POSTGRES_PASSWORD": "change-me"
      },
      "ports": [
        {
          "port": 5432
        }
      ],
      "networks": [
        "backend"
      ],
      "healthcheck": "pg_isready -U postgres",
      "healthcheckInterval": 10
    }
  ],
  "proxies": [
    {
      "domain": "www.example.com",
      "type": "http",
      "target": "web",
      "port": 8080,
      "strategy": "round-robin",
      "variant": "auto",
      "enforceHttps": true,
      "followRedirects": true,
      "preserveHeaders": true,
      "preserveHost": true,
      "preservePath": true,
      "preserveQuery": true,
      "verifyTls": true,
      "proxyPlanId": "<proxy-plan-id>"
    }
  ],
  "dns": [
    {
      "domain": "example.com",
      "name": "example.com",
      "ttl": 300,
      "priority": 0,
      "records": [
        {
          "name": "www",
          "type": "CNAME",
          "value": "proxy.futrou.com",
          "ttl": 300
        },
        {
          "name": "@",
          "type": "MX",
          "value": "mail.example.com",
          "ttl": 3600,
          "priority": 10
        }
      ]
    }
  ],
  "crons": [
    {
      "name": "cleanup",
      "type": "simple",
      "schedule": "30 14 * * *",
      "method": "POST",
      "url": "https://www.example.com/internal/cleanup",
      "headers": {
        "Authorization": "Bearer <token>"
      },
      "body": "{\"dryRun\":false}",
      "enabled": true,
      "cronPlanId": "<cron-plan-id>"
    }
  ]
};
```

<!-- example:futrou.mjs:end -->

Then deploy. `deploy` validates `futrou.json` first, then works like Terraform: it prints a plan (create / update / delete), asks `Apply these changes? [y/N]` and only applies on `y` or `yes` (Enter cancels).

```bash
futrou init config                # creates futrou.json (or futrou.yaml with -f futrou.yaml)
futrou init mcp                   # adds the Futrou MCP server to .mcp.json and .cursor/mcp.json
futrou init skills                # installs the Futrou agent skill
futrou deploy                     # plan, confirm, apply (alias: futrou up)
futrou deploy --dry-run           # show the plan only
futrou deploy -y                  # apply without asking (also automatic when CI=true)
futrou deploy --destroy           # delete only the resources declared in futrou.json
futrou deploy -f custom.json      # use a specific config file
futrou config                     # print the loaded configuration
futrou config pull --project demo # write the cloud project to futrou.json
```

Tear a project down with `futrou down` (alias `futrou teardown`). It plans the deletion of every cron, proxy, serverlet, storage, network and DNS zone in the project, then the project itself, and asks `[y/N]` first. It accepts the same `-y`, `--dry-run` and `-f` options and also skips the prompt when `CI=true`.

```bash
futrou down --dry-run             # show what would be deleted
futrou down -y                    # delete everything and the project
```

Top-level `region` (a region name or ID) is the default region of every network, serverlet, proxy, storage and cron in the project; a resource that sets its own `regionId` keeps it. An unknown region fails the deploy before anything is changed.

Serverlet fields: `name`, `image`, `serverletPlanId`, `regionId`, `env`, `ports`, `mounts` (mount path to storage ID), `networks`, `entrypoint` (one string, for example `node server.js`; an array is accepted and joined with spaces), `cwd`, `user`, `needs` (serverlets to wait for: `db`, `db:started` or `db:healthy`; they must be declared, and cycles are rejected), `healthcheck*`, `startTimeout`, `stopTimeout`, `minInstances`, `maxInstances`, `scaling`, `strategy`.

Commands use singular names with `ls`/`rm`. The older plural names (`serverlets`, `proxies`, ...) and `list`/`delete` still work as hidden aliases.

### Serverlets

```bash
futrou serverlet ls
futrou serverlet show <id>                     # also: get, inspect
futrou serverlet create --name my-app --image nginx:latest --plan <plan-id>
futrou serverlet update <id> --image nginx:1.25
futrou serverlet rm <id>
futrou serverlet start <id>
futrou serverlet stop <id>
futrou serverlet restart <id>
futrou serverlet wake <id>                     # wake a sleeping serverlet
futrou serverlet logs <id>
futrou serverlet metrics <id>
futrou serverlet instance <id>                 # same as: instance ls <id>
futrou serverlet instance ls <id>
futrou serverlet instance start <id> <instance-id>
futrou serverlet instance stop <id> <instance-id>
futrou serverlet instance restart <id> <instance-id>
futrou serverlet instance logs <id> <instance-id>
futrou serverlet instance metrics <id> <instance-id>
futrou serverlet shell <id>
futrou serverlet exec <id> -- ls -la
```

### Logs

```bash
futrou logs web                          # serverlet logs, by name or ID; streams live by default
futrou logs --live=false web             # print once and exit
futrou logs --tail -t proxy app.example.com  # only the most recent entries
futrou logs --instance <instance-id> web # one serverlet instance
futrou logs -t proxy app.example.com     # also: dns, storage, server, monitor
futrou logs --ls -t proxy                # list what can be browsed
futrou logs --limit 100 --search error --start-at 2026-01-01T00:00:00Z web
```

Every resource also has its own `logs` command with the same flags, for example `futrou proxy logs <id>`, `futrou dns logs --tail <id>`, `futrou storage logs --live=false <id>` and `futrou serverlet logs <id>`. `--tail` uses the API's recent-logs endpoint where there is one (proxy, dns, storage, monitor); otherwise it keeps the last 100 entries. `--live` (default true) polls every 2 seconds and prints only new entries; pass `--live=false` to print once and exit. `futrou proxy logs tail <id>` and `futrou dns logs tail <id>` print the recent entries once.

### Networks

```bash
futrou network ls
futrou network show <id>
futrou network create --name backend [--ipv4 <cidr>] [--ipv6 <cidr>] [--internal] [--region <id>]
futrou network update <id> --name new-name
futrou network rm <id>
futrou network devices <id>
futrou network metrics <id>
```

Attach a serverlet to networks in `futrou.json` with `"networks": ["backend"]`.

### Proxies

```bash
futrou proxy ls
futrou proxy show <id>
futrou proxy create --domain app.example.com --target my-app --port 8080 --type http --https
futrou proxy update <id> --target new-target --port 9000
futrou proxy rm <id>
futrou proxy purge <id>
futrou proxy metrics <id>
futrou proxy logs <id>
futrou proxy logs tail <id>
```

Proxy types: `http`, `tcp`, `udp`

### Storage

```bash
futrou storage ls
futrou storage show <id>
futrou storage create --name data --plan <storage-plan-id>
futrou storage update <id> --plan <storage-plan-id>
futrou storage rm <id>
futrou storage instance ls <id>
futrou storage metrics <id>
futrou storage logs <id>
futrou storage plan ls
futrou storage plan show <id>
futrou storage file ls <id> 'images/*'
futrou storage file info <id> images/logo.png
futrou storage file upload <id> ./local-dir remote/
futrou storage file download <id> remote/file.txt .
futrou storage file mkdir <id> remote/new-folder
futrou storage file move <id> remote/old.txt remote/new.txt
futrou storage file rm <id> old/
futrou storage sync <id> remote/ ./local-dir --dry-run
```

### DNS

```bash
futrou dns ls
futrou dns show <id>
futrou dns create --name example.com
futrou dns update <id> --name example.org
futrou dns rm <id>
futrou dns metrics <id>
futrou dns logs <id>
futrou dns logs tail <id>
futrou dns record ls <zone-id>
futrou dns record show <zone-id> <record-id>
futrou dns record create <zone-id> --name www --type A --value 203.0.113.10 --ttl 300
futrou dns record update <zone-id> <record-id> --value 203.0.113.11
futrou dns record rm <zone-id> <record-id>
```

### Projects

```bash
futrou project ls
futrou project show <id>
futrou project create --name my-project
futrou project update <id> --display-name "My Project"
futrou project rm <id>
```

### Workspaces

```bash
futrou workspace ls
futrou workspace show <id>
futrou workspace update <id> --display-name "Acme"
futrou workspace rm <id>
futrou workspace limit
futrou workspace contact show
futrou workspace contact update --company Acme --email billing@acme.example
futrou workspace user ls
futrou workspace user show <workspace-user-id>
futrou workspace user add --user-id <user-id> --role developer
futrou workspace user update <workspace-user-id> --role administrator
futrou workspace user rm <workspace-user-id>
```

### Servers

Servers belong to the workspace and are not part of `futrou.json`.

```bash
futrou server ls
futrou server show <id>
futrou server create --name edge-1 --region <region-id>
futrou server update <id> --name edge-2
futrou server rm <id>
futrou server reset-join <id>
futrou server leave <id>
futrou server logs <id>
futrou server metrics <id>
```

### Other

```bash
futrou upgrade                    # upgrade CLI to latest
futrou upgrade 2.0.5              # upgrade/downgrade to specific version
futrou schema                     # print the OpenAPI schema for https://api.futrou.com
futrou config init                # create a futrou.json with the default project name
futrou config schema              # print the JSON Schema of futrou.json
futrou version
futrou license
```

### Full command reference

<!-- cli-reference:start -->

Generated from the command definitions by `make build-docs`; do not edit by hand.

Global options: `--api-url <string>`, `--api-token <string>` (alias `--api-key`), `--log-level <string>`, `--local`, `--dev`, `--debug`.

- `futrou deploy` (alias: `up`) — Deploy or upgrade resources based on futrou.json|yaml config
  - `--file, -f <string>` Config file (default: auto-detect futrou.json|yaml etc.)
  - `--yes, -y` Auto-approve without prompting
  - `--dry-run` Show the planned changes and exit without applying them
  - `--destroy` Destroy the resources declared in the project configuration

- `futrou down` — Deprovision every resource in the project and delete the project
  - `--file, -f <string>` Config file (default: auto-detect futrou.json|yaml etc.)
  - `--yes, -y` Auto-approve without prompting
  - `--dry-run` Show what would be deleted and exit without deleting anything

- `futrou login` — Log in to Futrou Cloud on this machine
  - `--workspace <string>` Workspace name or ID (defaults to futrou.json|yaml, then the workspace selected at login)

- `futrou logout` — Log out from Futrou Cloud on this machine

- `futrou whoami` — Display the currently authenticated user or api token info

- `futrou config` — Display the loaded Futrou project configuration as JSON
  - `--file, -f <string>` Config file path (default: auto-detect)
- `futrou config init` — Create a new Futrou project config with the default project name
  - `--file, -f <string>` Output config file path (default futrou.json)
  - `--project <string>` Project name (default: current directory name)
- `futrou config pull` — Fetch a cloud project and write it as a Futrou project config
  - `--project, -p <string>` Cloud project name or ID (defaults to local config or default)
  - `--file, -f <string>` Output config file path (default futrou.json)
  - `--force` Overwrite an existing output file
- `futrou config schema` — Print the Futrou project config JSON Schema

- `futrou init` — Set up Futrou in this project (config, mcp, skills)
  - `--file, -f <string>` Output file path; .yaml and .yml files are written as YAML (default futrou.json)
  - `--name <string>` Serverlet name
  - `--image <string>` Container image
- `futrou init mcp` — Configure Futrou MCP for AI coding agents
- `futrou init skills` — Configure Futrou Skills for AI coding agents
- `futrou init config` — Configure a futrou.json|yaml config file for this project
  - `--file, -f <string>` Output file path; .yaml and .yml files are written as YAML (default futrou.json)
  - `--name <string>` Serverlet name
  - `--image <string>` Container image

- `futrou workspace ls` — List all workspaces
- `futrou workspace show <id>` — Show a workspace by ID
- `futrou workspace update <id>` — Update a workspace
  - `--name <string>` Workspace name
  - `--display-name <string>` Display name
- `futrou workspace rm <id>` — Delete a workspace
- `futrou workspace limit` — Get resource limits for a workspace
  - `--workspace <string>` Workspace name or ID (defaults to futrou.json|yaml, then the workspace selected at login)
- `futrou workspace contact show` — Show the workspace billing contact
  - `--workspace <string>` Workspace name or ID (defaults to futrou.json|yaml, then the workspace selected at login)
- `futrou workspace contact update` — Update the workspace billing contact
  - `--workspace <string>` Workspace name or ID (defaults to futrou.json|yaml, then the workspace selected at login)
  - `--firstname <string>` First name
  - `--lastname <string>` Last name
  - `--company <string>` Company name
  - `--email <string>` Contact email
  - `--phone <string>` Phone number
  - `--street-address <string>` Street address
  - `--city <string>` City
  - `--postal-code <string>` Postal code
  - `--country <string>` Country
- `futrou workspace user ls` — List users in a workspace
  - `--workspace <string>` Workspace name or ID (defaults to futrou.json|yaml, then the workspace selected at login)
- `futrou workspace user show <workspace-user-id>` — Show a user in a workspace
  - `--workspace <string>` Workspace name or ID (defaults to futrou.json|yaml, then the workspace selected at login)
- `futrou workspace user add` — Add a user to a workspace
  - `--workspace <string>` Workspace name or ID (defaults to futrou.json|yaml, then the workspace selected at login)
  - `--user-id <string>` User ID to add (required)
  - `--role <string>` Role: none, viewer, developer, billing_manager, administrator, owner (default viewer)
- `futrou workspace user update <workspace-user-id>` — Update a user's role in a workspace
  - `--workspace <string>` Workspace name or ID (defaults to futrou.json|yaml, then the workspace selected at login)
  - `--role <string>` Role: none, viewer, developer, billing_manager, administrator, owner (required)
- `futrou workspace user rm <workspace-user-id>` — Remove a user from a workspace
  - `--workspace <string>` Workspace name or ID (defaults to futrou.json|yaml, then the workspace selected at login)

- `futrou project ls` — List all projects
- `futrou project show <id>` — Show a project by ID
- `futrou project create` — Create a project
  - `--name <string>` Project name (slug) (required)
  - `--display-name <string>` Display name
  - `--workspace <string>` Workspace name or ID (defaults to futrou.json|yaml, then the workspace selected at login)
- `futrou project update <id>` — Update a project
  - `--name <string>` Project name
  - `--display-name <string>` Display name
- `futrou project rm <id>` — Delete a project

- `futrou server ls` — List servers
  - `--region <string>` Only list servers in this region ID
- `futrou server show <id>` — Show a server by ID
- `futrou server create` — Create a server in the workspace
  - `--name <string>` Server name (required)
  - `--region <string>` Region ID
  - `--workspace <string>` Workspace name or ID (defaults to futrou.json|yaml, then the workspace selected at login)
- `futrou server update <id>` — Update a server
  - `--name <string>` New name
  - `--ipv4 <string>` IPv4 address
  - `--ipv6 <string>` IPv6 address
  - `--api-port <string>` Server API port
- `futrou server rm <id>` — Delete a server
- `futrou server reset-join <id>` — Reset a server's join token
- `futrou server leave <id>` — Make a server leave Futrou Cloud
- `futrou server metrics <id>` — Show server metrics
- `futrou server logs <id>` — View server logs
  - `--type <string>` Only logs of this resource type (e.g. serverlet, storage)
  - `--id <string>` Only logs of the resource with this ID
  - `--instance-id <string>` Only logs of the instance with this ID
  - `--offset <int>` Offset into the log stream
  - `--limit <int>` Maximum number of log entries to return
  - `--search <string>` Filter logs by search term
  - `--start-at <string>` Only return logs at or after this time
  - `--end-at <string>` Only return logs at or before this time
  - `--tail` Show only the most recent log entries
  - `--live` Keep streaming new log entries; use --live=false to print once and exit (default true)

- `futrou serverlet ls` — List all serverlets
- `futrou serverlet show <id>` — Show a serverlet by ID
- `futrou serverlet create` — Create a new serverlet (defaults to futrou.json|yaml, then the login workspace's "default" project)
  - `--name <string>` Serverlet name (required)
  - `--image <string>` Container image (required)
  - `--plan <string>` Serverlet plan ID (required)
  - `--workspace <string>` Workspace name or ID (defaults to futrou.json|yaml, then the workspace selected at login)
  - `--project <string>` Project name or ID within the workspace (defaults to futrou.json|yaml, then "default")
- `futrou serverlet update <id>` — Update a serverlet
  - `--name <string>` New name
  - `--image <string>` New image
  - `--plan <string>` New serverlet plan ID
- `futrou serverlet rm <id>` — Delete a serverlet
- `futrou serverlet start <id>` — Start a serverlet
- `futrou serverlet stop <id>` — Stop a serverlet
- `futrou serverlet restart <id>` — Restart a serverlet
- `futrou serverlet wake <id>` — Wake a sleeping serverlet
- `futrou serverlet logs <id>` — View serverlet logs
  - `--offset <int>` Offset into the log stream
  - `--limit <int>` Maximum number of log entries to return
  - `--search <string>` Filter logs by search term
  - `--start-at <string>` Only return logs at or after this time
  - `--end-at <string>` Only return logs at or before this time
  - `--tail` Show only the most recent log entries
  - `--live` Keep streaming new log entries; use --live=false to print once and exit (default true)
- `futrou serverlet metrics <id>` — Show serverlet metrics
- `futrou serverlet instance <id>` — Manage serverlet instances
- `futrou serverlet instance ls <id>` — List serverlet instances
- `futrou serverlet instance start <id> <instance-id>` — Start a serverlet instance
- `futrou serverlet instance stop <id> <instance-id>` — Stop a serverlet instance
- `futrou serverlet instance restart <id> <instance-id>` — Restart a serverlet instance
- `futrou serverlet instance logs <id> <instance-id>` — View logs of a serverlet instance
  - `--offset <int>` Offset into the log stream
  - `--limit <int>` Maximum number of log entries to return
  - `--search <string>` Filter logs by search term
  - `--start-at <string>` Only return logs at or after this time
  - `--end-at <string>` Only return logs at or before this time
  - `--tail` Show only the most recent log entries
  - `--live` Keep streaming new log entries; use --live=false to print once and exit (default true)
- `futrou serverlet instance metrics <id> <instance-id>` — Show metrics of a serverlet instance
- `futrou serverlet shell <id>` — Open an interactive shell in a serverlet
  - `--instance <string>` Connect to a specific instance ID instead of a random one
- `futrou serverlet exec <id> -- <command>` — Run a command in a serverlet and wait for it to exit
  - `--instance <string>` Run in a specific instance ID instead of a random one

- `futrou network ls` — List all networks
- `futrou network show <id>` — Show a network by ID
- `futrou network create` — Create a network (defaults to futrou.json|yaml, then the login workspace's "default" project)
  - `--name <string>` Network name (required)
  - `--ipv4 <string>` IPv4 CIDR
  - `--ipv6 <string>` IPv6 CIDR
  - `--internal` Internal-only network (no outbound access)
  - `--region <string>` Region ID
  - `--workspace <string>` Workspace name or ID (defaults to futrou.json|yaml, then the workspace selected at login)
  - `--project <string>` Project name or ID within the workspace (defaults to futrou.json|yaml, then "default")
- `futrou network update <id>` — Update a network
  - `--name <string>` New name
  - `--ipv4 <string>` IPv4 CIDR
  - `--ipv6 <string>` IPv6 CIDR
  - `--internal` Internal-only network
- `futrou network rm <id>` — Delete a network
- `futrou network devices <id>` — List devices attached to a network
- `futrou network metrics <id>` — Show network metrics

- `futrou proxy ls` — List all proxies
- `futrou proxy show <id>` — Show a proxy by ID
- `futrou proxy create` — Create a new proxy
  - `--domain <string>` Domain name (required)
  - `--target <string>` Target host (required)
  - `--port <int>` Target port (default 80)
  - `--type <string>` Proxy type (http, tcp, udp) (default http)
  - `--strategy <string>` Load balancing strategy
  - `--https` Enforce HTTPS
- `futrou proxy update <id>` — Update a proxy
  - `--domain <string>` Domain name
  - `--target <string>` Target host
  - `--port <int>` Target port
  - `--type <string>` Proxy type
  - `--https` Enforce HTTPS
- `futrou proxy rm <id>` — Delete a proxy
- `futrou proxy purge <id>` — Purge cached responses for a proxy
- `futrou proxy metrics <id>` — Get metrics for a proxy
- `futrou proxy logs <id>` — View logs for a proxy
  - `--offset <int>` Offset into the log stream
  - `--limit <int>` Maximum number of log entries to return
  - `--search <string>` Filter logs by search term
  - `--start-at <string>` Only return logs at or after this time
  - `--end-at <string>` Only return logs at or before this time
  - `--tail` Show only the most recent log entries
  - `--live` Keep streaming new log entries; use --live=false to print once and exit (default true)
- `futrou proxy logs tail <id>` — View recent logs for a proxy
  - `--offset <int>` Offset into the log stream
  - `--limit <int>` Maximum number of log entries to return
  - `--search <string>` Filter logs by search term
  - `--start-at <string>` Only return logs at or after this time
  - `--end-at <string>` Only return logs at or before this time
  - `--tail` Show only the most recent log entries
  - `--live` Keep streaming new log entries; use --live=false to print once and exit (default true)

- `futrou dns ls` — List all DNS zones
- `futrou dns show <id>` — Show a DNS zone by ID
- `futrou dns create` — Create a DNS zone
  - `--name <string>` Zone domain name (required)
- `futrou dns update <id>` — Update a DNS zone
  - `--name <string>` Zone domain name
- `futrou dns rm <id>` — Delete a DNS zone
- `futrou dns metrics <id>` — Get metrics for a DNS zone
- `futrou dns logs <id>` — View logs for a DNS zone
  - `--offset <int>` Offset into the log stream
  - `--limit <int>` Maximum number of log entries to return
  - `--search <string>` Filter logs by search term
  - `--start-at <string>` Only return logs at or after this time
  - `--end-at <string>` Only return logs at or before this time
  - `--tail` Show only the most recent log entries
  - `--live` Keep streaming new log entries; use --live=false to print once and exit (default true)
- `futrou dns logs tail <id>` — View recent logs for a DNS zone
  - `--offset <int>` Offset into the log stream
  - `--limit <int>` Maximum number of log entries to return
  - `--search <string>` Filter logs by search term
  - `--start-at <string>` Only return logs at or after this time
  - `--end-at <string>` Only return logs at or before this time
  - `--tail` Show only the most recent log entries
  - `--live` Keep streaming new log entries; use --live=false to print once and exit (default true)
- `futrou dns record ls <zone-id>` — List records in a DNS zone
- `futrou dns record show <zone-id> <record-id>` — Show a DNS record
- `futrou dns record create <zone-id>` — Create a DNS record
  - `--name <string>` Record name (e.g. www or @) (required)
  - `--type <string>` Record type (A, AAAA, CNAME, MX, TXT, ...) (required)
  - `--value <string>` Record value (required)
  - `--ttl <int>` TTL in seconds (default 300)
  - `--priority <int>` Priority (MX, SRV records)
- `futrou dns record update <zone-id> <record-id>` — Update a DNS record
  - `--name <string>` Record name
  - `--type <string>` Record type
  - `--value <string>` Record value
  - `--ttl <int>` TTL in seconds
  - `--priority <int>` Priority
- `futrou dns record rm <zone-id> <record-id>` — Delete a DNS record

- `futrou storage ls` — List storages in a project (defaults to futrou.json|yaml, then the login workspace's "default" project)
  - `--workspace <string>` Workspace name or ID (defaults to futrou.json|yaml, then the workspace selected at login)
  - `--project <string>` Project name or ID within the workspace (defaults to futrou.json|yaml, then "default")
- `futrou storage show <id>` — Show a storage by ID
- `futrou storage create` — Create a storage (defaults to futrou.json|yaml, then the login workspace's "default" project)
  - `--name <string>` Storage name (required)
  - `--plan <string>` Storage plan ID (required)
  - `--workspace <string>` Workspace name or ID (defaults to futrou.json|yaml, then the workspace selected at login)
  - `--project <string>` Project name or ID within the workspace (defaults to futrou.json|yaml, then "default")
- `futrou storage update <id>` — Update a storage
  - `--name <string>` New name
  - `--plan <string>` New storage plan ID
- `futrou storage rm <id>` — Delete a storage
- `futrou storage instance <id>` — Manage storage instances
- `futrou storage instance ls <id>` — List storage instances
- `futrou storage metrics <id>` — Get storage metrics
- `futrou storage logs <id>` — View storage logs
  - `--offset <int>` Offset into the log stream
  - `--limit <int>` Maximum number of log entries to return
  - `--search <string>` Filter logs by search term
  - `--start-at <string>` Only return logs at or after this time
  - `--end-at <string>` Only return logs at or before this time
  - `--tail` Show only the most recent log entries
  - `--live` Keep streaming new log entries; use --live=false to print once and exit (default true)
- `futrou storage plan ls` — List all storage plans
- `futrou storage plan show <id>` — Show a storage plan by ID
- `futrou storage file ls <storage-id> [path-or-glob]` — List or search files in a storage; the path may be a glob (e.g. images/*, **/*.log)
  - `--query <string>` Search query
- `futrou storage file info <storage-id> <path>` — Inspect file or folder metadata
- `futrou storage file download <storage-id> <remote-path-or-glob> <local-path>` — Download a file, folder, or glob match from a storage
- `futrou storage file upload <storage-id> <local-path-or-glob> <remote-path>` — Upload a file, folder, or glob match to a storage
- `futrou storage file mkdir <storage-id> <path>` — Create a folder in a storage
- `futrou storage file move <storage-id> <path> <destination>` — Move or rename a file or folder in a storage
- `futrou storage file rm <storage-id> <path-or-glob>` — Delete a file, folder, or glob match in a storage
  - `--recursive` Delete folders recursively
- `futrou storage sync <storage-id> [remote-path] [local-dir]` — Two-way sync between a local directory and a storage path by modification time
  - `--dry-run` Show what would change without transferring anything

- `futrou logs <name-or-id>` — Browse logs of all services
  - `--type, -t <string>` Resource type: serverlet, proxy, dns, storage, server, monitor (default serverlet)
  - `--instance <string>` Serverlet instance ID
  - `--ls` List the resources of --type whose logs can be browsed
  - `--offset <int>` Offset into the log stream
  - `--limit <int>` Maximum number of log entries to return
  - `--search <string>` Filter logs by search term
  - `--start-at <string>` Only return logs at or after this time
  - `--end-at <string>` Only return logs at or before this time
  - `--tail` Show only the most recent log entries
  - `--live` Keep streaming new log entries; use --live=false to print once and exit (default true)

- `futrou upgrade [version]` — Upgrade Futrou CLI to the latest or a specific version

- `futrou schema` — Display the Futrou API v2 OpenAPI schema
  - `--format <string>` Output format: json or go (default json)

- `futrou version` — Display the Futrou CLI version

- `futrou license` — Display the Futrou CLI license

<!-- cli-reference:end -->

---

## REST API

Base URL: `https://api.futrou.com`  
Auth: `Authorization: Bearer <token>`  
OpenAPI spec: `GET /v2/openapi.json`

### Auth

```
POST /v2/auth/login
{ "email": "...", "password": "..." }
→ { apiToken: { id, token }, user: { ... } }

# The stored API key is: <apiToken.id>-<apiToken.token>
```

### Key endpoints

Full spec: `GET https://api.futrou.com/v2/openapi.json`  
Interactive docs: https://api.futrou.com/#v2

```
# API tokens
GET    /v2/api-tokens
POST   /v2/api-tokens
GET    /v2/api-tokens/filters
GET    /v2/api-tokens/{apiTokenId}
PATCH  /v2/api-tokens/{apiTokenId}
DELETE /v2/api-tokens/{apiTokenId}

# Users
GET    /v2/users
GET    /v2/users/filters
GET    /v2/users/{userId}
PATCH  /v2/users/{userId}
DELETE /v2/users/{userId}
POST   /v2/users/{userId}/update-password
POST   /v2/users/{userId}/update-email
POST   /v2/users/{userId}/update-mfa
POST   /v2/users/{userId}/setup-mfa

# Workspaces
GET    /v2/workspaces
POST   /v2/workspaces
GET    /v2/workspaces/filters
GET    /v2/workspaces/{workspaceId}
PATCH  /v2/workspaces/{workspaceId}
DELETE /v2/workspaces/{workspaceId}
GET    /v2/workspaces/{workspaceId}/limits
GET    /v2/workspaces/{workspaceId}/contact
PATCH  /v2/workspaces/{workspaceId}/contact
GET    /v2/workspaces/{workspaceId}/projects
GET    /v2/workspaces/{workspaceId}/projects/{projectId}
GET    /v2/workspaces/{workspaceId}/projects/filters
GET    /v2/workspaces/{workspaceId}/users
POST   /v2/workspaces/{workspaceId}/users
GET    /v2/workspaces/{workspaceId}/users/filters
GET    /v2/workspaces/{workspaceId}/users/{workspaceUserId}
PATCH  /v2/workspaces/{workspaceId}/users/{workspaceUserId}
DELETE /v2/workspaces/{workspaceId}/users/{workspaceUserId}

# Projects
GET    /v2/projects
POST   /v2/projects
GET    /v2/projects/filters
GET    /v2/projects/{projectId}
PATCH  /v2/projects/{projectId}
DELETE /v2/projects/{projectId}
GET    /v2/projects/{projectId}/config
POST   /v2/projects/{projectId}/transfer
POST   /v2/projects/{projectId}/clone

# Variables
GET    /v2/variables
POST   /v2/variables
GET    /v2/variables/filters
GET    /v2/variables/{variableId}
PATCH  /v2/variables/{variableId}
DELETE /v2/variables/{variableId}

# Dns
GET    /v2/dns/plans
POST   /v2/dns/plans
GET    /v2/dns/plans/filters
GET    /v2/dns/plans/{dnsPlanId}
PATCH  /v2/dns/plans/{dnsPlanId}
DELETE /v2/dns/plans/{dnsPlanId}
GET    /v2/dns
POST   /v2/dns
GET    /v2/dns/filters
GET    /v2/dns/{dnsId}
PATCH  /v2/dns/{dnsId}
DELETE /v2/dns/{dnsId}
GET    /v2/dns/{dnsId}/metrics
GET    /v2/dns/{dnsId}/logs
GET    /v2/dns/{dnsId}/logs/tail
POST   /v2/dns/{dnsId}/purge
GET    /v2/dns/{dnsId}/export
POST   /v2/dns/{dnsId}/import
GET    /v2/dns/{dnsId}/records
POST   /v2/dns/{dnsId}/records
GET    /v2/dns/{dnsId}/records/filters
GET    /v2/dns/{dnsId}/records/{dnsRecordId}
PATCH  /v2/dns/{dnsId}/records/{dnsRecordId}
DELETE /v2/dns/{dnsId}/records/{dnsRecordId}

# Activities
GET    /v2/activities
GET    /v2/activities/filters

# Crons
GET    /v2/crons/plans
POST   /v2/crons/plans
GET    /v2/crons/plans/filters
GET    /v2/crons/plans/{cronPlanId}
PATCH  /v2/crons/plans/{cronPlanId}
DELETE /v2/crons/plans/{cronPlanId}
GET    /v2/crons
POST   /v2/crons
GET    /v2/crons/filters
GET    /v2/crons/{cronId}
PATCH  /v2/crons/{cronId}
DELETE /v2/crons/{cronId}
POST   /v2/crons/test

# Proxies
GET    /v2/proxies/plans
POST   /v2/proxies/plans
GET    /v2/proxies/plans/filters
GET    /v2/proxies/plans/{proxyPlanId}
PATCH  /v2/proxies/plans/{proxyPlanId}
DELETE /v2/proxies/plans/{proxyPlanId}
GET    /v2/proxies
POST   /v2/proxies
GET    /v2/proxies/filters
POST   /v2/proxies/check
GET    /v2/proxies/{proxyId}
PATCH  /v2/proxies/{proxyId}
DELETE /v2/proxies/{proxyId}
GET    /v2/proxies/{proxyId}/metrics
GET    /v2/proxies/{proxyId}/logs
GET    /v2/proxies/{proxyId}/logs/tail
POST   /v2/proxies/{proxyId}/purge
POST   /v2/proxies/{proxyId}/verify

# Networks
GET    /v2/networks
POST   /v2/networks
GET    /v2/networks/filters
GET    /v2/networks/{networkId}
PATCH  /v2/networks/{networkId}
DELETE /v2/networks/{networkId}
GET    /v2/networks/{networkId}/metrics
GET    /v2/networks/{networkId}/devices

# Regions
GET    /v2/regions
POST   /v2/regions
GET    /v2/regions/filters
GET    /v2/regions/{regionId}
PATCH  /v2/regions/{regionId}
DELETE /v2/regions/{regionId}

# Servers
GET    /v2/servers
POST   /v2/servers
GET    /v2/servers/filters
POST   /v2/servers/{serverId}/join
POST   /v2/servers/{serverId}/reset-join
POST   /v2/servers/{serverId}/leave
GET    /v2/servers/{serverId}
PATCH  /v2/servers/{serverId}
DELETE /v2/servers/{serverId}
GET    /v2/servers/{serverId}/logs
GET    /v2/servers/{serverId}/metrics

# Storages
GET    /v2/storages/plans
POST   /v2/storages/plans
GET    /v2/storages/plans/filters
GET    /v2/storages/plans/{storagePlanId}
PATCH  /v2/storages/plans/{storagePlanId}
DELETE /v2/storages/plans/{storagePlanId}
GET    /v2/storages
POST   /v2/storages
GET    /v2/storages/filters
GET    /v2/storages/{storageId}
PATCH  /v2/storages/{storageId}
DELETE /v2/storages/{storageId}
GET    /v2/storages/{storageId}/instances
GET    /v2/storages/{storageId}/metrics
GET    /v2/storages/{storageId}/logs
GET    /v2/storages/{storageId}/logs/tail
GET    /v2/storages/{storageId}/files
POST   /v2/storages/{storageId}/files/mkdir/*
POST   /v2/storages/{storageId}/files/move/*
GET    /v2/storages/{storageId}/files/*
PUT    /v2/storages/{storageId}/files/*
DELETE /v2/storages/{storageId}/files/*

# Serverlets
GET    /v2/serverlets/plans
POST   /v2/serverlets/plans
GET    /v2/serverlets/plans/filters
GET    /v2/serverlets/plans/{serverletPlanId}
PATCH  /v2/serverlets/plans/{serverletPlanId}
DELETE /v2/serverlets/plans/{serverletPlanId}
GET    /v2/serverlets
POST   /v2/serverlets
GET    /v2/serverlets/filters
GET    /v2/serverlets/{serverletId}/icon
GET    /v2/serverlets/{serverletId}
PATCH  /v2/serverlets/{serverletId}
DELETE /v2/serverlets/{serverletId}
GET    /v2/serverlets/{serverletId}/instances
GET    /v2/serverlets/{serverletId}/instances/{instanceId}
POST   /v2/serverlets/{serverletId}/instances/{instanceId}/start
POST   /v2/serverlets/{serverletId}/instances/{instanceId}/stop
POST   /v2/serverlets/{serverletId}/instances/{instanceId}/restart
POST   /v2/serverlets/{serverletId}/instances/{instanceId}/exec
GET    /v2/serverlets/{serverletId}/instances/{instanceId}/metrics
GET    /v2/serverlets/{serverletId}/instances/{instanceId}/logs
GET    /v2/serverlets/{serverletId}/instances/{instanceId}/users
GET    /v2/serverlets/{serverletId}/instances/{instanceId}/processes
POST   /v2/serverlets/{serverletId}/start
POST   /v2/serverlets/{serverletId}/stop
POST   /v2/serverlets/{serverletId}/restart
POST   /v2/serverlets/{serverletId}/wake
POST   /v2/serverlets/{serverletId}/exec
POST   /v2/serverlets/{serverletId}/sync
GET    /v2/serverlets/{serverletId}/metrics
GET    /v2/serverlets/{serverletId}/logs
GET    /v2/serverlets/{serverletId}/users
GET    /v2/serverlets/{serverletId}/processes
```

### Example: deploy via API

```bash
# 1. Login
TOKEN=$(curl -s -X POST https://api.futrou.com/v2/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"you@example.com","password":"secret"}' \
  | jq -r '"\(.apiToken.id)-\(.apiToken.token)"')

# 2. Create serverlet
curl -X POST https://api.futrou.com/v2/serverlets \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"my-app","image":"nginx:latest","minInstances":1,"maxInstances":2}'
```

### Error format

```json
{
  "message": "Human-readable error",
  "requestId": "...",
  "errors": [{ "field": "name", "message": "...", "code": "..." }]
}
```

---

## MCP

Futrou provides an MCP server at `https://mcp.futrou.com` for AI agents to interact with your workspace, manage projects and services, and more.

### Add to Claude Code

```bash
claude mcp add futrou --transport http https://mcp.futrou.com \
  --header "Authorization: Bearer <your-api-token>"
```

Or add to your project's `.mcp.json`:

```json
{
  "mcpServers": {
    "futrou": {
      "type": "http",
      "url": "https://mcp.futrou.com",
      "headers": {
        "Authorization": "Bearer <your-api-token>"
      }
    }
  }
}
```

Once connected, the AI agent can list and manage your serverlets, proxies, networks, storages, projects, DNS zones, and more directly through natural language — no CLI commands needed.

### Authentication

Use your Futrou API token in the `Authorization` header:

```
Authorization: Bearer <token>
```

Get a token by running `futrou login` and reading `~/.futrou/cli.json`, or from the Futrou dashboard.

### Docs

- REST API: https://api.futrou.com/v2
- MCP server: https://mcp.futrou.com
- Full API docs: https://api.futrou.com
