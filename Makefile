.PHONY: build test fmt scan explain deps

BINARY := compatibility-lab

build:
	go build -o bin/$(BINARY) ./cmd/compatibility-lab

test:
	go test ./...

fmt:
	go fmt ./...

scan:
	go run ./cmd/compatibility-lab scan

explain:
	go run ./cmd/compatibility-lab explain routing-queue

deps:
	go run ./cmd/compatibility-lab dependency-closure routing-queue
