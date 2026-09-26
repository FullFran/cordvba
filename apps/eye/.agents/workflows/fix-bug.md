# Workflow: Fix Bug — eye

## Steps

1. **Reproduce** — write a failing test that demonstrates the bug exactly.
   - Run `go test -race ./...` — confirm the new test fails.
   - If you cannot reproduce with a test, you do not yet understand the bug. Keep digging.

2. **Identify root cause** — read the stack trace and trace the error through domain → application → infrastructure.

3. **Fix** — minimum change that makes the failing test pass. Do not refactor unrelated code in the same commit.

4. **Verify** — run `go test -race -cover ./...`. All previously passing tests must still pass.

5. **Check for related paths** — search for similar patterns:
   ```
   rg -n "<buggy_pattern>" ./internal
   ```
   Fix all occurrences in the same PR if under 400 lines; otherwise file a follow-up issue.

6. **Run full CI**
   ```
   make ci-local
   ```

7. **Commit**
   ```
   fix(<scope>): <imperative description>
   ```
   Include a brief *why* in the commit body referencing the root cause.

8. **Open PR** — link to the issue. Description must include: symptom, root cause, fix approach.
