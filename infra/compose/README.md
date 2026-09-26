# compose

**Status:** MVP stack — twin, api and web behind one edge.

## Responsibility

`compose.yml` (project name `cordvba-mvp`) brings up twin, api, web and a
Caddy edge through Docker Compose (`make up` / `make down`, or
`docker compose -p cordvba-mvp -f infra/compose/compose.yml ...` directly).

- **twin** and **api** join both this project's own network and eye's
  external one (named by `EYE_NETWORK`, see `.env.example`), so they can
  reach `eye-api` by service name. eye itself is not a service here — it
  is already running as its own compose project — and is never rebuilt or
  restarted by this file.
- **web** serves the built static site; it never talks to eye or twin
  directly, only through api.
- **edge** (`Caddyfile`, alongside this README) is the *only* service that
  publishes a port, and only on `127.0.0.1:${EDGE_PORT}`. It routes
  `/api/*` to api (prefix stripped) and everything else to web. Publishing
  that loopback port to the internet is done outside this stack, with
  Tailscale Funnel — see [`../deployment/README.md`](../deployment/README.md).

eye and intelligence still have no service entry here: eye runs
separately (see above), and intelligence has no code yet — entries are
added only when a component has something to run.

## Must not

- Contain application or domain code.
- Define empty service containers for components that do not exist yet.
- Publish any port other than the edge's.

## May depend on

Each component's own Dockerfile/build output (`services/twin/Dockerfile`,
`apps/api/Dockerfile`, `apps/web/Dockerfile`) and eye's already-running
compose project for the external network. See
[`docs/architecture/system-overview.md`](../../docs/architecture/system-overview.md).

## Data

None of its own. `caddy-data` is a named volume for the edge's `/data`
(autosave state); the stack's actual data lives in eye's own volume,
untouched by this project.
