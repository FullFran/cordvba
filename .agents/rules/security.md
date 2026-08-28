# Security Rules — eye

## Secrets

- Secrets (API keys, DB passwords, JWT signing keys) live in environment variables only.
- Never commit `.env`. `.env.example` contains only placeholder values.
- `gitleaks` runs in CI; any detected secret blocks the pipeline.

## Dependency hygiene

- `govulncheck ./...` runs in CI. Vulnerabilities at severity HIGH or CRITICAL block merge.
- Pin indirect dependencies via `go.sum`. Never use `replace` directives in production modules.
- Review `go mod tidy` output before each PR; unexpected new deps need a justification comment.

## Input validation

- Never trust user input. Validate all path params, query params, and request bodies before passing to the domain layer.
- Use `net/http` timeouts (already configured in `main.go`): ReadTimeout, WriteTimeout, IdleTimeout, ReadHeaderTimeout.
- Sanitize error messages returned to callers — never leak internal stack traces or SQL errors.

## Credentials for public sources

- API keys (`AEMET_API_KEY`, `FIRMS_MAP_KEY`) are read only in `internal/config`.
- Never log a key, a full payload, or a personal identifier.
- Never authenticate with someone else's credentials, and never bypass an
  access control. A source behind authentication we do not own is not a source.

## Outbound requests

- One shared `*http.Client` with an explicit timeout. Never `http.DefaultClient`.
- Cap every response body with `io.LimitReader`; a public endpoint can return
  more than you expect.
- Honour `Retry-After`, and back off on `429` and `5xx`.
- Never call an endpoint that is not declared in `configs/sources.yaml`.

## Runtime

- eye ships as a static, CGO-free binary. There is no image to harden.
- It needs no elevated privileges and must never ask for any.
- `trivy fs` scans the repository in CI. HIGH/CRITICAL CVEs block the pipeline.

## CI security tier

| Tool         | Stage       | Blocking |
| ------------ | ----------- | -------- |
| `gitleaks`   | pre-merge   | yes      |
| `govulncheck`| pre-merge   | yes (H/C)|
| `trivy fs`   | pre-merge   | yes (H/C)|
| `go vet`     | lint stage  | yes      |
