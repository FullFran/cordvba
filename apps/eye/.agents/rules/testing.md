# Testing Rules — eye

## Runner

```
go test -race -cover ./...        # standard
go test -race -cover -v ./...     # verbose
make test                         # via Makefile
```

Coverage threshold: **80%** minimum. Failing to meet it blocks CI.

## Where tests live

- Unit tests: `<package>/<file>_test.go` (same package, `package <pkg>_test` for black-box).
- Integration tests: tag with `//go:build integration` and keep in the same package dir.
- Test helpers / fixtures: `internal/testutil/` (never in production packages).

## TDD order (mandatory)

1. **RED** — write the failing test. It must fail for the right reason (assertion, not compile error).
2. **GREEN** — write the minimum implementation to pass.
3. **REFACTOR** — clean up while green.

Never write implementation before a failing test exists.

## Table-driven tests

Prefer table-driven tests for any function with multiple input/output cases:

```go
func TestFoo(t *testing.T) {
    t.Parallel()
    cases := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        {name: "valid input", input: "ok", want: "OK"},
        {name: "empty input", input: "", wantErr: true},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            t.Parallel()
            got, err := Foo(tc.input)
            if tc.wantErr {
                if err == nil { t.Fatal("expected error, got nil") }
                return
            }
            if err != nil { t.Fatalf("unexpected error: %v", err) }
            if got != tc.want { t.Errorf("got %q, want %q", got, tc.want) }
        })
    }
}
```

## Race detector

Always run with `-race`. Any race condition found is a blocking bug.

## HTTP handler tests

Use `net/http/httptest` — no live servers in unit tests:

```go
w := httptest.NewRecorder()
r := httptest.NewRequest(http.MethodGet, "/health", nil)
handler.ServeHTTP(w, r)
```

## Mocking

Use interfaces; inject fakes. Avoid reflection-based mock generators for core domain logic.
