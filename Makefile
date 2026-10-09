.PHONY: build test lint snapshot docker dev

IMAGE ?= ghcr.io/samssh/xray-exporter

build:
	go build -o dist/xray-exporter .

test:
	go test -race ./...

lint:
	golangci-lint run ./...

# Builds release archives into dist/ without publishing.
snapshot:
	goreleaser release --snapshot --clean

docker:
	docker build -t "$(IMAGE):dev" .

dev:
	skaffold dev --port-forward --no-prune=false
