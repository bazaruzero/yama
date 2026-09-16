// Package config loads and validates the agent and metrics YAML files.
package config

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// PostgresConfig holds the connection fields for the monitored database.
type PostgresConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Database string `yaml:"database"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	SSLMode  string `yaml:"sslmode"`
}

// CollectorConfig holds scheduling and query execution settings.
type CollectorConfig struct {
	Interval     Duration `yaml:"interval"`
	QueryTimeout Duration `yaml:"query_timeout"`
}

// StorageConfig holds local storage settings.
type StorageConfig struct {
	DataDir string `yaml:"data_dir"`
}

// APIConfig holds HTTP API settings.
type APIConfig struct {
	Listen string `yaml:"listen"`
}

// AgentConfig is the root of the agent configuration file.
type AgentConfig struct {
	Postgres  PostgresConfig  `yaml:"postgres"`
	Collector CollectorConfig `yaml:"collector"`
	Storage   StorageConfig   `yaml:"storage"`
	API       APIConfig       `yaml:"api"`
}

const (
	defaultPort         = 5432
	defaultDatabase     = "postgres"
	defaultSSLMode      = "prefer"
	defaultInterval     = 10 * time.Second
	defaultQueryTimeout = 3 * time.Second
	defaultDataDir      = "./data"
	defaultListen       = ":8080"
)

// LoadAgent reads and validates the agent configuration file, applying
// documented defaults for all optional fields. The postgres password falls
// back to the PG_PASSWORD environment variable when not set in the file.
func LoadAgent(path string) (*AgentConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read agent config %q: %w", path, err)
	}
	var cfg AgentConfig
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse agent config %q: %w", path, err)
	}
	if cfg.Postgres.Host == "" {
		return nil, fmt.Errorf("agent config %q: postgres.host is required", path)
	}
	if cfg.Postgres.User == "" {
		return nil, fmt.Errorf("agent config %q: postgres.user is required", path)
	}
	if cfg.Postgres.Port == 0 {
		cfg.Postgres.Port = defaultPort
	}
	if cfg.Postgres.Database == "" {
		cfg.Postgres.Database = defaultDatabase
	}
	if cfg.Postgres.SSLMode == "" {
		cfg.Postgres.SSLMode = defaultSSLMode
	}
	if cfg.Postgres.Password == "" {
		cfg.Postgres.Password = os.Getenv("PG_PASSWORD")
	}
	if cfg.Collector.Interval.Duration == 0 {
		cfg.Collector.Interval = Duration{defaultInterval}
	}
	if cfg.Collector.QueryTimeout.Duration == 0 {
		cfg.Collector.QueryTimeout = Duration{defaultQueryTimeout}
	}
	if cfg.Storage.DataDir == "" {
		cfg.Storage.DataDir = defaultDataDir
	}
	if cfg.API.Listen == "" {
		cfg.API.Listen = defaultListen
	}
	return &cfg, nil
}

// MetricType is the kind of a metric: counter (cumulative) or gauge (point value).
type MetricType string

const (
	MetricTypeCounter MetricType = "counter"
	MetricTypeGauge   MetricType = "gauge"
)

// Metric is a single metric definition from the metrics configuration file.
type Metric struct {
	Name     string     `yaml:"name"`
	Query    string     `yaml:"query"`
	Type     MetricType `yaml:"type"`
	Enabled  *bool      `yaml:"enabled"`
	Interval Duration   `yaml:"interval"`
}

// IsEnabled reports whether the metric should be collected (default true).
func (m Metric) IsEnabled() bool {
	return m.Enabled == nil || *m.Enabled
}

// EffectiveInterval returns the metric's own interval when set,
// otherwise the global interval.
func (m Metric) EffectiveInterval(global time.Duration) time.Duration {
	if m.Interval.Duration > 0 {
		return m.Interval.Duration
	}
	return global
}

// MetricsConfig is the root of the metrics configuration file.
type MetricsConfig struct {
	Metrics []Metric `yaml:"metrics"`
}

// LoadMetrics reads and validates the metrics configuration file.
func LoadMetrics(path string) (*MetricsConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read metrics config %q: %w", path, err)
	}
	var cfg MetricsConfig
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse metrics config %q: %w", path, err)
	}
	seen := make(map[string]bool)
	for i, m := range cfg.Metrics {
		if m.Name == "" {
			return nil, fmt.Errorf("metrics config %q: metric #%d is missing a name", path, i+1)
		}
		if seen[m.Name] {
			return nil, fmt.Errorf("metrics config %q: duplicate metric name %q", path, m.Name)
		}
		seen[m.Name] = true
		if m.Query == "" {
			return nil, fmt.Errorf("metrics config %q: metric %q is missing a query", path, m.Name)
		}
		switch m.Type {
		case MetricTypeCounter, MetricTypeGauge:
		default:
			return nil, fmt.Errorf("metrics config %q: metric %q has unknown type %q (must be counter or gauge)", path, m.Name, m.Type)
		}
	}
	return &cfg, nil
}

// Duration is a time.Duration that unmarshals from either a duration string
// ("30s", "5m") or an integer number of seconds.
type Duration struct {
	time.Duration
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		if secs, err := strconv.ParseInt(value.Value, 10, 64); err == nil {
			d.Duration = time.Duration(secs) * time.Second
			return nil
		}
		dur, err := time.ParseDuration(value.Value)
		if err != nil {
			return fmt.Errorf("invalid duration %q (use e.g. \"30s\" or seconds as integer)", value.Value)
		}
		d.Duration = dur
		return nil
	}
	return fmt.Errorf("invalid duration value")
}
