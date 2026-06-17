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

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	return application.Run(ctx)
}
