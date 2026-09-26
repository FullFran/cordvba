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

## What to collect and retain

eye's own registry (`apps/eye/configs/sources.yaml`) ships with every source
the project knows about, including several this page has no use for.
Measured on this VPS over about three hours: `dgt-vms` and `dgt-incidents`
alone wrote 7,337 records and grew the raw cache by roughly 200 MB, while the
only data the v0.1 page actually reads (METAR and MITECO ICA) is under 200
records a day.

eye reads an optional per-deployment overrides file
(`EYE_OVERRIDES_FILE`, see `apps/eye/AGENTS.md` and issue #125) that narrows
what it collects and how long it keeps it, without forking the registry.
Overrides can only restrict what the registry already permits: they cannot
switch on a source held for `review_terms` or `manual_link`, and they cannot
enable an undocumented personal source without the machine also setting
`EYE_ALLOW_PERSONAL_SOURCES` — see ADR-0008.

The profile this VPS runs, keeping only what the cordvba v0.1 page uses:

```yaml
# apps/eye/deploy/overrides.yaml — see apps/eye/deploy/overrides.example.yaml
sources:
  only: [metar-cordoba, miteco-ica, aemet-warnings, aemet-observation]
retention:
  metar-cordoba: historical
  miteco-ica: historical
```

To use it:

1. Copy `apps/eye/deploy/overrides.example.yaml` to
   `apps/eye/deploy/overrides.yaml` on the host and edit it. It is operator
   configuration, not registry content, and must never be committed.
2. Uncomment the bind mount and `EYE_OVERRIDES_FILE` lines in
   `apps/eye/deploy/docker-compose.yml` and `apps/eye/deploy/.env` (both
   already carry the commented-out example).
3. Restart eye:

   ```bash
   docker compose -p cordvba-eye -f apps/eye/deploy/docker-compose.yml up -d
   docker compose -p cordvba-eye -f apps/eye/deploy/docker-compose.yml logs eye-daemon
   ```

   Startup logs one `"effective set"` line naming what was collected, what
   the overrides disabled, and each collected source's retention. `eye
   sources` and `/v1/sources` also report an operator-excluded source as
   `disabled (operator)`, distinct from one the registry itself holds.

Confirm the effect over a day by comparing the daemon's periodic `"retention
enforced"` log line (`store_bytes`, `raw_cache_bytes`) before and after
applying the profile, or by diffing two `eye sources --json` runs.

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
- `PUBLIC_ORIGIN` — the api's CORS allow-list (`WEB_ORIGIN`, see
  `apps/api/README.md`): the `https://` origin this stack will be reachable
  at once Funnel is on (see below), plus, since the page is now also
  published on GitHub Pages (issue #115), the Pages origin,
  `https://www.fullfran.com` — comma-separated, e.g.
  `https://<machine>.<tailnet>.ts.net,https://www.fullfran.com`. A single
  origin still works exactly as before.
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

## Public host name through Traefik (recommended)

Tailscale Funnel's host name resolves to a private tailnet address on devices
that belong to the tailnet, and browsers block a public page (GitHub Pages)
from calling a private address. Publishing the edge on a public host name
through the host's Traefik works for every visitor (issue #120).

Prerequisites: a Traefik that watches Docker labels on `dokploy-network` (the
Dokploy Traefik does), a Let's Encrypt resolver named `letsencrypt`, and a DNS
record for the host name pointing at the server (a DNS-only wildcard such as
`*.<your-domain>` is enough).

1. In `infra/compose/.env` set `CORDVBA_PUBLIC_HOST=cordvba.<your-domain>` and
   add `https://cordvba.<your-domain>` to `PUBLIC_ORIGIN` if the page is also
   opened from that host.
2. Start the stack with the overlay:

   ```bash
   docker compose -p cordvba-mvp -f infra/compose/compose.yml \
     -f infra/compose/compose.traefik.yml up -d
   ```

3. Check `curl -sI https://cordvba.<your-domain>/api/health` returns 200 once
   Traefik has obtained the certificate (usually under a minute).
4. Point the Pages build at it: set the repository variable
   `CORDVBA_PUBLIC_API_BASE_URL=https://cordvba.<your-domain>/api` and re-run
   the Pages workflow.

Entry points, resolver, redirect middleware and network can be overridden with
`TRAEFIK_HTTPS_ENTRYPOINT`, `TRAEFIK_HTTP_ENTRYPOINT`, `TRAEFIK_CERT_RESOLVER`,
`TRAEFIK_REDIRECT_MIDDLEWARE` and `TRAEFIK_NETWORK`. Funnel can stay on as a
secondary entry point or be turned off with `tailscale funnel --https=443 off`.
