package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"
)

// queryCommand is the structured question, answered without a language model
// anywhere near it.
func queryCommand() Command {
	return Command{
		Name:    "query",
		Summary: "Query observations across every live source",
		Run: func(ctx context.Context, args []string, stdout, stderr io.Writer) error {
			fs := flag.NewFlagSet("query", flag.ContinueOnError)
			fs.SetOutput(stderr)

			opts := &observeOptions{}
			opts.bind(fs, 50, 24*time.Hour)
			topics := fs.String("topic", "", "comma-separated topics (default: every live topic)")

			if err := fs.Parse(args); err != nil {
				return err
			}

			wanted := splitCSV(*topics)
			if len(wanted) == 0 {
				rt, err := newRuntime(runtimeOptions{registry: opts.registry, dataDir: opts.dataDir})
				if err != nil {
					return err
				}
				wanted = liveTopics(rt)
				_ = rt.Close()
				if len(wanted) == 0 {
					return fmt.Errorf("no live source in the registry")
				}
			}

			return observe(ctx, opts, wanted, stdout, stderr, renderFeed)
		},
	}
}

// liveTopics lists the distinct topics eye can actually poll right now.
func liveTopics(rt *runtime) []string {
	ps, _ := rt.pollable()
	seen := map[string]bool{}
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		topic := p.Info().Topic
		if !seen[topic] {
			seen[topic] = true
			out = append(out, topic)
		}
	}
	return out
}

// splitCSV parses a comma-separated flag value.
func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}
