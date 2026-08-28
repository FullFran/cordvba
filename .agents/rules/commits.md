# Commit Conventions — eye

Format: `<type>(<scope>): <imperative description>`

## Types

| Type       | When to use                                              |
| ---------- | -------------------------------------------------------- |
| `feat`     | New behavior visible to callers or operators             |
| `fix`      | Bug fix                                                  |
| `refactor` | Internal restructure with no behavior change             |
| `test`     | Test additions or updates only                           |
| `chore`    | Tooling, deps, CI, config — no production code change    |
| `docs`     | Documentation only                                       |
| `perf`     | Performance improvement                                  |
| `build`    | Makefile, build scripts, release tooling                 |

## Scope enum

Use one of these scopes; add a new one only when a genuinely new domain is created.

`cli` | `config` | `observation` | `source` | `provider` | `event` | `scheduler` | `fusion` | `store` | `serve` | `docs` | `ci`

## Examples

```
feat(provider): add DGT DATEX II incident adapter
feat(cli): add eye radar command for the next 72 hours
fix(source): fail closed on unknown automation status
test(observation): add table-driven tests for record validation
refactor(scheduler): extract backoff into its own type
docs(adr): record the CGO-free storage decision
chore(ci): pin golangci-lint to v2
```

## Rules

- Description is imperative, lowercase, no trailing period.
- Body (optional): explain *why*, not *what*.
- Breaking changes: add `!` after scope and a `BREAKING CHANGE:` footer.
- No AI attribution lines (no "Co-Authored-By: Claude…").
