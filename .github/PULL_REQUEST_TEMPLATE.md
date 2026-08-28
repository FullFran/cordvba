## Summary

<!-- What does this PR do and why is it needed? 1–3 sentences. -->

Closes #<!-- issue number (required — every PR must reference an approved issue) -->

## Type of Change

<!-- Apply exactly ONE type:* label to this PR before requesting review. -->

- [ ] `type:feat` — new feature
- [ ] `type:fix` — bug fix
- [ ] `type:docs` — documentation
- [ ] `type:refactor` — internal restructuring
- [ ] `type:chore` — maintenance / tooling
- [ ] `type:test` — test coverage
- [ ] `type:ci` — CI/CD pipeline
- [ ] `type:build` — build system

## PR Size Budget

> **Policy:** PRs must stay under **400 changed lines** (excluding generated files).
> Above the budget: split into chained PRs (domain → application → infrastructure), or add a `size:exception` label with a justification comment.

- [ ] Under 400 changed lines, OR
- [ ] `size:exception` label added — justification: <!-- reason here -->

## Quality Gate (`make ci-local`)

> Run `make ci-local` and paste the result (or confirm it passed).

- [ ] `gofmt` passes (no formatting diffs)
- [ ] `go vet ./...` passes
- [ ] `golangci-lint run ./...` passes
- [ ] `go test -race -cover ./...` passes (all tests green)
- [ ] `go build ./...` succeeds

## Architecture Checklist

- [ ] `domain/` package imports the standard library only
- [ ] `os.Getenv` is only called in `internal/config/config.go`
- [ ] All errors are wrapped with `fmt.Errorf("context: %w", err)`
- [ ] `context.Context` is propagated through all I/O functions

## Testing

- [ ] Tests written BEFORE implementation (TDD: RED → GREEN → REFACTOR)
- [ ] Tests run with `-race` flag
- [ ] New tests added for each acceptance criterion in the linked issue
- [ ] Edge cases and error paths are covered
- [ ] Table-driven tests used where applicable

## Checklist

- [ ] Referenced issue has `status:approved`
- [ ] PR title follows Conventional Commits: `type(scope): description`
- [ ] Documentation updated (GoDoc comments, `docs/`, or ADR) if public API changed
