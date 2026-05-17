.PHONY: test build help

test:
	go test ./...

build:
	go build -o bin/griddyn ./cmd/griddyn

help:
	go run ./cmd/griddyn --help
