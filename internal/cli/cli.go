// Package cli is the command-line infrastructure adapter. It parses arguments
// and renders output; it holds no domain logic of its own.
//
// eye ships as a single binary with several modes. "status" and "query" answer
// from the local store, "daemon" runs the scheduler, and "serve" exposes HTTP
// only when explicitly asked for. HTTP is one adapter among several, never the
// centre of the system.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"
	"text/tabwriter"

	"github.com/FullFran/eye/internal/version"
)

// ErrUnknownCommand is returned when the first argument matches no command.
var ErrUnknownCommand = errors.New("unknown command")

// Command is one subcommand of the eye binary.
type Command struct {
	// Name is the token typed on the command line.
	Name string
	// Summary is the one-line description shown by "eye help".
	Summary string
	// Run executes the command. args excludes the command name itself.
	Run func(ctx context.Context, args []string, stdout, stderr io.Writer) error
}

// App is the command registry and the entry point of the binary.
type App struct {
	commands map[string]Command
}

// New builds an App with the commands available in this build.
func New() *App {
	app := &App{commands: make(map[string]Command)}
	for _, c := range []Command{
		statusCommand(),
		daemonCommand(),
		watchCommand(),
		newsCommand(),
		eventsCommand(),
		civicCommand(),
		skyCommand(),
		camerasCommand(),
		cameraCommand(),
		queryCommand(),
		sourcesCommand(),
		versionCommand(),
	} {
		app.Register(c)
	}
	return app
}

// Register adds a command to the registry, replacing any command with the same
// name.
func (a *App) Register(c Command) { a.commands[c.Name] = c }

// Run dispatches argv (without the program name) and returns the process exit
// code. Usage problems exit 2, runtime failures exit 1.
func (a *App) Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		a.usage(stdout)
		return 0
	}

	name := args[0]
	if name == "help" || name == "-h" || name == "--help" {
		a.usage(stdout)
		return 0
	}

	cmd, ok := a.commands[name]
	if !ok {
		_, _ = fmt.Fprintf(stderr, "eye: %v: %q\n\nRun \"eye help\" to list commands.\n", ErrUnknownCommand, name)
		return 2
	}

	if err := cmd.Run(ctx, args[1:], stdout, stderr); err != nil {
		_, _ = fmt.Fprintf(stderr, "eye %s: %v\n", name, err)
		return 1
	}
	return 0
}

// usage prints the command index.
func (a *App) usage(w io.Writer) {
	_, _ = fmt.Fprintf(w, "eye — a live, source-verifiable model of Cordoba\n\nUsage:\n  eye <command> [flags]\n\nCommands:\n")

	names := make([]string, 0, len(a.commands))
	for name := range a.commands {
		names = append(names, name)
	}
	sort.Strings(names)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, name := range names {
		_, _ = fmt.Fprintf(tw, "  %s\t%s\n", name, a.commands[name].Summary)
	}
	_, _ = fmt.Fprintf(tw, "  %s\t%s\n", "help", "Show this message")
	_ = tw.Flush()

	_, _ = fmt.Fprintf(w, "\nEvery figure eye prints can be traced back to the public source it came from.\n")
}

// parseInterspersed parses flags that may appear before, after or between
// positional arguments, and returns the positionals.
//
// Go's flag package stops at the first non-flag argument, so `eye camera A-4
// --width 32` would otherwise treat "--width" and "32" as part of the search
// text. People type it that way, so it has to work.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string

	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return positional, nil
		}
		positional = append(positional, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

// versionCommand reports the build identity.
func versionCommand() Command {
	return Command{
		Name:    "version",
		Summary: "Print the build version",
		Run: func(_ context.Context, _ []string, stdout, _ io.Writer) error {
			_, err := fmt.Fprintln(stdout, version.String())
			return err
		},
	}
}
