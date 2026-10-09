package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"

	"github.com/samssh/xray-exporter/internal/xrayapi/app/observatory/command"
)

type observatoryCollector struct {
	client       command.ObservatoryServiceClient
	up           *prometheus.Desc
	delay        *prometheus.Desc
	lastSeen     *prometheus.Desc
	lastTry      *prometheus.Desc
	probes       *prometheus.Desc
	failed       *prometheus.Desc
	rttAverage   *prometheus.Desc
	rttMin       *prometheus.Desc
	rttMax       *prometheus.Desc
	rttDeviation *prometheus.Desc
}

func newObservatoryCollector(conn *grpc.ClientConn, _ Options) (collector, error) {
	return &observatoryCollector{
		client: command.NewObservatoryServiceClient(conn),
		up:     newDesc("observatory_outbound_up", "Whether the outbound's last probe succeeded", "outbound"),
		delay: newDesc("observatory_outbound_delay_seconds",
			"Probe delay of the outbound. With burstObservatory, the average RTT. Only set while the outbound is up", "outbound"),
		lastSeen: newDesc("observatory_outbound_last_seen_timestamp_seconds",
			"When a probe of the outbound last succeeded, in Unix time (observatory only)", "outbound"),
		lastTry: newDesc("observatory_outbound_last_try_timestamp_seconds",
			"When the outbound was last probed, in Unix time (observatory only)", "outbound"),
		probes: newDesc("observatory_health_ping_probes",
			"Number of probes in the burstObservatory sampling window", "outbound"),
		failed: newDesc("observatory_health_ping_failed_probes",
			"Number of failed probes in the burstObservatory sampling window", "outbound"),
		rttAverage: newDesc("observatory_health_ping_rtt_average_seconds",
			"Average RTT in the burstObservatory sampling window", "outbound"),
		rttMin: newDesc("observatory_health_ping_rtt_min_seconds",
			"Minimum RTT in the burstObservatory sampling window", "outbound"),
		rttMax: newDesc("observatory_health_ping_rtt_max_seconds",
			"Maximum RTT in the burstObservatory sampling window", "outbound"),
		rttDeviation: newDesc("observatory_health_ping_rtt_deviation_seconds",
			"Standard deviation of the RTT in the burstObservatory sampling window", "outbound"),
	}, nil
}

func (c *observatoryCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{c.up, c.delay, c.lastSeen, c.lastTry, c.probes, c.failed, c.rttAverage, c.rttMin, c.rttMax, c.rttDeviation} {
		ch <- d
	}
}

func (c *observatoryCollector) Collect(ctx context.Context, ch chan<- prometheus.Metric) error {
	resp, err := c.client.GetOutboundStatus(ctx, &command.GetOutboundStatusRequest{})
	if err != nil {
		return fmt.Errorf("failed to get outbound status: %w", err)
	}

	gauge := func(d *prometheus.Desc, v float64, tag string) {
		ch <- prometheus.MustNewConstMetric(d, prometheus.GaugeValue, v, tag)
	}

	for _, s := range resp.GetStatus().GetStatus() {
		tag := s.GetOutboundTag()

		up := 0.0
		if s.GetAlive() {
			up = 1
			// Xray reports a placeholder delay for dead outbounds, so skip it.
			gauge(c.delay, (time.Duration(s.GetDelay()) * time.Millisecond).Seconds(), tag)
		}
		gauge(c.up, up, tag)

		if t := s.GetLastSeenTime(); t > 0 {
			gauge(c.lastSeen, float64(t), tag)
		}
		if t := s.GetLastTryTime(); t > 0 {
			gauge(c.lastTry, float64(t), tag)
		}

		// Only burstObservatory fills in health ping results. RTTs are in nanoseconds.
		if hp := s.GetHealthPing(); hp != nil && hp.GetAll() > 0 {
			gauge(c.probes, float64(hp.GetAll()), tag)
			gauge(c.failed, float64(hp.GetFail()), tag)
			if hp.GetAll() == hp.GetFail() {
				continue // No successful probe, so the RTTs are all zero.
			}
			gauge(c.rttAverage, time.Duration(hp.GetAverage()).Seconds(), tag)
			gauge(c.rttMin, time.Duration(hp.GetMin()).Seconds(), tag)
			gauge(c.rttMax, time.Duration(hp.GetMax()).Seconds(), tag)
			gauge(c.rttDeviation, time.Duration(hp.GetDeviation()).Seconds(), tag)
		}
	}

	return nil
}
