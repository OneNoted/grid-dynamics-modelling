.PHONY: test build help smoke

test:
	go test ./...

build:
	go build -o bin/griddyn ./cmd/griddyn

help:
	go run ./cmd/griddyn --help

smoke:
	scripts/e2e-smoke.sh
