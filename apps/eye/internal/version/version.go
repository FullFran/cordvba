// Package version exposes the build identity of the binary. Values are
// injected at link time by the Makefile; they fall back to development
// defaults so that "go run" still works.
package version

// Build metadata, overridden via -ldflags -X.
var (
	// Version is the semantic version or git describe output.
	Version = "dev"
	// Commit is the short git SHA.
	Commit = "none"
	// Date is the build timestamp in RFC 3339.
	Date = "unknown"
)

// String renders the full build identity on one line.
func String() string {
	return Version + " (" + Commit + ", built " + Date + ")"
}
