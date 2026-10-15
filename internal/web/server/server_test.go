package server

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bazaruzero/yama/internal/web/agent"
	"github.com/bazaruzero/yama/internal/web/config"
)

func TestStaticAssetsServedFromOwnOrigin(t *testing.T) {
	h := staticHandler()
	ts := httptest.NewServer(h)
	defer ts.Close()

	for _, tc := range []struct {
		path        string
		contentType string
		contains    string
	}{
		{"/static/htmx.min.js", "text/javascript", "htmx"},
		{"/static/app.css", "text/css", "committed asset"},
	} {
		resp, err := http.Get(ts.URL + tc.path)
		if err != nil {
			t.Fatalf("GET %s: %v", tc.path, err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: status = %d, want 200", tc.path, resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, tc.contentType) {
			t.Fatalf("GET %s: content-type = %q, want prefix %q", tc.path, ct, tc.contentType)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), tc.contains) {
			t.Fatalf("GET %s: body missing %q", tc.path, tc.contains)
		}
	}

	resp, err := http.Get(ts.URL + "/static/other.txt")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown asset: status = %d, want 404", resp.StatusCode)
	}
}

func newTestServer(t *testing.T, agentHandler http.HandlerFunc, graphs []config.Graph) *httptest.Server {
	t.Helper()
	agentTS := httptest.NewServer(agentHandler)
	t.Cleanup(agentTS.Close)

	u, err := url.Parse(agentTS.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}

	interval := &config.Duration{Duration: 3 * time.Second}
	cfg := config.SystemConfig{
		Agent:   config.AgentConfig{Host: u.Hostname(), Port: port},
		Refresh: config.RefreshConfig{Interval: interval},
	}
	srv, err := NewServer(cfg, graphs, agent.NewClient(u.Hostname(), port, time.Second), nil)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func get(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200", url, resp.StatusCode)
	}
	var sb strings.Builder
	if _, err := io.Copy(&sb, resp.Body); err != nil {
		t.Fatal(err)
	}
	return sb.String()
}

// okAgent serves two fresh data points for any metric, relative to the
// current time so they always fall inside the one-hour window.
func okAgent(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	fmt.Fprintf(w, `[{"timestamp":%q,"value":1},{"timestamp":%q,"value":1}]`,
		now.Add(-2*time.Second).Format(time.RFC3339Nano), now.Add(-time.Second).Format(time.RFC3339Nano))
}

func TestDashboardRendersPanelsInConfigOrder(t *testing.T) {
	graphs := []config.Graph{
		{Name: "Zeta", Metric: config.MetricList{"m_z"}, Description: "last panel by config"},
		{Name: "Alpha", Metric: config.MetricList{"m_a"}, Description: "first panel by config"},
	}
	ts := newTestServer(t, okAgent, graphs)
	page := get(t, ts.URL+"/")

	if i, j := strings.Index(page, "<h2>Zeta</h2>"), strings.Index(page, "<h2>Alpha</h2>"); i > j || i < 0 || j < 0 {
		t.Errorf("panels not in config order (Zeta@%d, Alpha@%d)", i, j)
	}
	if !strings.Contains(page, `hx-get="/panels/Zeta"`) || !strings.Contains(page, `hx-get="/panels/Alpha"`) {
		t.Errorf("missing panel polling URLs:\n%s", page)
	}
	if !strings.Contains(page, `hx-trigger="every 3s"`) {
		t.Errorf("refresh interval not wired into hx-trigger:\n%s", page)
	}
	if !strings.Contains(page, "<svg") {
		t.Errorf("initial page render missing chart svg:\n%s", page)
	}
	if strings.Contains(page, "banner-down") {
		t.Errorf("banner shown with healthy agent:\n%s", page)
	}
}

func TestDashboardEmptyGraphs(t *testing.T) {
	ts := newTestServer(t, okAgent, nil)
	page := get(t, ts.URL+"/")
	if strings.Contains(page, "<h2>") {
		t.Errorf("unexpected panel in empty dashboard:\n%s", page)
	}
}

func TestDashboardOnlySameOriginAssets(t *testing.T) {
	ts := newTestServer(t, okAgent, []config.Graph{{Name: "A", Metric: config.MetricList{"m"}}})
	page := get(t, ts.URL+"/")

	attrRe := regexp.MustCompile(`(?:src|href)="([^"]+)"`)
	for _, match := range attrRe.FindAllStringSubmatch(page, -1) {
		v := match[1]
		if !strings.HasPrefix(v, "/") {
			t.Errorf("external asset reference %q; all src/href must be same-origin paths", v)
		}
	}
	for _, want := range []string{`src="/static/htmx.min.js"`, `href="/static/app.css"`} {
		if !strings.Contains(page, want) {
			t.Errorf("missing %s", want)
		}
	}
}

func TestPanelFragmentChart(t *testing.T) {
	graphs := []config.Graph{{Name: "My Panel", Metric: config.MetricList{"m1"}}}
	ts := newTestServer(t, okAgent, graphs)

	frag := get(t, ts.URL+"/panels/My%20Panel")
	if !strings.Contains(frag, "<svg") || !strings.Contains(frag, `class="line"`) {
		t.Errorf("fragment missing chart:\n%s", frag)
	}
	if !strings.Contains(frag, `hx-get="/panels/My%20Panel"`) || !strings.Contains(frag, `hx-trigger="every 3s"`) {
		t.Errorf("fragment missing polling attributes to continue refresh:\n%s", frag)
	}
}

func TestPanelFragmentNoData(t *testing.T) {
	agentHandler := func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "empty_metric") {
			fmt.Fprint(w, "[]")
			return
		}
		okAgent(w, r)
	}
	ts := newTestServer(t, agentHandler, []config.Graph{{Name: "Empty", Metric: config.MetricList{"empty_metric"}}, {Name: "Full", Metric: config.MetricList{"m"}}})

	frag := get(t, ts.URL+"/panels/Empty")
	if !strings.Contains(frag, "No data in the last hour") {
		t.Errorf("missing no-data state:\n%s", frag)
	}
	// A no-data panel is not a backend failure: the banner must stay clear.
	if banner := get(t, ts.URL+"/banner"); strings.Contains(banner, "unreachable") {
		t.Errorf("banner down for no-data metric:\n%s", banner)
	}
}

func TestUnknownPanelIs404(t *testing.T) {
	ts := newTestServer(t, okAgent, []config.Graph{{Name: "A", Metric: config.MetricList{"m"}}})
	resp, err := http.Get(ts.URL + "/panels/Nope")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

// TestMultiSeriesGraphRendersOnePanel verifies a graph listing several
// series renders a single panel with one line per series.
func TestMultiSeriesGraphRendersOnePanel(t *testing.T) {
	ts := newTestServer(t, okAgent, []config.Graph{
		{Name: "DB activity", Metric: config.MetricList{"db_stats.commits", "db_stats.reads"}},
	})
	page := get(t, ts.URL+"/")

	if got := strings.Count(page, `<path class="line"`); got != 2 {
		t.Errorf("line paths = %d, want 2 (one per series, one panel):\n%s", got, page)
	}
	if strings.Count(page, "<h2>") != 1 {
		t.Errorf("panels = %d, want 1", strings.Count(page, "<h2>"))
	}
	if !strings.Contains(page, "db_stats.commits: 1.00 · db_stats.reads: 1.00") {
		t.Errorf("missing per-series legend:\n%s", page)
	}
}

// TestMultiSeriesGraphWithEmptySeriesStillRenders verifies a series with no
// data keeps the panel in chart state with the other series drawn.
func TestMultiSeriesGraphWithEmptySeriesStillRenders(t *testing.T) {
	agentHandler := func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "empty_metric") {
			fmt.Fprint(w, "[]")
			return
		}
		okAgent(w, r)
	}
	ts := newTestServer(t, agentHandler, []config.Graph{
		{Name: "Mixed", Metric: config.MetricList{"m", "empty_metric"}},
	})

	frag := get(t, ts.URL+"/panels/Mixed")
	if !strings.Contains(frag, "<svg") {
		t.Errorf("panel must render a chart when any series has data:\n%s", frag)
	}
	if got := strings.Count(frag, `<path class="line"`); got != 1 {
		t.Errorf("line paths = %d, want 1 (only the populated series):\n%s", got, frag)
	}
	if !strings.Contains(frag, "m: 1.00") || !strings.Contains(frag, "empty_metric: —") {
		t.Errorf("legend must name both series:\n%s", frag)
	}
	if strings.Contains(frag, "No data in the last hour") {
		t.Errorf("panel with partial data must not show the no-data state:\n%s", frag)
	}
}

// TestMultiSeriesOneSeriesFailureIsErrorState verifies a failing series
// fetch (agent error on one name) fails the whole panel, never a
// half-rendered chart.
func TestMultiSeriesOneSeriesFailureIsErrorState(t *testing.T) {
	agentHandler := func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "bad_metric") {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		okAgent(w, r)
	}
	ts := newTestServer(t, agentHandler, []config.Graph{
		{Name: "Frag", Metric: config.MetricList{"m", "bad_metric"}},
	})

	frag := get(t, ts.URL+"/panels/Frag")
	if !strings.Contains(frag, "Data unavailable") {
		t.Errorf("panel must show error state when any series fetch fails:\n%s", frag)
	}
	if strings.Contains(frag, "<svg") {
		t.Errorf("panel must not render a half-chart on partial failure:\n%s", frag)
	}
	if banner := get(t, ts.URL+"/banner"); !strings.Contains(banner, "unreachable") {
		t.Errorf("banner must be down when a series fetch fails:\n%s", banner)
	}
}

func TestBannerShowsAndClearsOnAgentFailureAndRecovery(t *testing.T) {
	var healthy atomic.Bool
	healthy.Store(true)
	agentHandler := func(w http.ResponseWriter, r *http.Request) {
		if !healthy.Load() {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		okAgent(w, r)
	}
	ts := newTestServer(t, agentHandler, []config.Graph{{Name: "A", Metric: config.MetricList{"m"}}})

	if frag := get(t, ts.URL+"/panels/A"); !strings.Contains(frag, "<svg") {
		t.Fatalf("healthy agent: fragment missing chart:\n%s", frag)
	}
	if banner := get(t, ts.URL+"/banner"); strings.Contains(banner, "unreachable") {
		t.Fatalf("banner down while healthy:\n%s", banner)
	}

	// Agent goes down: panel fragment errors and the banner appears.
	healthy.Store(false)
	frag := get(t, ts.URL+"/panels/A")
	if !strings.Contains(frag, "Data unavailable") {
		t.Errorf("missing error state:\n%s", frag)
	}
	banner := get(t, ts.URL+"/banner")
	if !strings.Contains(banner, "unreachable") {
		t.Errorf("missing down banner:\n%s", banner)
	}

	// Agent recovers: without restarting the frontend, data resumes and the
	// banner clears.
	healthy.Store(true)
	if frag := get(t, ts.URL+"/panels/A"); !strings.Contains(frag, "<svg") {
		t.Errorf("recovered fragment missing chart:\n%s", frag)
	}
	if banner := get(t, ts.URL+"/banner"); strings.Contains(banner, "unreachable") {
		t.Errorf("banner still down after recovery:\n%s", banner)
	}
}

func TestDashboardWithAgentDownStillServesPage(t *testing.T) {
	agentTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer agentTS.Close()
	u, _ := url.Parse(agentTS.URL)
	port, _ := strconv.Atoi(u.Port())

	cfg := config.SystemConfig{Agent: config.AgentConfig{Host: u.Hostname(), Port: port}}
	srv, err := NewServer(cfg, []config.Graph{{Name: "A", Metric: config.MetricList{"m"}}}, agent.NewClient(u.Hostname(), port, 500*time.Millisecond), nil)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	page := get(t, ts.URL+"/")
	if !strings.Contains(page, "<h2>A</h2>") {
		t.Errorf("page missing panel header while agent down:\n%s", page)
	}
	if !strings.Contains(page, "Data unavailable") {
		t.Errorf("page missing error state while agent down:\n%s", page)
	}
	if !strings.Contains(page, "unreachable") {
		t.Errorf("page missing down banner while agent down:\n%s", page)
	}
}
