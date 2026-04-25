// Package main starts the logmonitor CLI.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	clitransport "github.com/lenchik/logmonitor/internal/transport/cli"
)

// main executes the CLI root command until completion or signal cancellation.
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := clitransport.NewRootCommand().ExecuteContext(ctx); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
