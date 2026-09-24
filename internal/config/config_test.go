package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadAgentDefaults(t *testing.T) {
	path := writeTemp(t, `
postgres:
  host: "db.example.com"
  user: "yama"
`)
	cfg, err := LoadAgent(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Postgres.Port != 5432 {
		t.Errorf("port default: got %d", cfg.Postgres.Port)
	}
	if cfg.Postgres.Database != "postgres" {
		t.Errorf("database default: got %q", cfg.Postgres.Database)
	}
	if cfg.Postgres.SSLMode != "prefer" {
		t.Errorf("sslmode default: got %q", cfg.Postgres.SSLMode)
	}
	if cfg.Postgres.Password != "" {
		t.Errorf("password default without env: got %q", cfg.Postgres.Password)
	}
	if cfg.Collector.Interval.Duration != 10*time.Second {
		t.Errorf("interval default: got %s", cfg.Collector.Interval.Duration)
	}
	if cfg.Collector.QueryTimeout.Duration != 3*time.Second {
		t.Errorf("query_timeout default: got %s", cfg.Collector.QueryTimeout.Duration)
	}
	if cfg.Storage.DataDir != "./data" {
		t.Errorf("data_dir default: got %q", cfg.Storage.DataDir)
	}
	if cfg.API.Listen != ":8080" {
		t.Errorf("listen default: got %q", cfg.API.Listen)
	}
}

func TestLoadAgentMissingHost(t *testing.T) {
	path := writeTemp(t, "postgres:\n  user: \"yama\"\n")
	_, err := LoadAgent(path)
	if err == nil {
		t.Fatal("expected error for missing host")
	}
}

func TestLoadAgentMissingUser(t *testing.T) {
	path := writeTemp(t, "postgres:\n  host: \"db.example.com\"\n")
	_, err := LoadAgent(path)
	if err == nil {
		t.Fatal("expected error for missing user")
	}
}

func TestLoadAgentPasswordFromEnv(t *testing.T) {
	t.Setenv("PG_PASSWORD", "secret-from-env")
	path := writeTemp(t, `
postgres:
  host: "db.example.com"
  user: "yama"
`)
	cfg, err := LoadAgent(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Postgres.Password != "secret-from-env" {
		t.Errorf("env fallback: got %q", cfg.Postgres.Password)
	}
}

func TestLoadAgentMalformedYAML(t *testing.T) {
	path := writeTemp(t, "postgres:\n\tbad indent")
	if _, err := LoadAgent(path); err == nil {
		t.Fatal("expected error for malformed YAML")
	}
}

func TestLoadMetricsValid(t *testing.T) {
	path := writeTemp(t, `
metrics:
  - name: select_1
    query: "SELECT 1"
    type: gauge
  - name: pg_sessions
    query: "SELECT count(*) FROM pg_stat_activity"
    type: gauge
    enabled: false
    interval: 30s
`)
	cfg, err := LoadMetrics(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Metrics) != 2 {
		t.Fatalf("got %d metrics", len(cfg.Metrics))
	}
	if !cfg.Metrics[0].IsEnabled() {
		t.Error("enabled default should be true")
	}
	if cfg.Metrics[1].IsEnabled() {
		t.Error("explicit enabled: false should be false")
	}
	if cfg.Metrics[1].Interval.Duration != 30*time.Second {
		t.Errorf("interval parse: got %s", cfg.Metrics[1].Interval.Duration)
	}
}

func TestLoadMetricsUnknownType(t *testing.T) {
	path := writeTemp(t, `
metrics:
  - name: bad
    query: "SELECT 1"
    type: histogram
`)
	if _, err := LoadMetrics(path); err == nil {
		t.Fatal("expected error for unknown metric type")
	}
}

func TestLoadMetricsDuplicateNames(t *testing.T) {
	path := writeTemp(t, `
metrics:
  - name: dup
    query: "SELECT 1"
    type: gauge
  - name: dup
    query: "SELECT 2"
    type: counter
`)
	if _, err := LoadMetrics(path); err == nil {
		t.Fatal("expected error for duplicate metric names")
	}
}

func TestEffectiveInterval(t *testing.T) {
	global := 10 * time.Second
	noOverride := Metric{Name: "a"}
	if got := noOverride.EffectiveInterval(global); got != 10*time.Second {
		t.Errorf("without override: got %s", got)
	}
	override := Metric{Name: "b", Interval: Duration{30 * time.Second}}
	if got := override.EffectiveInterval(global); got != 30*time.Second {
		t.Errorf("with override: got %s", got)
	}
}

func TestDurationIntegerSeconds(t *testing.T) {
	path := writeTemp(t, `
postgres:
  host: h
  user: u
collector:
  interval: 20
`)
	cfg, err := LoadAgent(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Collector.Interval.Duration != 20*time.Second {
		t.Errorf("integer seconds: got %s", cfg.Collector.Interval.Duration)
	}
}

func TestMaxDBConnectionsDefaultIsOne(t *testing.T) {
	path := writeTemp(t, "postgres:\n  host: h\n  user: u\n")
	cfg, err := LoadAgent(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Postgres.MaxDBConnections != nil {
		t.Errorf("unset field should stay nil, got %d", *cfg.Postgres.MaxDBConnections)
	}
	if got := cfg.Postgres.EffectiveMaxDBConnections(); got != 1 {
		t.Errorf("effective default: got %d, want 1", got)
	}
}

func TestMaxDBConnectionsExplicitValues(t *testing.T) {
	for _, v := range []int{1, 5} {
		path := writeTemp(t, fmt.Sprintf("postgres:\n  host: h\n  user: u\n  max_db_connections: %d\n", v))
		cfg, err := LoadAgent(path)
		if err != nil {
			t.Fatalf("max_db_connections=%d: unexpected error: %v", v, err)
		}
		if got := cfg.Postgres.EffectiveMaxDBConnections(); got != v {
			t.Errorf("max_db_connections=%d: effective got %d", v, got)
		}
	}
}

func TestMaxDBConnectionsInvalidValues(t *testing.T) {
	for _, v := range []int{0, -3} {
		path := writeTemp(t, fmt.Sprintf("postgres:\n  host: h\n  user: u\n  max_db_connections: %d\n", v))
		_, err := LoadAgent(path)
		if err == nil {
			t.Fatalf("max_db_connections=%d: expected error", v)
		}
		if !strings.Contains(err.Error(), "postgres.max_db_connections") {
			t.Errorf("max_db_connections=%d: error does not name the field: %v", v, err)
		}
	}
}

func TestExampleConfigLoads(t *testing.T) {
	path := filepath.Join("..", "..", "configs", "config.yaml.example")
	cfg, err := LoadAgent(path)
	if err != nil {
		t.Fatalf("example config failed to load: %v", err)
	}
	if got := cfg.Postgres.EffectiveMaxDBConnections(); got != 1 {
		t.Errorf("example config effective max connections: got %d, want 1", got)
	}
}
