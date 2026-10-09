package collector

import (
	"context"
	"fmt"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"

	"github.com/samssh/xray-exporter/internal/xrayapi/app/stats/command"
)

func newDesc(name, help string, labels ...string) *prometheus.Desc {
	return prometheus.NewDesc(prometheus.BuildFQName(namespace, "", name), help, labels, nil)
}

type trafficCollector struct {
	client   command.StatsServiceClient
	uplink   *prometheus.Desc
	downlink *prometheus.Desc
}

func newTrafficCollector(conn *grpc.ClientConn, _ Options) (collector, error) {
	return &trafficCollector{
		client:   command.NewStatsServiceClient(conn),
		uplink:   newDesc("traffic_uplink_bytes_total", "Number of transmitted bytes", "dimension", "target"),
		downlink: newDesc("traffic_downlink_bytes_total", "Number of received bytes", "dimension", "target"),
	}, nil
}

func (c *trafficCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.uplink
	ch <- c.downlink
}

func (c *trafficCollector) Collect(ctx context.Context, ch chan<- prometheus.Metric) error {
	resp, err := c.client.QueryStats(ctx, &command.QueryStatsRequest{Reset_: false})
	if err != nil {
		return fmt.Errorf("failed to get stats: %w", err)
	}

	for _, s := range resp.GetStat() {
		// example value: inbound>>>socks-proxy>>>traffic>>>uplink
		p := strings.Split(s.GetName(), ">>>")
		if len(p) != 4 || p[2] != "traffic" {
			logrus.Debugf("Skipping stat with unexpected name: %q", s.GetName())
			continue
		}

		var desc *prometheus.Desc
		switch p[3] {
		case "uplink":
			desc = c.uplink
		case "downlink":
			desc = c.downlink
		default:
			logrus.Debugf("Skipping stat with unexpected name: %q", s.GetName())
			continue
		}

		ch <- prometheus.MustNewConstMetric(desc, prometheus.CounterValue, float64(s.GetValue()), p[0], p[1])
	}

	return nil
}

type runtimeCollector struct {
	client command.StatsServiceClient
	descs  map[string]*prometheus.Desc
}

func newRuntimeCollector(conn *grpc.ClientConn, _ Options) (collector, error) {
	c := &runtimeCollector{
		client: command.NewStatsServiceClient(conn),
		descs:  map[string]*prometheus.Desc{},
	}

	// We followed the naming style of Go collector from Prometheus.
	// See: https://github.com/prometheus/client_golang/blob/main/prometheus/go_collector.go
	// Metrics not exposed by the Go collector only get the "memstats_" prefix.
	for name, help := range map[string]string{
		"uptime_seconds":             "Xray uptime in seconds",
		"goroutines":                 "Number of goroutines that currently exist",
		"memstats_alloc_bytes":       "Number of bytes allocated and still in use",
		"memstats_alloc_bytes_total": "Total number of bytes allocated, even if freed",
		"memstats_sys_bytes":         "Number of bytes obtained from system",
		"memstats_mallocs_total":     "Total number of mallocs",
		"memstats_frees_total":       "Total number of frees",
		"memstats_num_gc":            "Number of completed GC cycles",
		"memstats_pause_total_ns":    "Cumulative nanoseconds in GC stop-the-world pauses",
	} {
		c.descs[name] = newDesc(name, help)
	}

	return c, nil
}

func (c *runtimeCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range c.descs {
		ch <- d
	}
}

func (c *runtimeCollector) Collect(ctx context.Context, ch chan<- prometheus.Metric) error {
	resp, err := c.client.GetSysStats(ctx, &command.SysStatsRequest{})
	if err != nil {
		return fmt.Errorf("failed to get sys stats: %w", err)
	}

	gauge := func(name string, v float64) {
		ch <- prometheus.MustNewConstMetric(c.descs[name], prometheus.GaugeValue, v)
	}
	counter := func(name string, v float64) {
		ch <- prometheus.MustNewConstMetric(c.descs[name], prometheus.CounterValue, v)
	}

	gauge("uptime_seconds", float64(resp.GetUptime()))
	gauge("goroutines", float64(resp.GetNumGoroutine()))
	gauge("memstats_alloc_bytes", float64(resp.GetAlloc()))
	counter("memstats_alloc_bytes_total", float64(resp.GetTotalAlloc()))
	gauge("memstats_sys_bytes", float64(resp.GetSys()))
	counter("memstats_mallocs_total", float64(resp.GetMallocs()))
	counter("memstats_frees_total", float64(resp.GetFrees()))
	gauge("memstats_num_gc", float64(resp.GetNumGC()))
	gauge("memstats_pause_total_ns", float64(resp.GetPauseTotalNs()))

	// live_objects is not exported. Calculate it in Prometheus using:
	// xray_memstats_mallocs_total - xray_memstats_frees_total
	// See: https://prometheus.io/docs/instrumenting/writing_exporters/#drop-less-useful-statistics

	return nil
}
