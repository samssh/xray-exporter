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
    - [Collectors](#collectors)
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
# HELP xray_up Whether all enabled collectors succeeded
# TYPE xray_up gauge
xray_up 1
# HELP xray_uptime_seconds Xray uptime in seconds
# TYPE xray_uptime_seconds gauge
xray_uptime_seconds 150624
...
```

The exporter starts even when Xray is not reachable yet. If `xray_up` is `0`, at least one collector failed; `xray_collector_up` shows which one, and the exporter logs (STDERR) say why.

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
usage: xray-exporter [<flags>]

Flags:
  -h, --[no-]help               Show context-sensitive help.
      --[no-]version            Show application version.
  -l, --listen=":9550"          Listen address ($XRAY_EXPORTER_LISTEN)
  -m, --metrics-path="/scrape"  Path that serves Xray metrics ($XRAY_EXPORTER_METRICS_PATH)
  -e, --xray-endpoint="127.0.0.1:8080"
                                Xray API endpoint ($XRAY_EXPORTER_XRAY_ENDPOINT)
  -t, --scrape-timeout=3        The timeout in seconds for every individual scrape ($XRAY_EXPORTER_SCRAPE_TIMEOUT)
      --log.level=info          Log level: debug, info, warn or error ($XRAY_EXPORTER_LOG_LEVEL)
      --[no-]collector.traffic  Enable the traffic collector (default: true)
      --[no-]collector.runtime  Enable the runtime collector (default: true)
      --[no-]collector.online   Enable the online collector (default: false)
      --[no-]collector.handler  Enable the handler collector (default: false)
      --[no-]collector.observatory
                                Enable the observatory collector (default: false)
      --[no-]collector.balancer Enable the balancer collector (default: false)
      --[no-]collector.online.ips
                                Export one series per online user and IP (high cardinality)
      --collector.balancer.tag=TAG ...
                                Tags of balancers to report on, comma-separated or repeated
```

Every flag can also be set with an environment variable, shown in `xray-exporter --help`. Collector flags use `XRAY_EXPORTER_COLLECTOR_<NAME>`, for example `XRAY_EXPORTER_COLLECTOR_ONLINE=true`.

Xray metrics are served on `--metrics-path` (`/scrape` by default). The exporter's own Go runtime metrics are served on `/metrics`.

### Collectors

Each group of metrics comes from a collector that can be turned on with `--collector.<name>` or off with `--no-collector.<name>`. Each collector needs the matching service in Xray's `api.services`.

| Collector     | Default | Xray service         | Metrics                                                    |
| :------------ | :------ | :------------------- | :--------------------------------------------------------- |
| `traffic`     | on      | `StatsService`       | Traffic counters per inbound, outbound and user            |
| `runtime`     | on      | `StatsService`       | Xray uptime and Go runtime stats                           |
| `online`      | off     | `StatsService`       | Online users and the number of IPs each one is online from |
| `handler`     | off     | `HandlerService`     | Users per inbound and the protocol of each outbound        |
| `observatory` | off     | `ObservatoryService` | Outbound health from `observatory` or `burstObservatory`   |
| `balancer`    | off     | `RoutingService`     | Outbounds selected by balancers                            |

If a collector fails, for example because its service isn't enabled in Xray, the other collectors still report their metrics and the exporter logs a warning.

To enable everything, list all the services in Xray's config:

```json
"api": {
    "tag": "api",
    "listen": "127.0.0.1:54321",
    "services": ["StatsService", "HandlerService", "ObservatoryService", "RoutingService"]
}
```

and start the exporter with:

```bash
xray-exporter --xray-endpoint "127.0.0.1:54321" \
  --collector.online --collector.handler --collector.observatory \
  --collector.balancer --collector.balancer.tag "my-balancer"
```

## Metrics

The exporter intentionally doesn't keep Xray's original metric names, and follows the Prometheus [naming conventions][prometheus-naming] instead.

### Scrape health

| Metric                                             | Description                                  |
| :------------------------------------------------- | :------------------------------------------- |
| `xray_up`                                          | `1` if all enabled collectors succeeded      |
| `xray_collector_up{collector="..."}`               | `1` if the collector succeeded               |
| `xray_collector_duration_seconds{collector="..."}` | How long the collector took                  |
| `xray_scrape_duration_seconds`                     | How long the whole scrape took               |
| `xray_scrapes_total`                               | Number of scrapes since the exporter started |

### `runtime` collector

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

- The value of `live_objects` can be calculated with `xray_memstats_mallocs_total - xray_memstats_frees_total`.

### `traffic` collector

| Statistic Metric                           | Exposed Metric                                                             |
| :----------------------------------------- | :------------------------------------------------------------------------- |
| `inbound>>>tag-name>>>traffic>>>uplink`    | `xray_traffic_uplink_bytes_total{dimension="inbound",target="tag-name"}`    |
| `inbound>>>tag-name>>>traffic>>>downlink`  | `xray_traffic_downlink_bytes_total{dimension="inbound",target="tag-name"}`  |
| `outbound>>>tag-name>>>traffic>>>uplink`   | `xray_traffic_uplink_bytes_total{dimension="outbound",target="tag-name"}`   |
| `outbound>>>tag-name>>>traffic>>>downlink` | `xray_traffic_downlink_bytes_total{dimension="outbound",target="tag-name"}` |
| `user>>>user-email>>>traffic>>>uplink`     | `xray_traffic_uplink_bytes_total{dimension="user",target="user-email"}`     |
| `user>>>user-email>>>traffic>>>downlink`   | `xray_traffic_downlink_bytes_total{dimension="user",target="user-email"}`   |

### `online` collector

Online users are only tracked when the user's policy level has `statsUserOnline` enabled, and the user has an `email`:

```json
"policy": {
    "levels": {
        "0": {
            "statsUserUplink": true,
            "statsUserDownlink": true,
            "statsUserOnline": true
        }
    }
}
```

| Metric                                                          | Description                                                     |
| :-------------------------------------------------------------- | :-------------------------------------------------------------- |
| `xray_online_users`                                             | Number of users with at least one online IP                     |
| `xray_user_online_ips{user="..."}`                              | Number of IPs the user is online from                           |
| `xray_user_online_ip_last_seen_timestamp_seconds{user,ip}`      | When the user last opened a connection from the IP, in Unix time |

- A user's series disappear when they go offline.
- `xray_user_online_ip_last_seen_timestamp_seconds` is only exported with `--collector.online.ips`, because it creates one series per user and IP.
- Xray only counts connections that are open, and ignores connections from localhost.
- On Xray v26.4.13 and newer, the collector reads everything with one API call. On older versions it makes one call per online user. Xray v26.1.13 or newer is required.

### `handler` collector

| Metric                                      | Description                                                          |
| :------------------------------------------ | :------------------------------------------------------------------- |
| `xray_inbound_users{inbound="..."}`         | Number of users configured on the inbound. Not the number online.    |
| `xray_outbound_info{outbound,protocol}`     | Always `1`. Lists the outbounds and their protocols, such as `vless` |

Inbounds without users, such as `dokodemo-door`, are skipped.

### `observatory` collector

Works with both [`observatory`][xray-observatory-docs] and [`burstObservatory`][xray-burst-observatory-docs]. Xray needs one of them configured to start `ObservatoryService`.

| Metric                                                          | Description                                                                     |
| :-------------------------------------------------------------- | :------------------------------------------------------------------------------ |
| `xray_observatory_outbound_up{outbound="..."}`                  | `1` if the outbound's last probe succeeded                                      |
| `xray_observatory_outbound_delay_seconds{outbound="..."}`       | Probe delay. With `burstObservatory`, the average RTT. Only set while up        |
| `xray_observatory_outbound_last_seen_timestamp_seconds{...}`    | When a probe last succeeded, in Unix time (`observatory` only)                  |
| `xray_observatory_outbound_last_try_timestamp_seconds{...}`     | When the outbound was last probed, in Unix time (`observatory` only)            |
| `xray_observatory_health_ping_probes{outbound="..."}`           | Probes in the sampling window (`burstObservatory` only)                         |
| `xray_observatory_health_ping_failed_probes{outbound="..."}`    | Failed probes in the sampling window (`burstObservatory` only)                  |
| `xray_observatory_health_ping_rtt_{average,min,max,deviation}_seconds{...}` | RTT stats in the sampling window (`burstObservatory` only, and only if a probe succeeded) |

### `balancer` collector

Xray's API can't list balancers, so name them with `--collector.balancer.tag`.

| Metric                                         | Description                                                                         |
| :--------------------------------------------- | :---------------------------------------------------------------------------------- |
| `xray_balancer_selected{balancer,outbound}`    | Always `1`. The outbounds the strategy currently prefers (`leastPing` and `leastLoad` only) |
| `xray_balancer_override{balancer,outbound}`    | Always `1`. The outbound the balancer was manually overridden to, if any            |

## Development

```bash
make test      # go test -race ./...
make lint      # golangci-lint run ./...
make build     # builds dist/xray-exporter
make proto     # re-fetches Xray's API protos and regenerates internal/xrayapi
make snapshot  # cross-platform release archives via GoReleaser, without publishing
make docker    # builds the container image locally
```

The exporter talks to Xray through Go code generated from Xray's own `.proto` files, pinned to the version in [`proto/XRAY_VERSION`](proto/XRAY_VERSION). To move to a newer Xray API, run `make proto XRAY_VERSION=vX.Y.Z` (needs `protoc`) and commit the result.

Pushing a `v*` tag publishes release archives with GoReleaser and multi-arch images to GHCR. Every push to `master` publishes the `master` image.

## Special Thanks

- <https://github.com/wi1dcard/v2ray-exporter>
- <https://github.com/schweikert/fping-exporter>
- <https://github.com/oliver006/redis_exporter>

## License

MIT, except for the files in [`proto/`](proto) and [`internal/xrayapi/`](internal/xrayapi), which are copied or generated from [Xray-core][xray-core] and are licensed under the [MPL-2.0](proto/LICENSE).

[github-releases]: https://github.com/samssh/xray-exporter/releases
[xray-api-docs]: https://xtls.github.io/en/config/api.html
[xray-policy-docs]: https://xtls.github.io/en/config/policy.html
[xray-observatory-docs]: https://xtls.github.io/en/config/observatory.html
[xray-burst-observatory-docs]: https://xtls.github.io/en/config/observatory.html#burstobservatoryobject
[prometheus-docs]: https://prometheus.io/docs/prometheus/latest/configuration/configuration/
[prometheus-naming]: https://prometheus.io/docs/practices/naming/
[grafana-dashboard]: ./dashboard.json
[grafana-importing-dashboard]: https://grafana.com/docs/grafana/latest/dashboards/build-dashboards/import-dashboards/
