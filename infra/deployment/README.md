# deployment

**Status:** MVP runbook — first deploy, publish, update, rollback.

## Responsibility

How the cordvba v0.1 MVP stack (`infra/compose/compose.yml`, project name
`cordvba-mvp`) runs on the VPS, next to eye, and how an operator brings it
up, publishes it, updates it and rolls it back.

## Must not

- Contain application or domain code.
- Duplicate a component's own build logic — it orchestrates and references
  it, following the same principle as the root
  [`Makefile`](../../Makefile).

## May depend on

Each component's build artefacts, `infra/compose/compose.yml`, and eye's
already-running compose project. See
[`docs/architecture/system-overview.md`](../../docs/architecture/system-overview.md).

## Data

None. eye's data volume is eye's own and this stack never touches it (see
boundary rules in the root [`AGENTS.md`](../../AGENTS.md)).

## Prerequisites

- eye is already running on this host as its own compose project
  (`docker compose -p cordvba-eye ...`, container names `eye-daemon` and
  `eye-api` — see `apps/eye/deploy/docker-compose.yml`). This stack does
  not start, stop or rebuild it.
- Docker and the Docker Compose plugin.
- [Tailscale](https://tailscale.com) installed and logged into the
  tailnet that will publish this stack (`tailscale up`), with Funnel
  enabled for the tailnet in the admin console.

## First deploy

```bash
cd infra/compose
cp .env.example .env
```

Edit `.env`:

- `EYE_API_TOKEN` — copy the exact value from eye's own host-side
  `.env` (`apps/eye/deploy/.env` on this host). Do not print it to a
  shell you don't control, and never commit it.
- `EYE_NETWORK` — leave as `cordvba-eye_default` unless eye was deployed
  under a different compose project name.
- `PUBLIC_ORIGIN` — the `https://` origin this stack will be reachable at
  once Funnel is on (see below).
- `EDGE_PORT` — leave as `8088` unless that loopback port is already taken
  on this host.

Then, from the repository root:

```bash
docker compose -p cordvba-mvp -f infra/compose/compose.yml up -d --build
```

Confirm only the edge is published:

```bash
docker compose -p cordvba-mvp -f infra/compose/compose.yml ps
```

## Publish with Tailscale Funnel

Funnel terminates public TLS itself and forwards to the loopback port the
edge published (`EDGE_PORT`, default `8088`); the edge inside the stack
stays plain HTTP.

```bash
tailscale funnel --bg --https=443 http://127.0.0.1:8088
```

If port 443 is already used by something else on this host, fall back to:

```bash
tailscale funnel --bg --https=8443 http://127.0.0.1:8088
```

Check what's currently published:

```bash
tailscale funnel status
```

Turn it off (the stack keeps running locally; only the public path is
removed):

```bash
tailscale funnel --https=443 off
```

(match the port you actually funneled, e.g. `--https=8443 off`.)

## Update

```bash
git pull
docker compose -p cordvba-mvp -f infra/compose/compose.yml up -d --build
```

Compose rebuilds only the images whose build context changed and restarts
just those services; eye is untouched, and Funnel keeps pointing at the
same loopback port throughout, so there's no publish/unpublish step.

## Rollback to the previous image

Compose builds tag images as `<project>-<service>`, and a plain rebuild
overwrites that tag — so keep the previous one before updating, and go
back to it if the update misbehaves:

```bash
# before the update:
docker tag cordvba-mvp-api cordvba-mvp-api:previous
docker tag cordvba-mvp-web cordvba-mvp-web:previous
docker tag cordvba-mvp-twin cordvba-mvp-twin:previous

# to roll back a service (api shown; same for web/twin):
docker tag cordvba-mvp-api:previous cordvba-mvp-api
docker compose -p cordvba-mvp -f infra/compose/compose.yml up -d --no-build api
```

## Logs

```bash
docker compose -p cordvba-mvp -f infra/compose/compose.yml logs -f [service]
```

Every service logs to stdout with the `json-file` driver, rotated at 10 MB
× 3 files, matching eye's own compose file.

## Moving into Dokploy later

The alternative considered in issue #103 and deferred: publishing through
Dokploy needs a Dokploy API key (or manual UI work) and its GitHub
connection for this private repository. When that's set up, `compose.yml`
is the starting point — Dokploy can import a compose-based app directly;
the external `eye` network reference and the loopback-only edge port stay
as they are, and Dokploy's own reverse proxy replaces Tailscale Funnel as
the public entry point (the `edge` service can then either keep fronting
web/api, or be dropped in favor of Dokploy's proxy talking to web and api
directly, once its routing supports the `/api` path split this Caddyfile
does).
