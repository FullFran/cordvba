package cli_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/FullFran/eye/internal/cli"
)

// run executes the app and returns exit code, stdout and stderr.
func run(t *testing.T, app *cli.App, args ...string) (int, string, string) {
	t.Helper()

	var stdout, stderr bytes.Buffer
	code := app.Run(context.Background(), args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestRunVersion(t *testing.T) {
	t.Parallel()

	code, stdout, stderr := run(t, cli.New(), "version")

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
	if !strings.Contains(stdout, "dev") {
		t.Errorf("stdout = %q, want it to contain the default version", stdout)
	}
}

func TestRunHelp(t *testing.T) {
	t.Parallel()

	cases := []struct{ name, arg string }{
		{name: "help subcommand", arg: "help"},
		{name: "short flag", arg: "-h"},
		{name: "long flag", arg: "--help"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			code, stdout, _ := run(t, cli.New(), tc.arg)
			if code != 0 {
				t.Errorf("exit code = %d, want 0", code)
			}
			if !strings.Contains(stdout, "version") {
				t.Errorf("usage should list the version command, got %q", stdout)
			}
		})
	}
}

func TestRunNoArgsPrintsUsage(t *testing.T) {
	t.Parallel()

	code, stdout, _ := run(t, cli.New())
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "Usage:") {
		t.Errorf("stdout = %q, want usage", stdout)
	}
}

func TestRunUnknownCommandExitsTwo(t *testing.T) {
	t.Parallel()

	code, _, stderr := run(t, cli.New(), "sudo-make-me-a-sandwich")

	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "unknown command") {
		t.Errorf("stderr = %q, want it to name the problem", stderr)
	}
}

func TestRunFailingCommandExitsOne(t *testing.T) {
	t.Parallel()

	app := cli.New()
	app.Register(cli.Command{
		Name:    "boom",
		Summary: "Always fails",
		Run: func(context.Context, []string, io.Writer, io.Writer) error {
			return errors.New("source unreachable")
		},
	})

	code, _, stderr := run(t, app, "boom")

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "source unreachable") {
		t.Errorf("stderr = %q, want the underlying error", stderr)
	}
}

func TestRegisterReplacesCommandOfTheSameName(t *testing.T) {
	t.Parallel()

	app := cli.New()
	app.Register(cli.Command{
		Name:    "version",
		Summary: "Overridden",
		Run: func(_ context.Context, _ []string, stdout, _ io.Writer) error {
			_, err := io.WriteString(stdout, "overridden\n")
			return err
		},
	})

	_, stdout, _ := run(t, app, "version")
	if !strings.Contains(stdout, "overridden") {
		t.Errorf("stdout = %q, want the replacement command to run", stdout)
	}
}
