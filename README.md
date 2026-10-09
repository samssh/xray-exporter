# Xray Exporter

[![CI](https://github.com/samssh/xray-exporter/actions/workflows/ci.yml/badge.svg)][ci]
[![Release](https://img.shields.io/github/v/release/samssh/xray-exporter)][github-releases]

A Prometheus exporter that collects [Xray-core][xray-core] metrics over its gRPC [Stats API][stats-api].

This project started as a fork of [wi1dcard/v2ray-exporter][upstream].

- [Xray Exporter](#xray-exporter)
  - [Quick Start](#quick-start)
    - [Binaries](#binaries)
    - [Docker](#docker)
    - [Grafana Dashboard](#grafana-dashboard)
  - [Tutorial](#tutorial)
  - [Command Line Options](#command-line-options)
  - [Metrics](#metrics)
  - [Development](#development)
  - [Special Thanks](#special-thanks)
  - [License](#license)

![][grafana-screenshot]

[xray-core]: https://github.com/XTLS/Xray-core
[stats-api]: https://xtls.github.io/en/config/stats.html
[upstream]: https://github.com/wi1dcard/v2ray-exporter
[ci]: https://github.com/samssh/xray-exporter/actions/workflows/ci.yml
[grafana-screenshot]: https://i.loli.net/2020/06/12/KzjOnyu93VEIPiW.png

## Quick Start

### Binaries

Archives for Linux, macOS, FreeBSD and Windows (amd64, arm64, armv7) are published on the [releases][github-releases] page:

```bash
VERSION=x.y.z  # pick a release
wget "https://github.com/samssh/xray-exporter/releases/download/v${VERSION}/xray-exporter_${VERSION}_linux_amd64.tar.gz"
tar -xzf "xray-exporter_${VERSION}_linux_amd64.tar.gz" xray-exporter
sudo install xray-exporter /usr/local/bin/
```

### Docker

Multi-arch images (amd64, arm64, arm/v7) are published to the GitHub Container Registry:

```bash
docker run --rm -it ghcr.io/samssh/xray-exporter:latest --xray-endpoint "host.docker.internal:54321"
```

Tags: `latest` and `X.Y.Z` / `X.Y` follow releases, and `master` is built from the tip of the master branch.

### Grafana Dashboard

A simple Grafana dashboard is available [here][grafana-dashboard]. Please refer to the [Grafana docs][grafana-importing-dashboard] for how to import dashboards from JSON files.

## Tutorial

Before we start, let's assume you have already set up Prometheus and Grafana.

First, make sure the API and statistics features are enabled in your Xray config file. For example:

```json
{
    "stats": {},
    "api": {
        "tag": "api",
        "listen": "127.0.0.1:54321",
        "services": [
            "StatsService"
        ]
    },
    "policy": {
        "levels": {
            "0": {
                "statsUserUplink": true,
                "statsUserDownlink": true
            }
        },
        "system": {
            "statsInboundUplink": true,
            "statsInboundDownlink": true,
            "statsOutboundUplink": true,
            "statsOutboundDownlink": true
        }
    },
    "inbounds": [
        {
            "tag": "vless-in",
            "port": 12345,
            "protocol": "vless",
            "settings": {
                "decryption": "none",
                "clients": [
                    {
                        "email": "foo@example.com",
                        "id": "e731f153-4f31-49d3-9e8f-ff8f396135ef",
                        "level": 0
                    },
                    {
                        "email": "bar@example.com",
                        "id": "e731f153-4f31-49d3-9e8f-ff8f396135ee",
                        "level": 0
                    }
                ]
            }
        }
    ],
    "outbounds": [
        {
            "tag": "direct",
            "protocol": "freedom"
        }
    ]
}
```

The `api.listen` field makes Xray serve its gRPC API on `127.0.0.1:54321`, which is the endpoint the exporter scrapes. If you'd like to run Xray and the exporter on different machines, listen on a reachable address instead and be careful with the security risks: the API can also be used to change Xray's configuration.

Per-user statistics are only collected for clients that have an `email` and use a policy level with `statsUserUplink` / `statsUserDownlink` enabled. For more information, see the Xray docs for [stats][stats-api], [api][xray-api-docs] and [policy][xray-policy-docs].

Next, start the exporter:

```bash
xray-exporter --xray-endpoint "127.0.0.1:54321"
## Or
docker run --rm -d --network host ghcr.io/samssh/xray-exporter:latest --xray-endpoint "127.0.0.1:54321"
```

The logs show that the exporter is listening on the default address (`:9550`):

```plain
Xray Exporter 1.0.0-1a2b3c4 (built 2026-10-09T05:32:01Z)
time="2026-10-09T06:18:09Z" level=info msg="Server is ready to handle incoming scrape requests."
```

Use the `--listen` option to change the listen address or port. Open `http://IP:9550` in your browser and click `Scrape Xray Metrics`, and the exporter will expose Xray's runtime and statistics data in the Prometheus format, for example:

```
...
# HELP xray_up Indicate scrape succeeded or not
# TYPE xray_up gauge
xray_up 1
# HELP xray_uptime_seconds Xray uptime in seconds
# TYPE xray_uptime_seconds gauge
xray_uptime_seconds 150624
...
```

The exporter starts even when Xray is not reachable yet. If `xray_up` is `0`, the scrape failed; check the exporter logs (STDERR) for details.

Now let Prometheus scrape these metrics. Here is an example Prometheus configuration:

```yaml
global:
  scrape_interval: 15s
  scrape_timeout: 5s

scrape_configs:
  - job_name: xray
    metrics_path: /scrape
    static_configs:
      - targets: [IP:9550]
```

To learn more about Prometheus, please visit the [official docs][prometheus-docs].

## Command Line Options

```
Usage:
  xray-exporter [OPTIONS]

Application Options:
  -l, --listen=[ADDR]:PORT         Listen address (default: :9550)
  -m, --metrics-path=PATH          Metrics path (default: /scrape)
  -e, --xray-endpoint=HOST:PORT    Xray API endpoint (default: 127.0.0.1:8080)
  -t, --scrape-timeout=N           The timeout in seconds for every individual
                                   scrape (default: 3)
      --version                    Display the version and exit
```

Xray metrics are served on `--metrics-path` (`/scrape` by default). The exporter's own Go runtime metrics are served on `/metrics`.

## Metrics

The exporter intentionally doesn't keep Xray's original metric names, and follows the Prometheus [naming conventions][prometheus-naming] instead.

| Runtime Metric   | Exposed Metric                    |
| :--------------- | :-------------------------------- |
| `uptime`         | `xray_uptime_seconds`             |
| `num_goroutine`  | `xray_goroutines`                 |
| `alloc`          | `xray_memstats_alloc_bytes`       |
| `total_alloc`    | `xray_memstats_alloc_bytes_total` |
| `sys`            | `xray_memstats_sys_bytes`         |
| `mallocs`        | `xray_memstats_mallocs_total`     |
| `frees`          | `xray_memstats_frees_total`       |
| `live_objects`   | Removed. See the note below.      |
| `num_gc`         | `xray_memstats_num_gc`            |
| `pause_total_ns` | `xray_memstats_pause_total_ns`    |

| Statistic Metric                           | Exposed Metric                                                             |
| :----------------------------------------- | :------------------------------------------------------------------------- |
| `inbound>>>tag-name>>>traffic>>>uplink`    | `xray_traffic_uplink_bytes_total{dimension="inbound",target="tag-name"}`    |
| `inbound>>>tag-name>>>traffic>>>downlink`  | `xray_traffic_downlink_bytes_total{dimension="inbound",target="tag-name"}`  |
| `outbound>>>tag-name>>>traffic>>>uplink`   | `xray_traffic_uplink_bytes_total{dimension="outbound",target="tag-name"}`   |
| `outbound>>>tag-name>>>traffic>>>downlink` | `xray_traffic_downlink_bytes_total{dimension="outbound",target="tag-name"}` |
| `user>>>user-email>>>traffic>>>uplink`     | `xray_traffic_uplink_bytes_total{dimension="user",target="user-email"}`     |
| `user>>>user-email>>>traffic>>>downlink`   | `xray_traffic_downlink_bytes_total{dimension="user",target="user-email"}`   |

The exporter also exposes `xray_up`, `xray_scrape_duration_seconds` and `xray_scrapes_total` about the scrapes themselves.

- The value of `live_objects` can be calculated with `xray_memstats_mallocs_total - xray_memstats_frees_total`.

## Development

```bash
make test      # go test -race ./...
make lint      # golangci-lint run ./...
make build     # builds dist/xray-exporter
make snapshot  # cross-platform release archives via GoReleaser, without publishing
make docker    # builds the container image locally
```

Pushing a `v*` tag publishes release archives with GoReleaser and multi-arch images to GHCR. Every push to `master` publishes the `master` image.

## Special Thanks

- <https://github.com/wi1dcard/v2ray-exporter>
- <https://github.com/schweikert/fping-exporter>
- <https://github.com/oliver006/redis_exporter>

## License

MIT

[github-releases]: https://github.com/samssh/xray-exporter/releases
[xray-api-docs]: https://xtls.github.io/en/config/api.html
[xray-policy-docs]: https://xtls.github.io/en/config/policy.html
[prometheus-docs]: https://prometheus.io/docs/prometheus/latest/configuration/configuration/
[prometheus-naming]: https://prometheus.io/docs/practices/naming/
[grafana-dashboard]: ./dashboard.json
[grafana-importing-dashboard]: https://grafana.com/docs/grafana/latest/dashboards/build-dashboards/import-dashboards/
