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
  `--live` defaults to true; `--live=false` prints once. Every resource's `logs`
  command must expose exactly the query parameters its API endpoint accepts.
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
6. **Region:** top-level `region` (name or ID) defaults `regionId` of networks, serverlets,
   proxies, storages and crons (not DNS); a resource's own `regionId` wins.
   `entrypoint` is one string; an array is joined with spaces (`config.Entrypoint`).
   `needs` entries are `name`, `name:started` or `name:healthy`; they are validated together
   (declared, no self-need, no cycles) and the deployer orders serverlets by them.
7. **Deploy behavior:** `deploy` prints a plan and asks `[y/N]` (`y`/`yes` apply, Enter
   cancels); `-y/--yes` and `CI=true` apply without asking; `--dry-run` only shows the
   plan. `down` follows the same rules.

## Tests

- Every change and every new behavior needs tests, including error and edge cases.
- A test file is named after the source file it tests: `storages.go` is tested in
  `storages_test.go`. An optional category suffix is allowed (`storages_logs_test.go`),
  but the prefix must match an existing source file in the same directory.
  `scripts/check-test-names.sh` (run by `make test`) enforces this.
- Commands are tested through `runArgs` against the mock server in `root_test.go`.
  Flags must come before positional arguments in test invocations.

## Before every commit

- `make build` and `make test` must both pass. Fix failures instead of committing around them.
- Commit messages are short and imperative. **Do not add a `Co-Authored-By` line** (or any
  other co-author/attribution trailer) to commits.
