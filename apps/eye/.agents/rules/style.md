# Style Rules — eye

## Formatting

- `gofmt` is the canonical formatter. No configuration, no exceptions.
- Run `make fmt` before every commit (it runs `gofmt -l -w .`).
- Line length is not enforced by gofmt; keep lines ≤120 chars as a human guideline.

## Linter

- `golangci-lint run ./...` (via `make lint`).
- Active linters (see `.golangci.yml` when present): `govet`, `errcheck`, `staticcheck`, `gosimple`, `unused`, `revive`, `gocyclo`, `misspell`.
- Never add `//nolint` without a comment explaining why.

## Naming

| Construct      | Convention                           | Example                        |
| -------------- | ------------------------------------ | ------------------------------ |
| Package        | lowercase, single word               | `application`, `domain`        |
| Exported type  | PascalCase                           | `HealthService`                |
| Unexported     | camelCase                            | `healthChecker`                |
| Interface      | noun or `<Verb>er`                   | `Checker`, `UserRepository`    |
| Constructor    | `New<Type>`                          | `NewHealthService`             |
| Error type     | `Err<Name>`                          | `ErrNotFound`                  |
| Test func      | `Test<Subject>_<scenario>`           | `TestLoad_missingDatabaseURL`  |

## File layout (within a package)

1. Package doc comment + `package` declaration
2. Imports (stdlib / external / internal — goimports groups them)
3. Constants and vars (unexported first, then exported)
4. Types
5. Constructor functions
6. Methods (exported first, then unexported helpers)

## Imports

- Use `goimports` grouping: stdlib | third-party | internal.
- Alias imports only when there is a genuine name conflict; document the alias with a comment.

## Comments

- Public symbols: full sentence, starts with the symbol name.
- Inline comments: explain *why*, not *what*.
- TODO format: `// TODO(owner): description` — always include an owner.

## Miscellaneous

- Prefer short variable names in short scopes (`i`, `err`, `r`, `w`); use descriptive names in wider scopes.
- Avoid `else` after a `return` or `continue` — reduces nesting.
- `defer` for cleanup (file close, DB rows close, mutex unlock) — document any non-obvious defer ordering.
