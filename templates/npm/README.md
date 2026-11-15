![Futrou CLI Banner](.github/assets/banner.jpg)

# Futrou CLI

The **Futrou CLI** allows you to manage your Futrou projects and infrastructure from your terminal.

## Installation
To install the CLI globally, run:

```bash
npm install -g futrou
```

Or use any other favorite package manager:

```bash
yarn global add futrou
```

## Usage

Once installed, you can access the CLI using the `futrou` command (or `npx futrou` without installing).

```bash
futrou login                       # sign in (opens the browser)
futrou deploy                      # apply it after confirming (alias: futrou up)
futrou serverlet ls                # list serverlets
futrou init mcp                    # let AI coding agents use Futrou
futrou init skills
```

## Infrastructure as code

Describe your infrastructure in a `futrou.yaml` file and deploy it with `futrou deploy`:

```yaml
# yaml-language-server: $schema=https://futrou.com/futrou.schema.json
workspace: acme
project: website

proxies:
  www.example.com: # the key is the domain
    type: http
    target: 10.20.0.5:8080
    enforceHttps: true
```

```bash
futrou deploy             # show the plan, then ask before applying
```

`futrou.json`, `futrou.yml`, JavaScript/TypeScript configs and Docker Compose files
work too. See the full [example with every option](https://github.com/futrou/futrou-cli/blob/main/examples/futrou.yaml).
The JSON Schema ships with the package as `futrou/futrou.schema.json`.

## Documentation

For more detailed guides on how to use the CLI, please visit [futrou.com/docs](https://futrou.com/docs) and [github.com/futrou/futrou-cli](https://github.com/futrou/futrou-cli).

## License
[MIT License](LICENSE)
