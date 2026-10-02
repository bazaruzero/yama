// Package server serves the dashboard web UI: panels defined by the graphs
// config, refreshed in place by htmx polling panel fragments from this
// server, which fetches metric data from the agent server-side.
package server

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/bazaruzero/yama/internal/web/agent"
	"github.com/bazaruzero/yama/internal/web/chart"
	"github.com/bazaruzero/yama/internal/web/config"
)

//go:embed templates/*.tmpl
var templatesFS embed.FS

// displayWindow is the fixed rolling time window each panel shows.
const displayWindow = time.Hour

// Panel body states.
const (
	stateChart  = "chart"
	stateNoData = "nodata"
	stateError  = "error"
)

// connectivity tracks whether the last agent interaction cycle succeeded.
// It is coarse by design: the dashboard talks to a single agent, so any
// panel failure marks the backend as unreachable until any fetch succeeds.
type connectivity struct {
	mu sync.Mutex
	ok bool
}

func (c *connectivity) set(ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ok = ok
}

func (c *connectivity) healthy() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ok
}

// Server renders the dashboard and proxies metric data from the agent.
type Server struct {
	cfg    config.SystemConfig
	graphs []config.Graph
	client *agent.Client
	log    *slog.Logger
	now    func() time.Time
	conn   connectivity
	tmpl   *template.Template
}

// NewServer creates the dashboard server. The agent client must be
// preconfigured with the agent address and timeout.
func NewServer(cfg config.SystemConfig, graphs []config.Graph, client *agent.Client, log *slog.Logger) (*Server, error) {
	if log == nil {
		log = slog.Default()
	}
	tmpl, err := template.ParseFS(templatesFS, "templates/*.tmpl")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	return &Server{
		cfg:    cfg,
		graphs: graphs,
		client: client,
		log:    log,
		now:    func() time.Time { return time.Now().UTC() },
		tmpl:   tmpl,
	}, nil
}

// Handler returns the routed HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleDashboard)
	mux.HandleFunc("GET /panels/{name}", s.handlePanel)
	mux.HandleFunc("GET /banner", s.handleBanner)
	mux.Handle("GET /static/", staticHandler())
	return mux
}

type panelView struct {
	Graph    config.Graph
	PanelURL string
	Interval string
	State    string
	Chart    template.HTML
}

type dashboardView struct {
	Interval    string
	BackendDown bool
	Panels      []panelView
}

// handleDashboard renders the full page with the initial panel content,
// fetched concurrently so a slow agent cannot serialize page delivery.
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	interval := s.cfg.Refresh.EffectiveInterval()
	views := make([]panelView, len(s.graphs))
	var wg sync.WaitGroup
	for i, g := range s.graphs {
		wg.Add(1)
		go func(i int, g config.Graph) {
			defer wg.Done()
			views[i] = s.fetchPanel(r.Context(), g, interval)
		}(i, g)
	}
	wg.Wait()

	down := false
	for _, v := range views {
		if v.State == stateError {
			down = true
		}
	}
	s.conn.set(!down)
	s.render(w, http.StatusOK, "dashboard.tmpl", dashboardView{
		Interval:    interval.String(),
		BackendDown: down,
		Panels:      views,
	})
}

// handlePanel returns one panel body fragment: chart, no-data, or error
// state. The fragment carries its own htmx polling attributes so the swap
// keeps refreshing in place. Any fetch result updates the connectivity
// state that drives the banner.
func (s *Server) handlePanel(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	g, ok := s.findGraph(name)
	if !ok {
		http.Error(w, "unknown panel", http.StatusNotFound)
		return
	}
	view := s.fetchPanel(r.Context(), g, s.cfg.Refresh.EffectiveInterval())
	s.conn.set(view.State != stateError)
	s.render(w, http.StatusOK, "panelbody.tmpl", view)
}

// handleBanner returns the connectivity banner fragment: a visible
// indication when the agent is unreachable, empty when healthy.
func (s *Server) handleBanner(w http.ResponseWriter, r *http.Request) {
	s.render(w, http.StatusOK, "banner.tmpl", !s.conn.healthy())
}

// fetchPanel builds the panel view for one graph, fetching the metric's
// points for the fixed last-hour window from the agent.
func (s *Server) fetchPanel(ctx context.Context, g config.Graph, interval time.Duration) panelView {
	view := panelView{
		Graph:    g,
		PanelURL: "/panels/" + url.PathEscape(g.Name),
		Interval: interval.String(),
		State:    stateError,
	}
	to := s.now().UTC()
	from := to.Add(-displayWindow)
	points, err := s.client.Points(ctx, g.Metric, from, to)
	if err != nil {
		s.log.Debug("panel fetch failed", "graph", g.Name, "metric", g.Metric, "error", err)
		return view
	}
	window := chart.Options{From: from, To: to}
	if len(chart.Clip(points, window)) == 0 {
		view.State = stateNoData
		return view
	}
	view.State = stateChart
	view.Chart = template.HTML(chart.Render(points, window))
	return view
}

func (s *Server) findGraph(name string) (config.Graph, bool) {
	for _, g := range s.graphs {
		if g.Name == name {
			return g, true
		}
	}
	return config.Graph{}, false
}

func (s *Server) render(w http.ResponseWriter, status int, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := s.tmpl.ExecuteTemplate(w, name, data); err != nil {
		s.log.Error("template render failed", "template", name, "error", err)
	}
}
