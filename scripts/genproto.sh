#!/usr/bin/env bash
# Fetches the Xray-core API protos for XRAY_VERSION into proto/ and generates
# Go code for them into internal/xrayapi/. Requires protoc and Go.
set -euo pipefail

XRAY_VERSION="${XRAY_VERSION:?set XRAY_VERSION, e.g. v26.9.30}"
PROTOC_GEN_GO_VERSION=v1.36.12
PROTOC_GEN_GO_GRPC_VERSION=v1.6.2

MODULE=github.com/samssh/xray-exporter
OUT_PKG=$MODULE/internal/xrayapi

# The services the exporter calls, and every file they import.
# If protoc reports a missing import after a bump, add it here.
FILES=(
  app/stats/command/command.proto
  app/proxyman/command/command.proto
  app/observatory/command/command.proto
  app/observatory/config.proto
  app/router/command/command.proto
  common/net/network.proto
  common/protocol/user.proto
  common/serial/typed_message.proto
  core/config.proto
)

cd "$(dirname "$0")/.."

rm -rf proto internal/xrayapi
mkdir -p proto
for f in "${FILES[@]}"; do
  mkdir -p "proto/$(dirname "$f")"
  curl -fsSL "https://raw.githubusercontent.com/XTLS/Xray-core/$XRAY_VERSION/$f" -o "proto/$f"
done
curl -fsSL "https://raw.githubusercontent.com/XTLS/Xray-core/$XRAY_VERSION/LICENSE" -o proto/LICENSE
echo "$XRAY_VERSION" > proto/XRAY_VERSION

bin="$(mktemp -d)"
trap 'rm -rf "$bin"' EXIT
GOBIN="$bin" go install "google.golang.org/protobuf/cmd/protoc-gen-go@$PROTOC_GEN_GO_VERSION"
GOBIN="$bin" go install "google.golang.org/grpc/cmd/protoc-gen-go-grpc@$PROTOC_GEN_GO_GRPC_VERSION"

go_opts=(--go_opt=module=$MODULE --go-grpc_opt=module=$MODULE)
for f in "${FILES[@]}"; do
  go_opts+=("--go_opt=M$f=$OUT_PKG/$(dirname "$f")" "--go-grpc_opt=M$f=$OUT_PKG/$(dirname "$f")")
done

protoc -I proto \
  --plugin=protoc-gen-go="$bin/protoc-gen-go" \
  --plugin=protoc-gen-go-grpc="$bin/protoc-gen-go-grpc" \
  --go_out=. --go-grpc_out=. \
  "${go_opts[@]}" \
  "${FILES[@]}"
