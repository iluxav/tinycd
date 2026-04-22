# tcd — TinyCD Compose Deployer

A tiny Go CLI that deploys private GitHub repos to a single host with Docker Compose and a shared Traefik reverse proxy.

See [CLAUDE.md](CLAUDE.md) for the product spec and [docs/plans/2026-04-22-tcd-design.md](docs/plans/2026-04-22-tcd-design.md) for the design.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/iluxa/tcd/main/install.sh | bash
```

Or pin a version:

```bash
TCD_VERSION=v0.1.0 curl -fsSL https://raw.githubusercontent.com/iluxa/tcd/main/install.sh | bash
```

## Usage

```bash
tcd init --domain example.com --acme-email you@example.com
# → prints an SSH deploy key; add it to GitHub per-repo

tcd deploy iluxa/app1 --port 3000              # ← serves at https://app1.example.com
tcd deploy iluxa/app2 --port 3001 --scale 2

tcd ls
tcd status app1
tcd logs app1 -f
tcd restart app1
tcd stop app1
tcd rm app1 --purge
```

## Build from source

```bash
make build        # → bin/tcd (current OS)
make build-all    # → cross-compiled binaries in bin/
```

## How it works

`tcd` maintains a single root compose project at `/var/lib/tcd/compose.yml` with Traefik as the first service. Each `tcd deploy` adds an `include:` entry pointing at the repo's own compose file plus a generated `override.yml` that injects Traefik labels, the shared `tcd-proxy` network, and the app's `.env` file — without mutating the repo's compose.

Apps designate their primary (scaled, public) service via the `tcd.primary=true` label in the repo's compose, or via the `--service` flag at deploy time.
