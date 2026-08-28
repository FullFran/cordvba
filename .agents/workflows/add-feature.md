# Workflow: Add Feature — eye

Strict TDD. Do not skip steps.

## Steps

1. **Create domain types** (`internal/<domain>/domain/`)
   - Define entity structs and value objects.
   - Define repository interface.
   - Define domain error types (`type ErrNotFound struct{…}`).

2. **Write failing application-layer test** (`internal/<domain>/application/<service>_test.go`)
   - Use a fake (in-process) repository implementing the domain interface.
   - Assert the expected behavior. Run `go test -race ./internal/<domain>/...` — it must FAIL.

3. **Implement application service** (`internal/<domain>/application/<service>.go`)
   - Minimum code to make the test pass.
   - Run `go test -race ./internal/<domain>/...` — it must PASS.

4. **Write failing handler test** (`internal/<domain>/infrastructure/<handler>_test.go`)
   - Use `httptest.NewRecorder` + `httptest.NewRequest`.
   - Inject a fake application service.
   - Assert HTTP status and response body. Run — must FAIL.

5. **Implement HTTP handler** (`internal/<domain>/infrastructure/<handler>.go`)
   - Parse request → call service → serialize response.
   - No business logic. Run — must PASS.

6. **Implement repository** (`internal/<domain>/infrastructure/<repo>.go`)
   - SQL via the SQLite adapter. Parameterized queries only.
   - Write integration test tagged `//go:build integration`.

7. **Wire in router** (`cmd/eye/main.go` → `buildRouter`)
   - Register the new handler under `/api/v1/<domain>`.

8. **Update config if needed**
   - New env vars → `internal/config/config.go` + `.env.example`.

9. **Run full CI locally**
   ```
   make ci-local
   ```
   Must pass with 0 lint errors and ≥80% coverage.

10. **Commit** — one commit per logical unit, conventional format (see `commits.md`).

11. **Open PR** — ≤400 changed lines. If larger, split into stacked PRs.
