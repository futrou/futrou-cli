# Futrou CLI

Futrou CLI is a command-line tool for deploying and managing resources on Futrou Cloud — serverlets, proxies, DNS zones, volumes, projects and more.

## Requirements
- **GoLang 1.25+**

## Supported platforms
- linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64, windows/arm64, freebsd/amd64, freebsd/arm64

## Installation

### 1. Clone the repository
```bash
git clone git@github.com/futrou/futrou-cli.git
```

### 2. Install Go from package repository
```bash
# macOS
brew install go
```

```bash
# Ubuntu/Debian based distros
sudo apt install golang
```

Or install the latest version from source on [GoLang website](https://go.dev/dl/)

### 3. Install go packages
```bash
make install
```

## Development
Run the following command to start the development server. It will automatically rebuild when you make changes to the code.
```bash
make
```

## Build
To build the app for the target platforms specified in the `.env` file under `PLATFORMS`, run:
```bash
make build
```

## Start
To run the built binary for the current platform:
```bash
make start
make start login
make start serverlets list
make start ARGS="--help"
```

## Tests
```bash
make test
```

### Run tests for a specific package
```bash
go test ./src/commands/... -v
```

## npm package
To build the npm-distributable package (includes all platform binaries):
```bash
make build-npm
```

## Release
To build all platforms and the npm package in one step:
```bash
make release
```

## Installation of released version

### Linux / macOS
```bash
curl -fsSL https://futrou.com/install.sh | bash
```

### Windows (PowerShell)
```powershell
irm https://futrou.com/install.ps1 | iex
```

### npm / npx
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

## Stable release
To release a new version, create a new release in the GitHub UI.
After the release is created, GitHub Actions will build the app for all target platforms and attach the binaries to the release.

Steps to release a new version:
1. Bump the version in `.env` using `make version bump` or set it manually with `make version set 1.2.0`
2. Create a new tag in the format `v{version}` — e.g. `v1.2.0`
3. Push the tag to GitHub: `git push origin v1.2.0`
4. The release pipeline will build binaries for all platforms and publish the release
5. Done

## Agent Skill

Install the Futrou skill for your AI coding agent (Claude Code, Cursor, Copilot, Codex, and 14+ others):

```bash
npx skills add futrou/futrou-cli
```

The skill teaches your agent how to use the Futrou CLI and REST API — deploying serverlets, managing proxies, DNS, volumes, projects, and the MCP server at `mcp.futrou.com`.

## License
[MIT License](LICENSE)