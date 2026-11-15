# Futrou CLI

Go 1.27 CLI (`urfave/cli/v2`) for Futrou Cloud. Commands live in `src/commands/`, the
declarative `futrou.json` model in `src/config/`, its validators in
`src/validators/`, and the Terraform-style plan/apply logic in `src/deployer/`.

## Command conventions

- Resource groups are **singular** (`serverlet`, `proxy`, `network`, `server`, ...).
- Listing is `ls`, deleting is `rm`, details are `show`. `get`, `inspect`, `list`, `delete`, `remove` and the
  old plural group names keep working as **hidden** aliases. Register them in the
  `legacyNames` table in `src/commands/aliases.go`; never add them as visible `Aliases`. (`up` is the one deliberate visible alias.)
- Align naming with Docker, Docker Compose, `uc` (Uncloud) and Fly.io (`flyctl`).
  Top-level verbs: `deploy` (visible alias `up`, listed as "up (deploy)" in help via `helpDisplayNames`), `down` (hidden alias `teardown`),
  `logs`, and `init` with `init config`, `init mcp`, `init skills` (hidden alias `setup`).
  The `cloud` command belongs to nasua, not this CLI; do not add it back.
- Log commands share `logFlags` (`--offset --limit --search --start-at --end-at --tail --live`).
  Logs print once by default; `--live` keeps following and `--tail` keeps the recent entries (as in the
  nasua CLI). Every resource's `logs`
  command must expose exactly the query parameters its API endpoint accepts.
- `--api-url` / `FUTROU_API_URL` accept `host`, `host:port` or `https://host/v2` (https added, http for loopback), as in nasua.
  Use `utils.NormalizeApiUrl` and `utils.IsUuid`; a test fails when a package defines its own copy.
- Go variables of commands are named after the command, singular (`serverletCommand`); a test checks it.
  A renamed flag keeps its old name as a **hidden** flag (see `firstSet`), as for aliases.
- `--project` is always the Cloud project. `deploy`/`down`/`config` pick a config file with `--project-name` (`-p`).
- CLI login and credentials (`~/.futrou/cli.json`) are in `src/cliconfig`; the `futrou.json` model is in `src/config`.
- Project run folders are constants (`constants.BuildDir`, `CacheDir`, `StaticoDir`, `TmpDir`, `DebugDir`) with helpers
  such as `utils.BuildDir(base)`; never write the names as string literals (a test checks it).
- API types in `src/api/types.go` are generated from `https://api.futrou.com/v2/openapi.json`
  (`make build-types`). Do not edit them by hand.

## After every change, keep these in sync

1. **Docs:** `make build-docs` regenerates the command/flag reference inside
   `SKILL.md` and `README.md` (between the `cli-reference` markers). Run it after any
   change to a command, subcommand, flag, alias or usage text. `TestReferenceUpToDate`
   fails when it is stale. Also update the hand-written examples in `SKILL.md` and
   `README.md` when behavior changes, so they match all parameters.
2. **Config schema:** `make build-schema` regenerates `futrou.schema.json` after any
   change to the `futrou.json` model or its validators.
3. **Validators:** every field of every model that can appear in `futrou.json` needs a
   validator in `src/validators/config_validator.go`, and `futrou deploy` / `futrou down`
   must validate before touching the API.
4. **Examples:** `examples/futrou.json`, `examples/futrou.yaml` and `examples/futrou.mjs` set
   every supported option and must stay equivalent (a test checks it). Add every new
   `futrou.json` option to all three. `make build-docs` embeds them in `SKILL.md`; `README.md` only links to them.
5. **Config formats:** discovery order is `futrou.json`, `futrou.js`, `futrou.cjs`,
   `futrou.mjs`, `futrou.ts`, `futrou.yaml`, `futrou.yml`, then a Docker Compose file (`compose.yaml`,
   `compose.yml`, `docker-compose.yaml`, `docker-compose.yml`). YAML is converted to the JSON
   equivalent (`Config.LoadYAML`) and then validated like JSON. Loading is `Config.Load`, which
   dispatches to `LoadJSON`/`LoadJS`/`LoadYAML`; saving is `Config.Save`, which dispatches to
   `SaveJSON`/`SaveYAML` and keeps the file's format (`SaveJS` is unsupported: JS/TS configs are code). Compose files are converted by
   `ConvertCompose`/`Config.LoadCompose` (`src/config/compose.go`), warn about every option they
   cannot convert, take Futrou-only settings from `x-futrou`, and are never saved. Support a new
   Compose option there and in `examples/docker-compose.yml`.
   `deploy`/`down`/`config --project-name <name>` (`-p`; the old `--project` is hidden, because `--project` is the Cloud project everywhere else) load `futrou.<name>.*` first (`Config.LoadProject`), else the
   default config renamed to the project; `-f` always wins.
   Collections (`networks`, `storages`, `statico`, `serverlets`, `proxies`, `dns`, `crons`) may be a list or
   an object keyed by name (domain for proxies/dns); `normalizeKeyedCollections` converts the
   object form to a list before validation, `ToJSONSchema` allows both, and saving writes lists.
6. **Region:** top-level `region` (name or ID) defaults `regionId` of networks, serverlets,
   proxies, storages, statico websites and crons (not DNS); a resource's own `regionId` wins.
   `entrypoint` is one string; an array is joined with spaces (`config.Entrypoint`).
   `needs` entries are `name`, `name:started` or `name:healthy`; they are validated together
   (declared, no self-need, no cycles) and the deployer orders serverlets by them.
7. **Deploy behavior:** `deploy` prints a plan and asks `[y/N]` (`y`/`yes` apply, Enter
   cancels); `-y/--yes` and `CI=true` apply without asking; `--dry-run` only shows the
   plan. `down` follows the same rules.

## Pitfalls (found in review)

- HTTP handlers that send on a channel use a non-blocking send and ignore requests that carry no result (browsers ask for `/favicon.ico`).
- Only a 401 means "log in again"; never clear a token or advise `futrou login` for a 403 or a permission error.
- A cancelled or timed-out request is not a transport failure (do not mark a host as no-QUIC); a `RoundTripper` never changes its request.
- Names turned into folder names must stay unique (`folderName` adds a hash when it changes a name).
- Resolve the workspace once per plan (`planWorkspace`): config value, else the login default.
- A doc comment goes directly above its function; check this when inserting code between them.

## Statico (static websites)

- Types: `Statico` is a website and `StaticoVersion` one deploy that can be activated (`src/statico`).
- Config key `statico` (list or keyed object, `StaticoConfig`). `name`, `regionId`, `public` are the website;
  `include`, `htmlToFolders`, `defaultFile/Status`, `fallbackFile/Status`, `redirects` describe its next version and are
  never sent with the website (`deployer.staticoPayload`). `include` is an **ordered** object (`statico.Rules`, `glob.go`):
  key = file/folder/glob relative to the config (`Config.File`/`BaseDir`) or absolute, value = `true` (keep the path),
  a destination, or `false` (exclude). Key order must survive loading: JSON/YAML/JS go through unordered maps, so
  `includeOrders` reads it from the file and `normalizeStaticoInclude` makes an array of one-key objects first.
  Redirects in the config are `{from?, to, statusCode?}`; a `_redirects` file (Cloudflare Pages syntax, `redirects.go`)
  is converted to the node's `if`/`then` rules, with named regex groups that the node fills into `to` (`PathMatcher.Expand`
  in nasua), and is set on the version after it is created, not uploaded.
- `Resolve` (`files.go`) runs the rules (hidden files/folders and `.map` files found below an include are skipped by default, `defaultExcluded`; a source that names one takes it), adds `page/index.html` copies (HTML to folders), reads `_redirects`; `Stage` copies the
  result to `.futrou/statico/<name>/<version>`; `Sync`/`Check` (`upload.go`) are used by both `futrou statico deploy`
  and `futrou deploy` (the deployer has no upload action: a changed website is a normal create/update with `Details`
  lines and an `after` step). `.futrou` is cleared at the start of every deploy and upload except `.futrou/cache` (hash cache, install record: `utils.CacheDir(base)`); `.futrou` and `cache` are created only when something is written, with a self-ignoring `.gitignore`. A failed upload keeps its
  staged files. Progress is `statico.Progress` (`terminal.go`: live batch on a terminal, lines otherwise).
- Files live in the storage `statico-<id>`, one folder per version; every request uses the CLI's normal API token (Cloud
  authorises it and forwards to the node), the CLI never holds a node credential. Tests use `src/statico/mock`, a fake of
  those endpoints. The flags of repeatable options hold commas (`DisableSliceFlagSeparator`).

## Install and build commands

- Top-level `installCommand` / `buildCommand` (strings, `Config`). `runProjectCommands` (`commands/build.go`) runs them with
  `utils.RunShell` (sh -c, cmd /C on Windows) in `Config.BaseDir()`, streaming stdout/stderr, in `deploy` (after validation,
  before login) and `statico deploy` (before anything). The install is remembered in `<config folder>/.futrou/cache/done-commands.json`
  (`utils.CommandDone`), keyed by the command, only after it succeeded; the build runs every time, also with
  `--dry-run`. A failure stops the deploy before any API request. JSON mode sends their output to stderr.

## Release signing

- `make sign` (`scripts/sign.sh`, same as in nasua) signs every file in `dist/` (binaries,
  `install.sh`, `install.ps1`) with a detached `<file>.asc` and writes a signed `SHA256SUMS`, the only checksum file (`build.sh` no longer writes `checksums.txt`). CI
  (`release-run.yml`) runs it before uploading `dist/*`, with the secrets `GPG_PRIVATE_KEY_BASE64`
  and `GPG_PASSPHRASE` (optional, empty for a key without one). The public key is not in this repo or
  the binary: it is published at the URL below.
- `install.sh [version] [--ignore-signature]` and `install.ps1 -IgnoreSignature` verify the binary against
  `https://futrou.com/.well-known/pgp-key.txt` in a private keyring; without `gpg` they only warn.

## Tests

- Every change and every new behavior needs tests, including error and edge cases.
- A test file is named after the source file it tests: `storage.go` is tested in
  `storage_test.go`. An optional category suffix is allowed (`storage_logs_test.go`),
  but the prefix must match an existing source file in the same directory.
  `scripts/check-test-names.sh` (run by `make test`) enforces this.
- Every source file has a test file named after it (`storage.go` has `storage_test.go`). This is a rule for you,
  not a script: when you add a source file, add its test file in the same change.
- Commands are tested through `runArgs` against the mock server in `root_test.go`.
  Flags must come before positional arguments in test invocations.

## Before every commit

- `make build` and `make test` must both pass. Fix failures instead of committing around them.
- Commit messages are short and imperative. **Do not add a `Co-Authored-By` line** (or any
  other co-author/attribution trailer) to commits.
