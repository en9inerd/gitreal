package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"sync"
	"syscall"
	"time"

	"github.com/en9inerd/gitreal/internal/config"
	"github.com/en9inerd/gitreal/internal/github"
	"github.com/en9inerd/gitreal/internal/log"
	"github.com/en9inerd/gitreal/internal/server"
)

var version = "dev"

func versionString() string {
	var revision, buildTime string
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, kv := range info.Settings {
			switch kv.Key {
			case "vcs.revision":
				if len(kv.Value) >= 7 {
					revision = kv.Value[:7]
				}
			case "vcs.time":
				buildTime = kv.Value
			}
		}
	}
	s := "gitreal version " + version
	if revision != "" {
		s += " (" + revision + ")"
	}
	if buildTime != "" {
		s += " built " + buildTime
	}
	return s
}

func run(ctx context.Context, args []string, getenv func(string) string) error {
	for _, a := range args[1:] {
		if a == "--version" || a == "-version" {
			fmt.Println(versionString())
			return nil
		}
	}

	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	cfg, err := config.ParseConfig(getenv)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	logger := log.NewLogger(cfg.Verbose)
	logger.Info("starting server", "version", version, "port", cfg.Port)

	ghClient := github.NewClient(cfg.GitHubToken)

	handler, err := server.NewServer(logger, cfg, ghClient)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	var wg sync.WaitGroup
	wg.Go(func() {
		<-ctx.Done()
		logger.Info("shutdown signal received")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			fmt.Fprintf(os.Stderr, "error shutting down http server: %s\n", err)
		}
		logger.Info("server stopped")
	})

	logger.Info("listening", "addr", httpServer.Addr)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("error listening and serving: %w", err)
	}

	wg.Wait()
	return nil
}

func main() {
	ctx := context.Background()
	if err := run(ctx, os.Args, os.Getenv); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
