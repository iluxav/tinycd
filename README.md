# tcd — TinyCD Compose Deployer

A tiny Go CLI that deploys private GitHub repos to a single host with Docker Compose and a shared Traefik reverse proxy.

See [CLAUDE.md](CLAUDE.md) for the product spec and [docs/plans/2026-04-22-tcd-design.md](docs/plans/2026-04-22-tcd-design.md) for the design.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/iluxav/tinycd/main/install.sh | bash
```

Or pin a version:

```bash
TCD_VERSION=v0.1.0 curl -fsSL https://raw.githubusercontent.com/iluxav/tinycd/main/install.sh | bash
```

## Usage

```bash
# one-time per host — generates an SSH deploy key, prints it, and bootstraps Traefik
tcd init --domain example.com --acme-email you@example.com

# deploy a repo — served at https://<app>.<domain>
tcd deploy iluxav/app1 --port 3000
tcd deploy iluxav/app2 --port 3001 --scale 2

# lifecycle
tcd ls
tcd status app1
tcd logs app1 -f
tcd restart app1
tcd stop app1
tcd rm app1 --purge
```

Add the printed key at `https://github.com/<owner>/<repo>/settings/keys` (per-repo deploy key) or at your account-level SSH keys for broader access.

## Routing: public domains and aliases

By default an app is reachable at `<app>.<domain>`. If the host is also behind a tunnel (e.g. [etunl](https://github.com/iluxav/ntunl)) or answers to multiple public domains, you have two knobs:

**`public_domains` (set once, auto-applies to every deploy):**

```bash
tcd init --domain localhost --public-domain etunl.com --public-domain foo.example.com
```

Every deploy now adds `<app>.etunl.com` and `<app>.foo.example.com` to Traefik's router rule automatically — no `--alias` needed.

**`--alias` (per-deploy, for one-off hostnames):**

```bash
tcd deploy iluxav/myapp --port 3000 --alias www.example.com --alias api.example.com
```

Both stack — Traefik ends up with `Host(<app>.<domain>) || Host(<app>.<pd1>) || … || Host(<alias1>) || …`.

## Behind a tunnel (etunl)

To expose a tcd deployment through an etunl tunnel, add the etunl server domain to `public_domains` once, then point an etunl route at Traefik:

```bash
# on the tcd host
tcd init --domain localhost --public-domain etunl.com
tcd deploy iluxav/hdrezka --port 3000
#   → Traefik matches Host(`hdrezka.localhost`) || Host(`hdrezka.etunl.com`)

etunl add --name hdrezka --type http --target localhost:80
#   → https://hdrezka.etunl.com reaches Traefik, which routes to the container
```

`tcd init` prints a hint when it detects `~/.etunl/config.yaml` but the etunl server isn't in `public_domains` yet.

## Build from source

```bash
make build        # → bin/tcd (current OS)
make build-all    # → cross-compiled binaries in bin/
make test
```

## How it works

`tcd` maintains a single root compose project at `/var/lib/tcd/compose.yml` (or `~/.local/share/tcd/compose.yml` if `/var/lib` isn't writable) with Traefik as the first service. Each `tcd deploy` adds an `include:` entry pointing at the repo's own compose file plus a generated `override.yml` that injects Traefik labels, the shared `tcd-proxy` network, and the app's `.env` file — without mutating the repo's compose.

Apps designate their primary (scaled, public) service via the `tcd.primary=true` label in the repo's compose, via the `--service` flag, or by being the first service declared (in that order).

### File layout

```
~/.config/tcd/
  config.yml              # { domain, public_domains, acme_email, apps_dir, state_dir }
  id_ed25519, id_ed25519.pub

<state-dir>/               # /var/lib/tcd or ~/.local/share/tcd
  compose.yml             # root: traefik + include: list
  traefik/acme.json       # Let's Encrypt state
  apps/<app>/
    repo/                 # git clone
    .env                  # from --env-file
    override.yml          # generated
    state.json
```
