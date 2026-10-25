.PHONY: run test build lint

run:
	go run ./cmd/api

test:
	go test -race -count=1 ./...

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/api ./cmd/api

lint:
	golangci-lint run
