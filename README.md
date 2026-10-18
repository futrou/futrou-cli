![Futrou CLI Banner](.github/assets/banner.jpg)

# Futrou CLI

Futrou CLI is a command-line tool for deploying and managing resources on Futrou Cloud — serverlets, proxies, DNS zones, networks, storages, projects and more.

## Installation

### Linux / macOS
```bash
curl -fsSL https://futrou.com/install.sh | bash
```

### Windows (PowerShell)
```powershell
irm https://futrou.com/install.ps1 | iex
```

### npm / npx (Linux, MacOS and Windows)
Requires [Node.js](https://nodejs.org/) and npm.

```bash
npm install -g futrou
# or run without installing
npx futrou --help
```

### Upgrade
```bash
futrou upgrade           # upgrade to latest
futrou upgrade 1.2.0     # upgrade/downgrade to specific version
```

## Supported platforms
- linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64, windows/arm64, freebsd/amd64, freebsd/arm64

## Quick start

```bash
futrou login                       # sign in (opens the browser)
futrou init config                 # create futrou.json (or: -f futrou.yaml)
futrou deploy --dry-run            # show the plan
futrou deploy                      # apply it after confirming (alias: futrou up)
futrou serverlet ls                # list serverlets
futrou logs web                    # stream the logs of the serverlet "web"
futrou down                        # delete everything in the project, then the project
futrou init mcp                    # let AI coding agents use Futrou
futrou init skills
```

Every resource has the same verbs: `ls`, `show`, `create`, `update` and `rm` (the older `list`, `get`, `inspect`, `delete` and plural names such as `serverlets` still work). The full list of commands and flags is in the [command reference](#command-reference).

## Deploying

`futrou deploy` validates `futrou.json`, prints a Terraform-style plan and asks `[y/N]` before applying it. Pass `-y` to skip the prompt (it is skipped automatically when `CI=true`) or `--dry-run` to only show the plan. `futrou up` is an alias. `futrou down` (alias `teardown`) plans the deletion of every resource in the project and then the project itself, with the same confirmation rules.

## Configuration

`futrou deploy` reads the project configuration from the first of these files it finds in the current directory: `futrou.json`, `futrou.js`, `futrou.cjs`, `futrou.mjs`, `futrou.ts`, `futrou.yaml`, `futrou.yml` (or the file given with `-f`). YAML files are converted to the equivalent `futrou.json` before they are validated, so everything below applies to all formats. `.js`, `.cjs` and `.mjs` need Node (or Bun/Deno); `.ts` needs Bun or Deno. When the CLI updates a project config (`futrou config pull`, or the lock refresh after a deploy) it writes it back in the same format, JSON or YAML (comments in YAML files are not preserved). `.js`, `.cjs`, `.mjs` and `.ts` configs are code and are never rewritten.

The files in [examples/](examples) set every supported option and are tested to be equivalent. `futrou init config` creates a starter file (`-f futrou.yaml` for YAML), `futrou init mcp` and `futrou init skills` set up AI coding agents.

Top-level `region` (a region name or ID) is the default region of every network, serverlet, proxy, storage and cron in the project; a resource that sets its own `regionId` keeps it. A serverlet's `needs` lists the serverlets it waits for, as `db`, `db:started` or `db:healthy`; each must be declared in the same config and cycles are rejected, and serverlets are created after the ones they need (and deleted before them). A serverlet's `entrypoint` is one string such as `node server.js`; an array is accepted and joined with spaces.

### futrou.json

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

### futrou.yaml

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

### futrou.mjs

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

## Command reference

Commands use singular names with `ls` and `rm` (the older plural names, `list` and `delete` still work as hidden aliases).

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

## Agent Skill

Install the Futrou skill for your AI coding agent (Claude Code, Cursor, Copilot, Codex, and 14+ others):

```bash
npx skills add futrou/futrou-cli
```

The skill teaches your agent how to use the Futrou CLI and REST API — deploying serverlets, managing proxies, DNS, networks, storages, projects, and the MCP server at `mcp.futrou.com`.

## Development

### Requirements
- **Go 1.27+**

### Setup
```bash
# macOS
brew install go

# Ubuntu/Debian
sudo snap install go --classic
```

Go 1.27 or newer is required (see `go.mod`). Install it from [go.dev/dl](https://go.dev/dl/) if your package manager ships an older version.

```bash
git clone git@github.com/futrou/futrou-cli.git
make install
```

Run the dev server (auto-rebuilds on changes):
```bash
make
```

### Build
```bash
make build        # build all platforms (also regenerates the schema and the command reference)
make build-docs   # regenerate the command reference in README.md and SKILL.md
make build-npm    # build npm-distributable package
make release      # build all platforms + npm
```

### Start
```bash
make start
make start login
make start serverlet ls
make start ARGS="--help"
```

### Tests
```bash
make test
go test ./src/commands/... -v
```

## Release

Steps to release a new version:
1. Bump the version in `.env` using `make version bump` or set it manually with `make version set 1.2.0`
2. Create a new tag in the format `v{version}` — e.g. `v1.2.0`
3. Push the tag to GitHub: `git push origin v1.2.0`
4. The release pipeline will build binaries for all platforms and publish the release
5. Done

## License
[MIT License](LICENSE)
