// Command immerle is the immerle-server binary: a Subsonic/OpenSubsonic
// compatible music server with native social and on-demand catalog extensions.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/immerle/immerle/internal/app"
	"github.com/immerle/immerle/internal/config"
)

// awaitShutdownSignal cancels ctx on the first SIGINT/SIGTERM (starting a
// graceful shutdown) and calls forceExit on the second, so an operator who
// doesn't want to wait out the shutdown grace period can press Ctrl+C again
// to quit immediately instead of it being silently swallowed — signal.Notify
// keeps relaying every signal, unlike signal.NotifyContext, which stops
// reacting after the first (see internal/server/server.go's shutdownGrace).
func awaitShutdownSignal(cancel context.CancelFunc, forceExit func()) {
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
		<-sigCh
		forceExit()
	}()
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	envPath := flag.String("env", "", "path to a .env file (default: .env if present)")
	flag.Parse()

	cfg, err := config.Load(*envPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	application, err := app.New(cfg)
	if err != nil {
		return fmt.Errorf("init app: %w", err)
	}
	defer func() { _ = application.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	awaitShutdownSignal(cancel, func() { os.Exit(1) })

	return application.Run(ctx)
}
