package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeConfig(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadSystemDefaults(t *testing.T) {
	path := writeConfig(t, "webui.yaml", "agent:\n  host: \"pgbox\"\n")
	cfg, err := LoadSystem(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Agent.Host != "pgbox" {
		t.Errorf("host = %q, want pgbox", cfg.Agent.Host)
	}
	if cfg.Agent.Port != 8080 {
		t.Errorf("port = %d, want 8080", cfg.Agent.Port)
	}
	if got := cfg.Agent.EffectiveTimeout(); got != 5*time.Second {
		t.Errorf("timeout = %s, want 5s", got)
	}
	if cfg.UI.Listen != ":8081" {
		t.Errorf("listen = %q, want :8081", cfg.UI.Listen)
	}
	if got := cfg.Refresh.EffectiveInterval(); got != 10*time.Second {
		t.Errorf("interval = %s, want 10s", got)
	}
}

func TestLoadSystemFullySpecified(t *testing.T) {
	path := writeConfig(t, "webui.yaml", `
agent:
  host: "192.168.1.20"
  port: 9000
  timeout: 2s
ui:
  listen: "127.0.0.1:9999"
refresh:
  interval: 30s
`)
	cfg, err := LoadSystem(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Agent.Host != "192.168.1.20" || cfg.Agent.Port != 9000 {
		t.Errorf("agent = %s:%d", cfg.Agent.Host, cfg.Agent.Port)
	}
	if got := cfg.Agent.EffectiveTimeout(); got != 2*time.Second {
		t.Errorf("timeout = %s, want 2s", got)
	}
	if cfg.UI.Listen != "127.0.0.1:9999" {
		t.Errorf("listen = %q", cfg.UI.Listen)
	}
	if got := cfg.Refresh.EffectiveInterval(); got != 30*time.Second {
		t.Errorf("interval = %s, want 30s", got)
	}
}

func TestLoadSystemRejections(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name:    "zero interval",
			content: "refresh:\n  interval: 0s\n",
			wantErr: "refresh.interval",
		},
		{
			name:    "negative interval",
			content: "refresh:\n  interval: -3s\n",
			wantErr: "refresh.interval",
		},
		{
			name:    "zero timeout",
			content: "agent:\n  timeout: 0s\n",
			wantErr: "agent.timeout",
		},
		{
			name:    "negative timeout",
			content: "agent:\n  timeout: -1s\n",
			wantErr: "agent.timeout",
		},
		{
			name:    "unknown field",
			content: "agentx:\n  host: \"a\"\n",
			wantErr: "agentx",
		},
		{
			name:    "malformed yaml",
			content: "agent: [unclosed\n",
			wantErr: "parse system config",
		},
		{
			name:    "unparseable duration",
			content: "refresh:\n  interval: fast\n",
			wantErr: "invalid duration",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, "webui.yaml", tt.content)
			_, err := LoadSystem(path)
			if err == nil {
				t.Fatalf("LoadSystem succeeded, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
			if !strings.Contains(err.Error(), path) {
				t.Errorf("error = %q, want it to name the file %q", err, path)
			}
		})
	}
}

func TestLoadSystemMissingFile(t *testing.T) {
	_, err := LoadSystem(filepath.Join(t.TempDir(), "nope.yaml"))
	if err == nil {
		t.Fatal("want error for missing file")
	}
}

func TestLoadGraphsAllFields(t *testing.T) {
	path := writeConfig(t, "graphs.yaml", `
graphs:
  - name: "Connectivity"
    metric: select_1
    description: "SELECT 1 check"
  - name: "Connections"
    metric: pg_connections
`)
	cfg, err := LoadGraphs(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Graphs) != 2 {
		t.Fatalf("graphs = %d, want 2", len(cfg.Graphs))
	}
	g := cfg.Graphs[0]
	if g.Name != "Connectivity" || g.Metric != "select_1" || g.Description != "SELECT 1 check" {
		t.Errorf("graph[0] = %+v", g)
	}
	if cfg.Graphs[1].Description != "" {
		t.Errorf("graph[1].description = %q, want empty default", cfg.Graphs[1].Description)
	}
}

func TestLoadGraphsRejections(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name:    "duplicate names",
			content: "graphs:\n  - name: A\n    metric: m1\n  - name: A\n    metric: m2\n",
			wantErr: `duplicate graph name "A"`,
		},
		{
			name:    "missing metric",
			content: "graphs:\n  - name: A\n",
			wantErr: `graph "A" is missing a metric`,
		},
		{
			name:    "missing name",
			content: "graphs:\n  - metric: m1\n",
			wantErr: "missing a name",
		},
		{
			name:    "unknown field",
			content: "graphs:\n  - name: A\n    metric: m1\n    unit: q\n",
			wantErr: "unit",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, "graphs.yaml", tt.content)
			_, err := LoadGraphs(path)
			if err == nil {
				t.Fatalf("LoadGraphs succeeded, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
			if !strings.Contains(err.Error(), path) {
				t.Errorf("error = %q, want it to name the file %q", err, path)
			}
		})
	}
}

func TestLoadGraphsEmptyList(t *testing.T) {
	path := writeConfig(t, "graphs.yaml", "graphs: []\n")
	cfg, err := LoadGraphs(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Graphs) != 0 {
		t.Errorf("graphs = %d, want 0", len(cfg.Graphs))
	}
}

func TestLoadGraphsMissingFile(t *testing.T) {
	_, err := LoadGraphs(filepath.Join(t.TempDir(), "nope.yaml"))
	if err == nil {
		t.Fatal("want error for missing file")
	}
}
