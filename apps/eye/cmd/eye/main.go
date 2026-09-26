// Command eye is the single binary of the project: a local OSINT gateway that
// builds a live picture of Cordoba from public sources only.
//
// It has no mandatory services. There is nothing to deploy but this file's
// build output.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/FullFran/cordvba/apps/eye/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	os.Exit(cli.New().Run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}
