BINARY := bin/yama

.PHONY: build test lint clean

build:
	CGO_ENABLED=0 go build -o $(BINARY) ./cmd/yama

test:
	go test ./...

lint:
	go vet ./...
	@test -z "$$(gofmt -l .)" || { echo "gofmt: files need formatting"; gofmt -l .; exit 1; }

clean:
	rm -rf bin/
