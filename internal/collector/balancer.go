package collector

import (
	"context"
	"errors"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"

	"github.com/samssh/xray-exporter/internal/xrayapi/app/router/command"
)

type balancerCollector struct {
	client   command.RoutingServiceClient
	tags     []string
	selected *prometheus.Desc
	override *prometheus.Desc
}

func newBalancerCollector(conn *grpc.ClientConn, opts Options) (collector, error) {
	// Xray's API can't list balancers, so their tags must be configured.
	if len(opts.BalancerTags) == 0 {
		return nil, errors.New("at least one balancer tag is required")
	}

	return &balancerCollector{
		client: command.NewRoutingServiceClient(conn),
		tags:   opts.BalancerTags,
		selected: newDesc("balancer_selected",
			"Outbounds the balancer's strategy currently prefers, always 1 (leastPing and leastLoad only)", "balancer", "outbound"),
		override: newDesc("balancer_override",
			"Outbound the balancer is manually overridden to, always 1", "balancer", "outbound"),
	}, nil
}

func (c *balancerCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.selected
	ch <- c.override
}

func (c *balancerCollector) Collect(ctx context.Context, ch chan<- prometheus.Metric) error {
	for _, tag := range c.tags {
		resp, err := c.client.GetBalancerInfo(ctx, &command.GetBalancerInfoRequest{Tag: tag})
		if err != nil {
			return fmt.Errorf("failed to get info of balancer %q: %w", tag, err)
		}

		b := resp.GetBalancer()
		for _, out := range b.GetPrincipleTarget().GetTag() {
			ch <- prometheus.MustNewConstMetric(c.selected, prometheus.GaugeValue, 1, tag, out)
		}
		if out := b.GetOverride().GetTarget(); out != "" {
			ch <- prometheus.MustNewConstMetric(c.override, prometheus.GaugeValue, 1, tag, out)
		}
	}

	return nil
}
