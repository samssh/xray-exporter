FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build

ARG TARGETOS TARGETARCH
ARG VERSION=dev COMMIT=none DATE=unknown

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
    -ldflags "-s -w -X main.buildVersion=${VERSION} -X main.buildCommit=${COMMIT} -X main.buildDate=${DATE}" \
    -o /out/xray-exporter .

FROM gcr.io/distroless/static-debian13:nonroot

EXPOSE 9550
COPY --from=build /out/xray-exporter /usr/bin/xray-exporter
ENTRYPOINT ["/usr/bin/xray-exporter"]
