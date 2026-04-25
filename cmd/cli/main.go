// Package main starts the logmonitor CLI.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	clitransport "github.com/lenchik/logmonitor/internal/transport/cli"
	"github.com/lenchik/logmonitor/pkg/appmode"
)

// main executes the CLI root command until completion or signal cancellation.
func main() {
	if _, err := appmode.Require(appmode.CLI); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := clitransport.NewRootCommand().ExecuteContext(ctx); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
