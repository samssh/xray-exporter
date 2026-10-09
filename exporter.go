package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

const namespace = "xray"

// collector gathers one group of metrics from the Xray API.
type collector interface {
	Describe(ch chan<- *prometheus.Desc)
	// Collect sends metrics to ch, and returns an error if the data could not be fetched.
	Collect(ctx context.Context, ch chan<- prometheus.Metric) error
}

type collectorInfo struct {
	name             string
	help             string
	enabledByDefault bool
	new              func(conn *grpc.ClientConn) collector
}

// collectors lists every available collector, in the order they are documented.
var collectors = []collectorInfo{
	{"traffic", "Traffic counters per inbound, outbound and user (StatsService)", true, newTrafficCollector},
	{"runtime", "Xray uptime and Go runtime stats (StatsService)", true, newRuntimeCollector},
}

var (
	upDesc = prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "up"),
		"Whether all enabled collectors succeeded", nil, nil)
	scrapeDurationDesc = prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "scrape_duration_seconds"),
		"Scrape duration in seconds", nil, nil)
	collectorUpDesc = prometheus.NewDesc(prometheus.BuildFQName(namespace, "collector", "up"),
		"Whether the collector succeeded", []string{"collector"}, nil)
	collectorDurationDesc = prometheus.NewDesc(prometheus.BuildFQName(namespace, "collector", "duration_seconds"),
		"Collector duration in seconds", []string{"collector"}, nil)
)

type Exporter struct {
	sync.Mutex
	scrapeTimeout time.Duration
	registry      *prometheus.Registry
	totalScrapes  prometheus.Counter
	conn          *grpc.ClientConn
	collectors    map[string]collector
}

// NewExporter creates an exporter for the Xray API at endpoint that runs the
// named collectors. The gRPC connection is established lazily, so Xray does
// not need to be running yet.
func NewExporter(endpoint string, scrapeTimeout time.Duration, enabled []string, dialOpts ...grpc.DialOption) (*Exporter, error) {
	dialOpts = append([]grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}, dialOpts...)
	conn, err := grpc.NewClient(endpoint, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC client for %q: %w", endpoint, err)
	}

	e := Exporter{
		scrapeTimeout: scrapeTimeout,
		registry:      prometheus.NewRegistry(),
		totalScrapes: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "scrapes_total",
			Help:      "Total number of scrapes performed",
		}),
		conn:       conn,
		collectors: map[string]collector{},
	}

	for _, name := range enabled {
		info, ok := findCollector(name)
		if !ok {
			_ = conn.Close()
			return nil, fmt.Errorf("unknown collector %q", name)
		}
		e.collectors[name] = info.new(conn)
	}

	e.registry.MustRegister(&e)

	return &e, nil
}

func findCollector(name string) (collectorInfo, bool) {
	for _, c := range collectors {
		if c.name == name {
			return c, true
		}
	}
	return collectorInfo{}, false
}

// Close releases the underlying gRPC connection.
func (e *Exporter) Close() error {
	return e.conn.Close()
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- upDesc
	ch <- scrapeDurationDesc
	ch <- collectorUpDesc
	ch <- collectorDurationDesc
	ch <- e.totalScrapes.Desc()

	for _, c := range e.collectors {
		c.Describe(ch)
	}
}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	e.Lock()
	defer e.Unlock()
	e.totalScrapes.Inc()

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), e.scrapeTimeout)
	defer cancel()

	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		failed bool
	)
	for name, c := range e.collectors {
		wg.Go(func() {
			ok := e.runCollector(ctx, name, c, ch)
			if !ok {
				mu.Lock()
				failed = true
				mu.Unlock()
			}
		})
	}
	wg.Wait()

	up := 1.0
	if failed {
		up = 0
	}
	ch <- prometheus.MustNewConstMetric(upDesc, prometheus.GaugeValue, up)
	ch <- prometheus.MustNewConstMetric(scrapeDurationDesc, prometheus.GaugeValue, time.Since(start).Seconds())
	ch <- e.totalScrapes
}

func (e *Exporter) runCollector(ctx context.Context, name string, c collector, ch chan<- prometheus.Metric) bool {
	start := time.Now()
	err := c.Collect(ctx, ch)
	duration := time.Since(start).Seconds()

	up := 1.0
	if err != nil {
		up = 0
		if status.Code(err) == codes.Unimplemented {
			err = fmt.Errorf("%w (is the service listed in Xray's api.services, and is Xray new enough?)", err)
		}
		logrus.Warnf("Collector %q failed: %s", name, err)
	}

	ch <- prometheus.MustNewConstMetric(collectorUpDesc, prometheus.GaugeValue, up, name)
	ch <- prometheus.MustNewConstMetric(collectorDurationDesc, prometheus.GaugeValue, duration, name)

	return err == nil
}
