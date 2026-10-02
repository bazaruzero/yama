// Package config loads and validates the frontend's system and graphs YAML
// configuration files.
package config

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// AgentConfig holds the address of the YAMA agent to visualize and the
// timeout for calls to it.
type AgentConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
	// Timeout bounds each call to the agent. nil means unset (defaults to
	// 5s); an explicit non-positive value is rejected.
	Timeout *Duration `yaml:"timeout"`
}

// EffectiveTimeout returns the resolved agent timeout, defaulting to 5s
// when not set.
func (a AgentConfig) EffectiveTimeout() time.Duration {
	if a.Timeout == nil {
		return defaultAgentTimeout
	}
	return a.Timeout.Duration
}

// UIConfig holds the dashboard HTTP settings.
type UIConfig struct {
	Listen string `yaml:"listen"`
}

// RefreshConfig holds the automatic refresh settings.
type RefreshConfig struct {
	// Interval is the htmx panel polling interval. nil means unset
	// (defaults to 10s); an explicit non-positive value is rejected.
	Interval *Duration `yaml:"interval"`
}

// EffectiveInterval returns the resolved refresh interval, defaulting to
// 10s when not set.
func (r RefreshConfig) EffectiveInterval() time.Duration {
	if r.Interval == nil {
		return defaultRefreshInterval
	}
	return r.Interval.Duration
}

// SystemConfig is the root of the frontend system configuration file.
type SystemConfig struct {
	Agent   AgentConfig   `yaml:"agent"`
	UI      UIConfig      `yaml:"ui"`
	Refresh RefreshConfig `yaml:"refresh"`
}

const (
	defaultAgentHost       = "localhost"
	defaultAgentPort       = 8080
	defaultAgentTimeout    = 5 * time.Second
	defaultUIListen        = ":8081"
	defaultRefreshInterval = 10 * time.Second
)

// LoadSystem reads and validates the frontend system configuration file,
// applying documented defaults for all optional fields.
func LoadSystem(path string) (*SystemConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read system config %q: %w", path, err)
	}
	var cfg SystemConfig
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse system config %q: %w", path, err)
	}
	if cfg.Agent.Host == "" {
		cfg.Agent.Host = defaultAgentHost
	}
	if cfg.Agent.Port == 0 {
		cfg.Agent.Port = defaultAgentPort
	}
	if cfg.Agent.Timeout != nil && cfg.Agent.Timeout.Duration <= 0 {
		return nil, fmt.Errorf("system config %q: agent.timeout must be a positive duration, got %s", path, cfg.Agent.Timeout.Duration)
	}
	if cfg.UI.Listen == "" {
		cfg.UI.Listen = defaultUIListen
	}
	if cfg.Refresh.Interval != nil && cfg.Refresh.Interval.Duration <= 0 {
		return nil, fmt.Errorf("system config %q: refresh.interval must be a positive duration, got %s", path, cfg.Refresh.Interval.Duration)
	}
	return &cfg, nil
}

// Graph is a single panel definition from the graphs configuration file.
type Graph struct {
	Name        string `yaml:"name"`
	Metric      string `yaml:"metric"`
	Description string `yaml:"description"`
}

// GraphsConfig is the root of the graphs configuration file.
type GraphsConfig struct {
	Graphs []Graph `yaml:"graphs"`
}

// LoadGraphs reads and validates the graphs configuration file. An empty
// graphs list is accepted and renders an empty dashboard.
func LoadGraphs(path string) (*GraphsConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read graphs config %q: %w", path, err)
	}
	var cfg GraphsConfig
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse graphs config %q: %w", path, err)
	}
	seen := make(map[string]bool, len(cfg.Graphs))
	for i, g := range cfg.Graphs {
		if g.Name == "" {
			return nil, fmt.Errorf("graphs config %q: graph #%d is missing a name", path, i+1)
		}
		if seen[g.Name] {
			return nil, fmt.Errorf("graphs config %q: duplicate graph name %q", path, g.Name)
		}
		seen[g.Name] = true
		if g.Metric == "" {
			return nil, fmt.Errorf("graphs config %q: graph %q is missing a metric", path, g.Name)
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
