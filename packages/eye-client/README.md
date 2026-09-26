# eye-client

**Status:** implemented (issue #99).

## Usage

```python
from eye_client import EyeClient

client = EyeClient(base_url="http://127.0.0.1:8080", token="...", timeout=10.0)
records = client.records(source="metar-cordoba", topic="weather", limit=100)
entities = client.entities(topic="transit")
sources = client.sources()
```

`records()`, `entities()` and `sources()` raise `EyeAuthError` (401),
`EyeHTTPError` (other 4xx), `EyeServerError` (5xx) or `EyeTimeoutError` on a
timeout. The bearer token is never logged or included in `repr()`.

## Responsibility

A client for eye's HTTP contract, probably Python first since twin and
intelligence are its first consumers. Ideally generated from eye's OpenAPI
spec once it is stable.

## Must not

- Contain business logic — it is a thin client, not a use case.
- Read eye's SQLite store or raw cache directly, or import
  `apps/eye/internal`; it talks to eye's HTTP API only.

## May depend on

[`apps/eye`](../../apps/eye/README.md)'s public HTTP API and OpenAPI spec
only. See
[`docs/architecture/system-overview.md`](../../docs/architecture/system-overview.md).

## Data

None of its own.
