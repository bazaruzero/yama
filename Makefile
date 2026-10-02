BINARY := bin/yama
WEB_BINARY := bin/yama-web

.PHONY: build test lint css clean

build:
	CGO_ENABLED=0 go build -o $(BINARY) ./cmd/yama
	CGO_ENABLED=0 go build -o $(WEB_BINARY) ./cmd/yama-web

test:
	go test ./...

lint:
	go vet ./...
	@test -z "$$(gofmt -l .)" || { echo "gofmt: files need formatting"; gofmt -l .; exit 1; }

# Optional theme workflow: recompiles the committed app.css with the
# Tailwind standalone CLI (https://tailwindcss.com/blog/standalone-cli)
# when both the CLI and a source stylesheet are available. No-op otherwise.
css:
	@if command -v tailwindcss >/dev/null 2>&1 && [ -f internal/web/server/assets/tailwind.src.css ]; then \
		tailwindcss -i internal/web/server/assets/tailwind.src.css -o internal/web/server/assets/app.css --minify; \
	else \
		echo "css: skipped (requires the tailwindcss standalone CLI and internal/web/server/assets/tailwind.src.css); using committed app.css"; \
	fi

clean:
	rm -rf bin/
