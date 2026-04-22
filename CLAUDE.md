# PRD: tcd — TinyCD Compose Deployer

## Goal

Build a small server-side CLI called `tcd` that deploys private GitHub repos to a single host using Docker Compose. It should be simple, deterministic, and reusable across servers.

## Product Scope

`tcd` is a thin controller over:

- Git clone/pull via SSH
- Docker build
- Docker Compose lifecycle
- Shared reverse proxy for subdomain routing

## Core UX

```bash
tcd init --domain example.com
tcd deploy iluxa/app1 --scale 2 --port 3000
tcd deploy iluxa/app2 --scale 2 --port 3001
tcd restart app1
tcd stop app1
tcd rm app1 --purge
tcd ls
tcd status app1
tcd logs app1
```

## Expected Behavior

### tcd init

- Verify Docker and docker compose are installed
- Create config/state directories
- Generate SSH key if missing
- Print public key for user to add to GitHub
- Store base domain

### tcd deploy <repo>

- Normalize repo to SSH URL
- Derive app name (or use --name)
- Clone or pull repo
- Checkout ref if provided
- Persist env vars
- Detect compose file (compose.yml / docker-compose.yml)
- Fallback: generate simple compose if missing
- Run docker compose up -d --build
- Apply scaling
- Register app with reverse proxy

### Lifecycle

- restart → docker compose restart
- stop → docker compose stop
- rm → docker compose down
- rm --purge → also delete repo + state

### tcd ls

Show:

- name
- repo
- ref
- url
- scale
- status

### tcd status

Show:

- repo
- paths
- commit
- containers
- URL

### tcd logs

- docker compose logs

## Routing Model

- Subdomain-based routing
- app1.example.com, app2.example.com
- One shared reverse proxy (Traefik recommended)
- Apps are internal only (no direct port binding)

## Scaling

- Use --scale
- No host port binding on scaled services
- Internal port from --port or PORT env
- Proxy handles load balancing

## File Layout

~/.config/tcd/
config.yml
id_ed25519

/var/lib/tcd/apps/<app>/
repo/
.env
compose.generated.yml
state.json

## State

Track:

- name
- repo
- ref
- commit
- env file
- compose file
- port
- scale
- URL

## Implementation

- Language: Go
- Use git, docker, docker compose via shell
- Idempotent commands
- Clear error handling
- No hidden magic

## Conventions

- app name = repo name
- URL = <app>.<domain>
- scale default = 1
- prefer repo compose over generated

## Deliverables

Implement:

- init
- deploy
- restart
- stop
- rm (--purge)
- ls
- status
- logs

## IMPORTANT

- build with go lang
- don't commit to git
