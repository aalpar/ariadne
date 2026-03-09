.PHONY: build test lint format

build:
	go build ./...

test:
	go test -race ./...

lint:
	golangci-lint run ./...

format:
	gofmt -s -w .
