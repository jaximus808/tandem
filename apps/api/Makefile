.PHONY: build run test tidy dist

# Host-platform binary with the embedded web bundle → ../../dist/tandem
dist:
	../../scripts/build.sh

# API only (no web bundle refresh): quick local build
build:
	go build -o bin/tandem ./cmd/server

# Local mode, data under ~/.tandem; set WEB_DIST_PATH=../../apps/web/dist to
# serve a freshly built web bundle instead of the embedded one.
run:
	go run ./cmd/server

test:
	go test ./...

tidy:
	go mod tidy
