// yama — Yet Another Monitoring Agent. Collects PostgreSQL metrics on a
// schedule, stores them in embedded BadgerDB, and serves them over HTTP.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bazaruzero/yama/internal/api"
	"github.com/bazaruzero/yama/internal/collect"
	"github.com/bazaruzero/yama/internal/config"
	"github.com/bazaruzero/yama/internal/postgres"
	"github.com/bazaruzero/yama/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "config.yaml", "path to the agent config YAML")
	metricsPath := flag.String("metrics", "metrics.yaml", "path to the metrics config YAML")
	flag.Parse()

	log := slog.Default()

	cfg, err := config.LoadAgent(*configPath)
	if err != nil {
		return err
	}
	mcfg, err := config.LoadMetrics(*metricsPath)
	if err != nil {
		return err
	}

	st, err := store.OpenBadger(cfg.Storage.DataDir)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := st.Close(); cerr != nil {
			log.Error("storage close failed", "error", cerr)
		}
	}()
	log.Info("storage opened", "data_dir", cfg.Storage.DataDir)

	pool, err := postgres.Open(context.Background(), cfg.Postgres, cfg.Collector.QueryTimeout.Duration)
	if err != nil {
		return err
	}
	defer pool.Close()
	log.Info("connected to PostgreSQL",
		"host", cfg.Postgres.Host, "port", cfg.Postgres.Port, "database", cfg.Postgres.Database)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	collector := collect.NewCollector(pool, st, cfg.Collector.QueryTimeout.Duration)
	scheduler := collect.NewScheduler(collector, mcfg.Metrics, cfg.Collector.Interval.Duration, log)
	schedDone := make(chan struct{})
	go func() {
		defer close(schedDone)
		scheduler.Run(ctx)
	}()

	apiServer := api.NewServer(st, log)
	httpServer := &http.Server{Addr: cfg.API.Listen, Handler: apiServer.Handler()}
	serveErr := make(chan error, 1)
	go func() {
		log.Info("API listening", "address", cfg.API.Listen)
		serveErr <- httpServer.ListenAndServe()
	}()

	// Wait for shutdown signal or an API server failure.
	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("api server: %w", err)
		}
	}

	// Shutdown order: stop accepting API requests, cancel collection context,
	// drain in-flight collector goroutines, then close storage (defer above).
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Error("API shutdown failed", "error", err)
	}
	stop()
	<-schedDone
	log.Info("shutdown complete")
	return nil
}
