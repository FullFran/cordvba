package version_test

import (
	"strings"
	"testing"

	"github.com/FullFran/cordvba/apps/eye/internal/version"
)

func TestString(t *testing.T) {
	t.Parallel()

	got := version.String()

	for _, want := range []string{version.Version, version.Commit, version.Date} {
		if !strings.Contains(got, want) {
			t.Errorf("String() = %q, want it to contain %q", got, want)
		}
	}
}
