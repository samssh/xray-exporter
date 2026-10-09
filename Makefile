.PHONY: build test lint proto snapshot docker dev

IMAGE ?= ghcr.io/samssh/xray-exporter
XRAY_VERSION ?= $(shell cat proto/XRAY_VERSION)

build:
	go build -o dist/xray-exporter .

test:
	go test -race ./...

lint:
	golangci-lint run ./...

# Re-fetches the Xray API protos and regenerates internal/xrayapi.
# Bump with: make proto XRAY_VERSION=vX.Y.Z
proto:
	XRAY_VERSION=$(XRAY_VERSION) ./scripts/genproto.sh

# Builds release archives into dist/ without publishing.
snapshot:
	goreleaser release --snapshot --clean

docker:
	docker build -t "$(IMAGE):dev" .

dev:
	skaffold dev --port-forward --no-prune=false
