package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/FullFran/eye/internal/api"
	"github.com/FullFran/eye/internal/logging"
)

// serveTimeouts keep a stalled client from holding a connection forever.
const (
	serveReadTimeout  = 15 * time.Second
	serveWriteTimeout = 60 * time.Second
	serveIdleTimeout  = 2 * time.Minute
	serveShutdownWait = 10 * time.Second
)

// serveCommand exposes the store over HTTP.
//
// This is the adapter that makes eye a gateway: providers are written once, and
// anything that speaks HTTP reads the same normalized records with the same
// provenance attached.
func serveCommand() Command {
	return Command{
		Name:    "serve",
		Summary: "Serve observations over HTTP, so other programs can read them",
		Run: func(ctx context.Context, args []string, stdout, stderr io.Writer) error {
			fs := flag.NewFlagSet("serve", flag.ContinueOnError)
			fs.SetOutput(stderr)
			var (
				registryP = fs.String("registry", "", "path to an alternative sources.yaml")
				dataDir   = fs.String("data-dir", "", "override where the store and raw cache live")
				addr      = fs.String("addr", "127.0.0.1:8787", "address to listen on")
				public    = fs.Bool("public", false, "allow binding to a non-loopback address")
			)
			if err := fs.Parse(args); err != nil {
				return err
			}

			if err := checkBind(*addr, *public); err != nil {
				return err
			}

			rt, err := newRuntime(runtimeOptions{registry: *registryP, dataDir: *dataDir})
			if err != nil {
				return err
			}
			defer func() { _ = rt.Close() }()

			log := logging.New(logging.Options{Level: rt.cfg.LogLevel, Writer: stderr})
			server := &http.Server{
				Addr:         *addr,
				Handler:      api.New(rt.store, rt.sources, log).Handler(),
				ReadTimeout:  serveReadTimeout,
				WriteTimeout: serveWriteTimeout,
				IdleTimeout:  serveIdleTimeout,
			}

			// ListenConfig rather than net.Listen: the listener then
			// respects the same context that stops the server.
			var lc net.ListenConfig
			listener, err := lc.Listen(ctx, "tcp", *addr)
			if err != nil {
				return fmt.Errorf("listen on %s: %w", *addr, err)
			}

			_, _ = fmt.Fprintf(stdout, "eye is serving on http://%s\n", listener.Addr())
			_, _ = fmt.Fprintf(stdout, "  /v1/records   /v1/entities   /v1/sources   /health\n")
			_, _ = fmt.Fprintf(stdout, "  store: %s\n", rt.store.Path())
			_, _ = fmt.Fprintln(stdout, "\nThis serves observations. It never serves camera images:")
			_, _ = fmt.Fprintln(stdout, "their licence permits personal use only.")

			errs := make(chan error, 1)
			go func() {
				if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
					errs <- err
				}
			}()

			select {
			case err := <-errs:
				return err
			case <-ctx.Done():
			}

			shutdownCtx, cancel := context.WithTimeout(context.Background(), serveShutdownWait)
			defer cancel()

			_, _ = fmt.Fprintln(stdout, "\nshutting down")
			return server.Shutdown(shutdownCtx)
		},
	}
}

// checkBind refuses a non-loopback address unless it was asked for explicitly.
//
// eye reads sources under licences that in several cases permit personal use
// only, and the store holds an aircraft position history. Exposing that to a
// network should be a decision somebody made on purpose, not the default that
// came with a config file.
func checkBind(addr string, public bool) error {
	if public {
		return nil
	}

	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("addr %q: %w", addr, err)
	}

	if host == "" || host == "0.0.0.0" || host == "::" {
		return errors.New("refusing to listen on every interface: pass --public if that is what you mean")
	}
	if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() {
		return fmt.Errorf("refusing to listen on the non-loopback address %s: pass --public if that is what you mean", host)
	}
	if ip := net.ParseIP(host); ip == nil && !strings.EqualFold(host, "localhost") {
		return fmt.Errorf("refusing to listen on %q: pass --public if that is what you mean", host)
	}
	return nil
}
