# Deploying eye

eye runs perfectly well as a command on a laptop. This document is for the
other case: leaving it running somewhere, with `eye daemon` collecting and
`eye serve` answering over HTTP.

Everything here assumes you have read [ADR-0007](adr/0007-ethical-boundary.md)
and [ADR-0008](adr/0008-undocumented-personal-sources.md). What a deployment
may expose is narrower than what your own machine may read, and the difference
is enforced in code rather than described here.

## The token

`eye serve` binds to loopback by default. On loopback, a token is ceremony:
the only thing that can reach the port is you. Bind it to a network and the
same store is offered to whoever finds the port, so:

**`eye serve --public` refuses to start without a token.** That is not a
setting.

Generate one:

```sh
openssl rand -base64 32
# or
head -c 32 /dev/urandom | base64
```

Give it to eye in one of two ways:

```sh
export EYE_API_TOKEN='…'                 # environment
eye serve --public --token-file /etc/eye/token   # or a file, read and trimmed
```

Prefer the file for anything unattended. An environment variable shows up in
`/proc/<pid>/environ`, in `systemctl show`, and in every crash report that
dumps the environment. `--token-file` beats `EYE_API_TOKEN` when both are set.

Clients then send it as a bearer token:

```sh
curl -H "Authorization: Bearer $EYE_API_TOKEN" http://127.0.0.1:8787/v1/stats
```

What stays open, always:

| Path | Why |
|---|---|
| `GET /health` | A probe has no credential. A health check that needs one reports the deployment down the moment the token rotates. |
| `GET /` | Says what the service is and what it serves. |
| `GET /openapi.json` | The same, for a machine. |
| `OPTIONS *` | A browser sends the preflight *before* it sends the token. Guarding it fails every cross-origin call before it is made. |

Everything under `/v1` needs the token, and answers `401
{"error":"unauthorized"}` without it.

## What a deployment may expose

The registry records, per source, whether eye may pass its data on. Three
rules are enforced in code:

1. **Records from undocumented personal sources are never served.** The filter
   lives in one place, not in each handler, so it cannot be forgotten in the
   next one.
2. **Camera images are never served.** There is no persistence path for them
   and no endpoint that returns one.
3. **Live bus arrivals are gated twice.** `/v1/transit/arrivals` reads
   `aucorsa-arrivals`, which is `access: undocumented_personal`. It answers
   `403 {"error":"personal source not served"}` unless **both**
   `EYE_ALLOW_PERSONAL_SOURCES` is truthy **and** `EYE_API_TOKEN` is set.

The second half of that gate is the point. The opt-in says this machine may
*read* the source. The token says the deployment is *private*. Reading an
undocumented endpoint for yourself is one thing; putting the result on the
open web is redistribution, and the licence does not permit it. Live arrivals
are also cached for 20 seconds in-process, so a browser on a refresh loop
cannot turn your deployment into a load generator against AUCORSA.

Everything else eye serves carries its own licence, per record, in
`provenance.license`. Several sources are `unspecified` — that is the
publisher's doing, not an omission here, and it means you are the one deciding
whether to pass it on. If you publish an eye deployment openly, you are
redistributing those records under terms nobody has written down. Read the
`license` field before you do.

## Docker

Two services over one store. They are split because they fail differently: a
provider that starts timing out must not take the API down with it, and
restarting the API must not lose a poll.

```sh
cp deploy/.env.example deploy/.env
$EDITOR deploy/.env                      # at minimum, set EYE_API_TOKEN
docker compose -f deploy/docker-compose.yml up -d
docker compose -f deploy/docker-compose.yml logs -f eye-api
```

The image is a multi-stage build ending in distroless: one static, CGO-free
binary, running as `nonroot`, with no shell and no package manager to pivot
to. The health check is eye itself — `eye serve --probe <url>` performs one
request and exits — because there is no `curl` in the image and adding one
would undo the reason it is distroless. Nothing secret is baked in; the token,
the API keys and the data directory are all supplied at run time.

Build it by hand if you prefer:

```sh
docker build -f deploy/Dockerfile -t eye:latest .
docker run --rm -p 127.0.0.1:8787:8787 \
  -e EYE_API_TOKEN="$EYE_API_TOKEN" -v eye-data:/var/lib/eye eye:latest
```

**The API is published on `127.0.0.1` only, on purpose.** Do not change that
line to `"8787:8787"` and consider it done: that publishes the port on every
interface *and* punches a hole through the host firewall, because Docker
writes its own iptables rules ahead of ufw's. Put a reverse proxy in front of
the loopback port instead.

## systemd

For a host without Docker. Two units, the same split.

```sh
sudo install -m 0755 -D bin/eye /usr/local/bin/eye
sudo install -m 0644 -D configs/sources.yaml /etc/eye/sources.yaml
sudo install -m 0644 deploy/eye-daemon.service deploy/eye-api.service /etc/systemd/system/

sudo install -m 0600 -D /dev/null /etc/eye/api.env
echo "EYE_API_TOKEN=$(openssl rand -base64 32)" | sudo tee /etc/eye/api.env >/dev/null

sudo install -m 0600 -D /dev/null /etc/eye/daemon.env   # AEMET_API_KEY, FIRMS_MAP_KEY, EYE_EXTRA_CA_FILE

sudo systemctl daemon-reload
sudo systemctl enable --now eye-daemon eye-api
systemctl status eye-api
journalctl -u eye-daemon -f
```

Both units use `DynamicUser=yes`, so there is no account to create and nothing
left behind when you remove them. They share the store through
`StateDirectory=eye`, which resolves to `/var/lib/eye` owned by the transient
user. Only the daemon writes to it.

The sandbox is not decoration: `ProtectSystem=strict` and `ProtectHome=yes`
make the whole filesystem read-only apart from the state directory,
`NoNewPrivileges=yes` and an empty `CapabilityBoundingSet` mean the process
cannot acquire anything it was not started with, and
`RestrictAddressFamilies=AF_INET AF_INET6` leaves it able to make outbound
HTTP and nothing else. eye needs no privileges and must never ask for any.

Secrets go in `EnvironmentFile=`, never in `Environment=` lines: a unit file
is world-readable and `systemctl show` prints it back to anyone who asks. The
leading `-` on the path means an absent file does not block startup — and if
the token file is missing, `eye serve --public` refuses, which is the correct
failure.

## Servers with an incomplete certificate chain

One source in the registry needs a word of setup, and it is worth understanding
rather than copying.

MITECO's air-quality host serves a certificate that is valid and rooted in a CA
your system already trusts — but it omits the FNMT-RCM intermediate that links
the two. A browser hides this by fetching the missing certificate from the
address in the certificate's AIA extension. Go does not do that, by design, so
`eye` fails the handshake with `certificate signed by unknown authority`
against a perfectly good server.

`EYE_EXTRA_CA_FILE` points at a PEM file whose authorities are added to the
system pool:

```sh
curl -o /tmp/accomp.crt http://www.cert.fnmt.es/certs/ACCOMP.crt
openssl x509 -inform DER -in /tmp/accomp.crt -out /etc/eye/fnmt-intermediate.pem
# then in the daemon's environment file:
EYE_EXTRA_CA_FILE=/etc/eye/fnmt-intermediate.pem
```

Supplying a missing intermediate **completes** a chain. That is the opposite of
skipping verification, which `eye` has no option for and will not grow: every
certificate is still checked, against a pool you widened on purpose, one named
file at a time. A path that cannot be read, or that holds no certificate, stops
the command rather than quietly falling back — a typo here would otherwise
become a source that fails at TLS for a reason nobody would think to look for.

Installing the intermediate system-wide works equally well, if you would rather
fix it once for every program on the machine.

## TLS and a public name

`deploy/Caddyfile.example` terminates TLS and proxies to the loopback port.
Point the DNS name at the host first; Caddy obtains the certificate itself.

Any proxy works — the shape is the same. What matters:

- The proxy **does not authenticate anything**. `EYE_API_TOKEN` does. TLS in
  front of an open API encrypts the theft; it does not stop it.
- Do not log request headers. The token travels in `Authorization`.
- Keep eye on loopback and let only the proxy reach it.
- Rate-limit `/v1`. A published deployment gets scanned within the hour.
  Leave `/health` alone so a monitor can poll it.

## Checking it works

```sh
curl -s http://127.0.0.1:8787/health | jq .
curl -s -H "Authorization: Bearer $EYE_API_TOKEN" http://127.0.0.1:8787/v1 | jq .
curl -s -H "Authorization: Bearer $EYE_API_TOKEN" http://127.0.0.1:8787/v1/stats | jq .
curl -s -H "Authorization: Bearer $EYE_API_TOKEN" http://127.0.0.1:8787/v1/sources | jq '.sources[] | {id, last_success, consecutive_errors}'
curl -s http://127.0.0.1:8787/openapi.json | jq '.paths | keys'
```

`/v1/sources` is the one worth watching. It carries stored polling health next
to each registry entry, so "eye is allowed to read this" and "eye has actually
managed to read this" are two visibly different facts. A source with a rising
`consecutive_errors` and a stale `last_success` has broken, whatever the
registry says.
