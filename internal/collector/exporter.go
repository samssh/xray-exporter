// Package collector gathers metrics from the Xray API and exposes them to Prometheus.
package collector

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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

// Info describes an available collector.
type Info struct {
	Name             string
	Help             string
	EnabledByDefault bool
	new              func(conn *grpc.ClientConn, opts Options) (collector, error)
}

// collectors lists every available collector, in the order they are documented.
var collectors = []Info{
	{"traffic", "Traffic counters per inbound, outbound and user (StatsService)", true, newTrafficCollector},
	{"runtime", "Xray uptime and Go runtime stats (StatsService)", true, newRuntimeCollector},
}

// All returns every available collector.
func All() []Info {
	return collectors
}

// Options configures an Exporter.
type Options struct {
	// Endpoint is the Xray API address, as HOST:PORT.
	Endpoint string
	// ScrapeTimeout bounds each scrape, including all collectors.
	ScrapeTimeout time.Duration
	// Collectors are the names of the collectors to run.
	Collectors []string
	// DialOptions are added to the gRPC client's options.
	DialOptions []grpc.DialOption
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

// Exporter runs the enabled collectors on every scrape.
type Exporter struct {
	sync.Mutex
	scrapeTimeout time.Duration
	registry      *prometheus.Registry
	totalScrapes  prometheus.Counter
	conn          *grpc.ClientConn
	collectors    map[string]collector
}

// New creates an exporter for the Xray API. The gRPC connection is
// established lazily, so Xray does not need to be running yet.
func New(opts Options) (*Exporter, error) {
	dialOpts := append([]grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}, opts.DialOptions...)
	conn, err := grpc.NewClient(opts.Endpoint, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC client for %q: %w", opts.Endpoint, err)
	}

	e := Exporter{
		scrapeTimeout: opts.ScrapeTimeout,
		registry:      prometheus.NewRegistry(),
		totalScrapes: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "scrapes_total",
			Help:      "Total number of scrapes performed",
		}),
		conn:       conn,
		collectors: map[string]collector{},
	}

	for _, name := range opts.Collectors {
		info, ok := findCollector(name)
		if !ok {
			_ = conn.Close()
			return nil, fmt.Errorf("unknown collector %q", name)
		}
		c, err := info.new(conn, opts)
		if err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("collector %q: %w", name, err)
		}
		e.collectors[name] = c
	}

	e.registry.MustRegister(&e)

	return &e, nil
}

func findCollector(name string) (Info, bool) {
	for _, c := range collectors {
		if c.Name == name {
			return c, true
		}
	}
	return Info{}, false
}

// Handler serves the Xray metrics in the Prometheus format.
func (e *Exporter) Handler() http.Handler {
	return promhttp.HandlerFor(e.registry, promhttp.HandlerOpts{ErrorHandling: promhttp.ContinueOnError})
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
