---
name: futrou-cli
description: Use when working with Futrou Cloud — deploying serverlets, managing proxies, DNS, volumes, projects, or calling the Futrou REST API directly.
---

# Futrou Cloud Skill

Futrou Cloud lets you run containerised workloads (serverlets), manage HTTP/TCP proxies, DNS zones, persistent volumes, and projects — all from the CLI or REST API.

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
futrou login                      # interactive prompt
futrou login --email you@example.com --password secret

# Or use env vars / flags instead of stored credentials
FUTROU_API_TOKEN=<token> futrou serverlets list
futrou --api-key <token> serverlets list
```

Credentials are stored in `~/.futrou/cli.json`. The env var `FUTROU_API_TOKEN` always takes precedence.

---

## Core concepts

| Resource | What it is |
|---|---|
| **Serverlet** | A containerised service — runs one or more instances of a Docker image |
| **Proxy** | HTTP/TCP reverse proxy with a domain, TLS, and load balancing |
| **Volume** | Persistent block storage attached to a serverlet |
| **Project** | Logical grouping for serverlets, crons, and variables |
| **DNS** | DNS zone management |

---

## CLI reference

### Global flags

```bash
futrou --api-key <token>          # override stored token
futrou --api-url <url>            # override API base URL (default: https://api.futrou.com)
futrou --log-format json          # json or plain (default: plain)
futrou --log-level debug          # debug, info, warn, error
```

### Auth

```bash
futrou login
futrou logout
```

### Deploy (declarative)

Create a `futrou.json` in your project root:

```json
{
  "name": "my-app",
  "image": "nginx:latest",
  "serverletPlanId": "<plan-id>",
  "workspaceId": "<workspace-id>",
  "projectId": "<project-id>",
  "minInstances": 1,
  "maxInstances": 3,
  "env": {
    "PORT": "8080"
  }
}
```

Then deploy:

```bash
futrou init                       # interactive wizard — creates futrou.json
futrou deploy                     # create or update serverlet from futrou.json
futrou deploy -y                  # skip confirmation prompt
futrou deploy --destroy           # delete the serverlet
futrou deploy -f custom.json      # use a specific config file
```

### Serverlets

```bash
futrou serverlets list
futrou serverlets get <id>
futrou serverlets create --name my-app --image nginx:latest --plan <id> --min 1 --max 3
futrou serverlets update <id> --image nginx:1.25 --max 5
futrou serverlets delete <id>
futrou serverlets start <id>
futrou serverlets stop <id>
futrou serverlets restart <id>
futrou serverlets logs <id>
futrou serverlets instances <id>
```

### Proxies

```bash
futrou proxies list
futrou proxies get <id>
futrou proxies create --domain app.example.com --target my-app.internal --port 8080 --type http --https
futrou proxies update <id> --target new-target --port 9000
futrou proxies delete <id>
futrou proxies verify <id>        # trigger domain/TLS verification
```

Proxy types: `http`, `tcp`, `udp`

### Volumes

```bash
futrou volumes list
futrou volumes get <id>
futrou volumes create --name my-data --size 20 --type ssd
futrou volumes update <id> --size 50
futrou volumes delete <id>
```

### DNS

```bash
futrou dns list
futrou dns get <id>
futrou dns create --zone example.com
futrou dns delete <id>
```

### Projects

```bash
futrou projects list
futrou projects get <id>
futrou projects create --name my-project
futrou projects delete <id>
```

### Other

```bash
futrou upgrade                    # upgrade CLI to latest
futrou upgrade 2.0.5              # upgrade/downgrade to specific version
futrou schema                     # print the OpenAPI schema for https://api.futrou.com
```

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
# Serverlets
GET    /v2/serverlets
POST   /v2/serverlets
GET    /v2/serverlets/{id}
PATCH  /v2/serverlets/{id}
DELETE /v2/serverlets/{id}
POST   /v2/serverlets/{id}/start
POST   /v2/serverlets/{id}/stop
POST   /v2/serverlets/{id}/restart
POST   /v2/serverlets/{id}/sync
GET    /v2/serverlets/{id}/logs
GET    /v2/serverlets/{id}/metrics
GET    /v2/serverlets/{id}/instances
GET    /v2/serverlets/{id}/instances/{instanceId}/logs
GET    /v2/serverlets/{id}/instances/{instanceId}/metrics
POST   /v2/serverlets/{id}/instances/{instanceId}/start
POST   /v2/serverlets/{id}/instances/{instanceId}/stop
POST   /v2/serverlets/{id}/instances/{instanceId}/restart
GET    /v2/serverlets/plans

# Proxies
GET    /v2/proxies
POST   /v2/proxies
GET    /v2/proxies/{id}
PATCH  /v2/proxies/{id}
DELETE /v2/proxies/{id}
POST   /v2/proxies/{id}/verify

# Volumes
GET    /v2/volumes
POST   /v2/volumes
GET    /v2/volumes/{id}
PATCH  /v2/volumes/{id}
DELETE /v2/volumes/{id}
GET    /v2/volumes/plans

# DNS
GET    /v2/dns
POST   /v2/dns
GET    /v2/dns/{id}
PATCH  /v2/dns/{id}
DELETE /v2/dns/{id}
GET    /v2/dns/{id}/records
POST   /v2/dns/{id}/records
PATCH  /v2/dns/{id}/records/{recordId}
DELETE /v2/dns/{id}/records/{recordId}
GET    /v2/dns/{id}/export
POST   /v2/dns/{id}/import
GET    /v2/dns/plans

# Workspaces & Projects
GET    /v2/workspaces
POST   /v2/workspaces
GET    /v2/workspaces/{id}
PATCH  /v2/workspaces/{id}
GET    /v2/workspaces/{id}/projects
GET    /v2/workspaces/{id}/projects/{projectId}
GET    /v2/workspaces/{id}/users
POST   /v2/workspaces/{id}/users
PATCH  /v2/workspaces/{id}/users/{userId}
DELETE /v2/workspaces/{id}/users/{userId}
GET    /v2/workspaces/{id}/limits
PATCH  /v2/workspaces/{id}/contact

# Deployments
GET    /v2/deployments
POST   /v2/deployments
GET    /v2/deployments/{id}
PATCH  /v2/deployments/{id}
DELETE /v2/deployments/{id}
GET    /v2/deployments/schema

# Variables (env vars per serverlet/project)
GET    /v2/variables
POST   /v2/variables
GET    /v2/variables/{id}
PATCH  /v2/variables/{id}
DELETE /v2/variables/{id}

# Crons
GET    /v2/crons
POST   /v2/crons
GET    /v2/crons/{id}
PATCH  /v2/crons/{id}
DELETE /v2/crons/{id}
POST   /v2/crons/test
GET    /v2/crons/plans

# TLS Certificates
GET    /v2/certs
POST   /v2/certs
GET    /v2/certs/{id}
DELETE /v2/certs/{id}
POST   /v2/certs/{id}/renew

# API Tokens
GET    /v2/api-tokens
POST   /v2/api-tokens
GET    /v2/api-tokens/{id}
PATCH  /v2/api-tokens/{id}
DELETE /v2/api-tokens/{id}

# Auth
POST   /v2/auth/login
POST   /v2/auth/logout
POST   /v2/auth/signup
POST   /v2/auth/reset-password
GET    /v2/auth/context

# Users
GET    /v2/users
GET    /v2/users/{id}
PATCH  /v2/users/{id}

# Misc
GET    /v2/activities
GET    /v2/transactions
GET    /v2/servers
GET    /v2/whois/{domain}
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

Once connected, the AI agent can list and manage your serverlets, proxies, volumes, projects, DNS zones, and more directly through natural language — no CLI commands needed.

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
