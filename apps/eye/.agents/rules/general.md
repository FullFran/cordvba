# General Rules — eye

## Runtime & toolchain

- Go 1.25+. Set in `go.mod`; do not downgrade.
- CGO disabled by default (`CGO_ENABLED=0`) for reproducible static binaries.
- All tooling invoked via `make` targets; never run raw `go` commands in CI steps unless the Makefile lacks the target.

## Code hygiene

- No global mutable state. Pass dependencies explicitly (constructor injection).
- No `init()` functions that produce side effects (network, disk, config reads).
- All exported symbols must have a doc comment.
- Error values must be wrapped with context: `fmt.Errorf("thing: %w", err)`.
- Never swallow errors silently; `_ = err` is forbidden outside generated code.

## Environment variables

- The single source of truth for config is `internal/config/config.go` (`Load` + `Validate`).
- Never call `os.Getenv` or `os.LookupEnv` outside the `config` package.
- Every new env var must be added to `config.go` AND `.env.example`.

## PRs

- Keep PRs under 400 changed lines. Split larger changes into stacked PRs.
- Conventional commits only (see `commits.md`).
- `make ci-local` must pass locally before opening a PR.

## Language

- All code, comments, identifiers, and commit messages are in **English**.
- Any Spanish in API responses must be neutral Spanish (tú/tienes/puedes); never Rioplatense.
