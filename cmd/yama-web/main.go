// yama-web — the YAMA dashboard frontend. Connects to a running YAMA
// agent's read-only metric API and serves a live dashboard of its metrics.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	webagent "github.com/bazaruzero/yama/internal/web/agent"
	webconfig "github.com/bazaruzero/yama/internal/web/config"
	webserver "github.com/bazaruzero/yama/internal/web/server"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "webui.yaml", "path to the frontend system config YAML")
	graphsPath := flag.String("graphs", "graphs.yaml", "path to the graphs config YAML")
	flag.Parse()

	log := slog.Default()

	cfg, err := webconfig.LoadSystem(*configPath)
	if err != nil {
		return err
	}
	graphs, err := webconfig.LoadGraphs(*graphsPath)
	if err != nil {
		return err
	}

	// The agent is contacted only when panels are fetched, so an
	// unreachable agent is a degraded state, not a startup failure.
	client := webagent.NewClient(cfg.Agent.Host, cfg.Agent.Port, cfg.Agent.EffectiveTimeout())
	srv, err := webserver.NewServer(*cfg, graphs.Graphs, client, log)
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", cfg.UI.Listen)
	if err != nil {
		return fmt.Errorf("listen on ui.listen %s: %w", cfg.UI.Listen, err)
	}
	log.Info("dashboard listening",
		"address", cfg.UI.Listen,
		"agent", fmt.Sprintf("%s:%d", cfg.Agent.Host, cfg.Agent.Port),
		"refresh_interval", cfg.Refresh.EffectiveInterval(),
		"panels", len(graphs.Graphs))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	httpServer := &http.Server{Handler: srv.Handler()}
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- httpServer.Serve(listener)
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("ui server on %s: %w", cfg.UI.Listen, err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Error("ui shutdown failed", "error", err)
	}
	log.Info("shutdown complete")
	return nil
}
