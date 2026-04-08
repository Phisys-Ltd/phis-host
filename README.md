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

## Local Development

This repository is initialized as a standalone Go project.

Expected next steps:

1. Install Go locally.
2. Run `go build ./cmd/phis-host`.
3. Add the first command groups for site init and service wiring.
