package server

import (
	"embed"
	"net/http"
)

//go:embed assets/htmx.min.js assets/app.css
var assetsFS embed.FS

// staticHandler serves the embedded page assets at fixed same-origin paths.
// Only the two vendored files are exposed.
func staticHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /static/htmx.min.js", func(w http.ResponseWriter, r *http.Request) {
		serveAsset(w, "htmx.min.js", "text/javascript; charset=utf-8")
	})
	mux.HandleFunc("GET /static/app.css", func(w http.ResponseWriter, r *http.Request) {
		serveAsset(w, "app.css", "text/css; charset=utf-8")
	})
	return mux
}

func serveAsset(w http.ResponseWriter, name, contentType string) {
	b, err := assetsFS.ReadFile("assets/" + name)
	if err != nil {
		http.Error(w, "asset not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(b)
}
