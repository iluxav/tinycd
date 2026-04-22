# tcd — TinyCD Compose Deployer — Design

**Date:** 2026-04-22
**Source PRD:** `CLAUDE.md`

## Summary

`tcd` is a Go CLI that deploys private GitHub repos to a single host using Docker Compose, with a shared Traefik reverse proxy for subdomain routing. Apps live as `include:`-referenced services inside one root compose project; each app gets a generated override file that injects Traefik labels, the shared network, and the `.env` file — without mutating the repo's own compose.

## Decisions

| Area               | Choice                                                                        | Rationale                                            |
| ------------------ | ----------------------------------------------------------------------------- | ---------------------------------------------------- |
| Compose topology   | Root compose with `include:` list                                             | Unified `docker compose up`, repo compose preserved  |
| Proxy              | Traefik v3 bootstrapped by `tcd init`                                         | Self-contained, "turn-key" per PRD                   |
| Primary service    | `tcd.primary=true` label → `--service` flag → first service                   | Works for tcd-aware repos and repos-we-don't-control |
| Override mechanism | Generated `override.yml` merged via `include: path: [repo.yml, override.yml]` | Zero mutation of repo files                          |
| Env vars           | `--env-file` only, copied once, preserved on re-deploy                        | Clean, matches `docker compose` idiom                |
| Git ref            | `--ref` flag, defaults to repo default branch                                 | Explicit reproducibility                             |
| CLI framework      | `spf13/cobra`                                                                 | De facto standard for Go CLIs                        |
| Module path        | `github.com/iluxa/tinycd`                                                     |                                                      |
| Build              | `Makefile` with `build` + `build-linux` targets                               |                                                      |
| `tcd logs`         | One-shot dump; `-f` flag to follow                                            | Matches `docker compose logs` default                |
| Distribution       | GitHub Actions release → cross-compiled binaries + `install.sh`               | Curl-installable                                     |

## File Layout

```
~/.config/tcd/
  config.yml              # { domain, acme_email, apps_dir, state_dir }
  id_ed25519, id_ed25519.pub

/var/lib/tcd/
  compose.yml             # root: traefik + include: list
  traefik/
    acme.json             # chmod 600
  apps/<app>/
    repo/                 # git clone
    .env                  # from --env-file
    override.yml          # generated
    state.json
```

## Root compose.yml

```yaml
services:
  traefik:
    image: traefik:v3
    restart: unless-stopped
    command:
      - --providers.docker=true
      - --providers.docker.exposedbydefault=false
      - --entrypoints.web.address=:80
      - --entrypoints.websecure.address=:443
      - --certificatesresolvers.le.acme.email=${ACME_EMAIL}
      - --certificatesresolvers.le.acme.storage=/acme.json
      - --certificatesresolvers.le.acme.httpchallenge=true
      - --certificatesresolvers.le.acme.httpchallenge.entrypoint=web
    ports: ["80:80", "443:443"]
    networks: [tcd-proxy]
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - ./traefik/acme.json:/acme.json

networks:
  tcd-proxy:
    name: tcd-proxy

include:
  # managed by tcd — do not edit
  - path: [apps/<app>/repo/compose.yml, apps/<app>/override.yml]
```

## Deploy flow (`tcd deploy <repo>`)

1. Normalize repo → SSH URL (`iluxa/app1` → `git@github.com:iluxa/app1.git`).
2. Derive app name from repo basename (unless `--name`).
3. Clone into `apps/<app>/repo/` if missing, else `git fetch && git checkout <ref> && git reset --hard origin/<ref>`. Record commit.
4. If `--env-file F`: copy `F` → `apps/<app>/.env`. Else preserve existing.
5. Detect `compose.yml` / `docker-compose.yml` in repo. If missing, generate `compose.generated.yml` using repo's Dockerfile (service named `<app>`, expose `--port`).
6. Resolve primary service: `tcd.primary=true` label → `--service` flag → first service. Save in state.
7. Write `apps/<app>/override.yml` with `env_file`, `networks: [tcd-proxy]`, and Traefik labels:
   - `traefik.enable=true`
   - `traefik.http.routers.<app>.rule=Host(` `<app>.<domain>` `)`
   - `traefik.http.routers.<app>.entrypoints=websecure`
   - `traefik.http.routers.<app>.tls.certresolver=le`
   - `traefik.http.services.<app>.loadbalancer.server.port=<port>`
8. Update root `compose.yml` `include:` list to reference this app's compose + override (additive + idempotent).
9. Run `docker compose -f /var/lib/tcd/compose.yml -p tcd up -d --build --scale <primary>=<scale>`.
10. Write `apps/<app>/state.json`.

## Error model

- Every shell-out wrapped; stderr streamed and surfaced verbatim on failure.
- Non-zero exit with one-line summary on failure.
- Idempotent: re-running with same args is a no-op past `git fetch`.
- Partial failure leaves `state.json` un-updated so next run retries cleanly.

## Lifecycle

- `restart/stop` → `docker compose -p tcd restart|stop <primary>` (multi-service apps: all services listed in the app's compose).
- `rm <app>` → remove include entry from root compose, `docker compose -p tcd up -d` (tears down removed services).
- `rm --purge` → also `rm -rf apps/<app>/`.
- `ls` → join every `state.json` with `docker compose ps --format json` output.
- `status <app>` → state + `docker compose ps <primary>` + last commit.
- `logs <app>` → `docker compose -p tcd logs --tail=200 [-f] <primary>`.

## Testing

- Unit (default): `NormalizeRepoURL`, compose parsing, `ResolvePrimaryService`, `RenderOverride`, state marshalling.
- Integration (gated, `TCD_INTEGRATION=1`): deploy a tiny hello-world repo and verify `tcd ls` output.

## Distribution

- `.github/workflows/release.yml` triggered on `v*` tag push.
- Matrix: `linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64`.
- Artifacts: `tcd_<version>_<os>_<arch>.tar.gz` (+ `.zip` for Windows) with `SHA256SUMS`.
- `install.sh`: detects OS/arch, resolves latest (or `$TCD_VERSION`), downloads, verifies checksum, installs to `/usr/local/bin/tcd`
