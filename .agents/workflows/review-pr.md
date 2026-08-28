# Workflow: Review PR — eye

## Checklist

### Architecture
- [ ] Layer boundaries respected: `domain/` imports stdlib only; no `net/http` or storage drivers in `application/`.
- [ ] Dependency direction: `infrastructure` → `application` → `domain` (never reversed).
- [ ] No global state added (no package-level vars that mutate after init).

### Error handling
- [ ] All errors wrapped with `fmt.Errorf("context: %w", err)`.
- [ ] No `_ = err` outside generated/test code.
- [ ] HTTP handlers translate domain errors to status codes — no raw error strings in 4xx/5xx.

### Context
- [ ] All I/O functions accept `ctx context.Context` as first param.
- [ ] Context propagated to all downstream calls (DB, HTTP, external).

### Testing
- [ ] New behavior has a failing test written BEFORE the implementation (confirm by reading commit order or asking author).
- [ ] Table-driven format used for multiple cases.
- [ ] `-race` flag used (covered by `make test`).
- [ ] Coverage ≥80% after the change.

### Security
- [ ] No secrets or env vars read outside `internal/config/`.
- [ ] SQL uses parameterized queries only.
- [ ] `govulncheck` clean (CI enforces this).

### Style
- [ ] `gofmt` applied (CI enforces).
- [ ] No `//nolint` without explanation.
- [ ] Exported symbols have doc comments.

### Size
- [ ] PR ≤400 changed lines. If larger, request a split.
