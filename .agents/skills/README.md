# Agent Skills — eye

`skills-lock.json` lists the recommended Claude Code skills for this service.

## How to use

When working in this repository, invoke the skills listed in `skills-lock.json` via the `/skill` command or the `Skill` tool. Each entry documents *why* the skill is relevant so you can decide whether to activate it for a given task.

Skill loading is context-dependent:

| Task                    | Skills to activate                              |
| ----------------------- | ----------------------------------------------- |
| Adding a new domain     | `senior-architect`, `go-testing`                |
| Security-sensitive code | `owasp-security`, `senior-security`             |
| Performance work        | `performance`                                   |
| PR review               | `code-review`, `senior-architect`               |

Skills are **not** auto-loaded — activate them explicitly for the relevant task.
