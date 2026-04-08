# phis-host

`phis-host` is the privileged host integration companion for `phis`.

## Scope

`phis-host` should handle host-level lifecycle tasks such as:

- site path creation and initialization
- controlled copy or release linking
- `systemd` unit installation and lifecycle
- `nginx` or `apache` config installation and reload

It should not replace the normal `phis` CLI. The split is:

- `phis`: database actions, `phis-runtime.json` management, config generation
- `phis-host`: path, copy, init, and lifecycle management on the host

`phis-host` is optional:

- it is a helper for host bootstrap and lifecycle work
- it may be versioned separately from `phis`
- it may also be installed separately when a deployment only needs the main `phis` app and CLI
- it is the place where the full host-side automation of `phi-server` lifecycle can live
- it should support both Debian-based host setups and Docker-based lifecycle flows
- when a command requires elevated privileges, `phis-host` should prefer telling the operator to rerun the same command with `sudo`

## Local Development

This repository is initialized as a standalone Go project.

`phis-host` reads its own stage configuration from:

- `./phis-host.json`
- or `PHIS_HOST_CONFIG`
- or `/var/lib/phis/phis-host/config.json`

An example file is included as [phis-host.example.json](./phis-host.example.json).

Recommended host layout:

- `phis-host` config:
  - `/var/lib/phis/phis-host/config.json`
- stage roots:
  - `/srv/phis/stages/prod`
  - `/srv/phis/stages/test`
  - `/srv/phis/stages/dev`

Expected ownership:

- mutable host state and stage roots should be owned by `phis:phis`

Current command groups:

Append `--json|-j` to any command for JSON output.

- `phis-host stage list`
- `phis-host stage init [--stage|-s <stage>] [--root|-r <path>] [--phis-path <path>]`
- `phis-host stage init db [--stage|-s <stage>] --admin-uri <postgres-uri> [--db-password <password>] [--force|-f]`
- `phis-host site show runtime --key|-k <site key> [--stage|-s <stage>] [--phis-path <path>]`
- `phis-host site show source --key|-k <site key> [--stage|-s <stage>] [--phis-path <path>]`
- `phis-host site show instances --key|-k <site key> [--stage|-s <stage>] [--phis-path <path>]`

`stage init` prepares the local host state for one stage:

- creates or updates the `phis-host` config
- creates the stage root
- creates `<stage-root>/config`
- copies the bundled `phis-config.json` from the installed `phis`
- can take `--phis-path` when the `phis` wrapper is not in `PATH`
- creates empty `phis-runtime.json` when missing
- writes shared stage-config documentation to `<phis-host-config-dir>/stages/README.md`
- applies ownership `phis:phis` when that user and group exist
- updates the `phis-host` config even when the stage runtime already exists
- leaves existing stage files untouched

Defaults:

- stage: `prod`
- root:
  - `prod` -> `/srv/phis/stages/prod`
  - `test` -> `/srv/phis/stages/test`
  - `dev` -> `/srv/phis/stages/dev`

`stage init db` bootstraps PostgreSQL for one stage:

- requires `--admin-uri` for PostgreSQL bootstrap access
- derives:
  - database: `phis_<stage>`
  - user: `phis_<stage>_user`
- generates a strong database password unless `--db-password` is passed
- creates or updates the role
- creates the database when missing
- writes the resulting `database.uri` into `<stage-root>/config/phis-runtime.json`
- aborts when `database.uri` is already present unless `--force` is passed

Each site command calls the shared `phis` CLI through:

- `phis --root <stage-root> site config runtime --key <site-key> --json`

The stage defaults to `prod` unless `defaultStage` is set explicitly in the `phis-host` config.

Privilege model:

- `phis-host` should own the orchestration logic for host mutations
- if the current process is already privileged, it may perform those actions directly
- if privileges are missing, it should prefer returning the same `phis-host` command prefixed with `sudo`
- operators should not need to translate a failed `phis-host` action into lower-level `systemctl`, `nginx`, or `apache` commands by hand

Expected next steps:

1. Install Go locally.
2. Run `mkdir -p build && go build -o build/phis-host ./cmd/phis-host`.
3. Add write-side host commands for site init, service wiring, and deploy lifecycle.
