package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"sort"
	"text/tabwriter"

	providers "github.com/FullFran/eye/internal/provider/infrastructure"
	source "github.com/FullFran/eye/internal/source/domain"
)

// sourcesCommand lists the registry and, for each entry, whether eye is
// permitted to poll it and whether this build can read it.
//
// Those are two different questions and both are worth an honest answer.
func sourcesCommand() Command {
	return Command{
		Name:    "sources",
		Summary: "List the source registry and what eye may and can read",
		Run: func(ctx context.Context, args []string, stdout, stderr io.Writer) error {
			fs := flag.NewFlagSet("sources", flag.ContinueOnError)
			fs.SetOutput(stderr)
			var (
				asJSON    = fs.Bool("json", false, "output as JSON")
				topic     = fs.String("topic", "", "only sources of this topic")
				registryP = fs.String("registry", "", "path to an alternative sources.yaml")
			)
			if err := fs.Parse(args); err != nil {
				return err
			}

			rt, err := newRuntime(*registryP)
			if err != nil {
				return err
			}

			list := rt.sources
			if *topic != "" {
				list = filterByTopic(list, []string{*topic})
			}
			sort.Slice(list, func(i, j int) bool {
				if list[i].Topic != list[j].Topic {
					return list[i].Topic < list[j].Topic
				}
				return list[i].ID < list[j].ID
			})

			if *asJSON {
				return writeJSON(stdout, sourceViews(list))
			}
			return renderSources(stdout, rt.registryPath, list)
		},
	}
}

// sourceView is the JSON shape of a registry entry.
type sourceView struct {
	ID         string `json:"id"`
	Authority  string `json:"authority"`
	Topic      string `json:"topic"`
	Format     string `json:"format"`
	License    string `json:"license"`
	Access     string `json:"access"`
	Automation string `json:"automation"`
	Pollable   bool   `json:"pollable"`
	HasAdapter bool   `json:"has_adapter"`
	Interval   string `json:"interval,omitempty"`
	URL        string `json:"url"`
	Notes      string `json:"notes,omitempty"`
}

// sourceViews projects the registry for machine consumption.
func sourceViews(list []source.Source) []sourceView {
	out := make([]sourceView, 0, len(list))
	for _, s := range list {
		v := sourceView{
			ID:         s.ID,
			Authority:  s.Authority,
			Topic:      s.Topic,
			Format:     s.Format,
			License:    s.License,
			Access:     string(s.Access),
			Automation: string(s.Automation),
			Pollable:   s.Automation.Pollable(),
			HasAdapter: providers.Supported(s.Format),
			URL:        s.URL,
			Notes:      s.Notes,
		}
		if s.Interval > 0 {
			v.Interval = s.Interval.String()
		}
		out = append(out, v)
	}
	return out
}

// renderSources writes the human-readable registry table.
func renderSources(w io.Writer, registryPath string, list []source.Source) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "SOURCE\tTOPIC\tFORMAT\tSTATE\tLICENCE\tAUTHORITY")

	var live, held, noAdapter int
	for _, s := range list {
		state, ok := stateOf(s)
		switch state {
		case "live":
			live++
		case "no adapter":
			noAdapter++
		default:
			held++
		}
		_ = ok

		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			s.ID, s.Topic, s.Format, state,
			ellipsis(s.License, 22), ellipsis(s.Authority, 30))
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(w, "\n%s · %s live · %s held · %s awaiting an adapter\n",
		plural(len(list), "source", "sources"),
		fmt.Sprint(live), fmt.Sprint(held), fmt.Sprint(noAdapter))
	_, _ = fmt.Fprintf(w, "registry: %s\n", registryPath)

	if held > 0 {
		_, _ = fmt.Fprintln(w, "\nHeld sources are not a failure. They are sources whose reuse terms")
		_, _ = fmt.Fprintln(w, "are unresolved, or that publish no documented machine interface.")
	}
	return nil
}

// stateOf describes what eye will actually do with a source.
func stateOf(s source.Source) (state string, pollable bool) {
	switch {
	case !s.Automation.Pollable():
		return string(s.Automation), false
	case !providers.Supported(s.Format):
		return "no adapter", false
	default:
		return "live", true
	}
}
