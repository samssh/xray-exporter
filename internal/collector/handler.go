package collector

import (
	"context"
	"fmt"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/samssh/xray-exporter/internal/xrayapi/app/proxyman/command"
)

type handlerCollector struct {
	client       command.HandlerServiceClient
	inboundUsers *prometheus.Desc
	outboundInfo *prometheus.Desc
}

func newHandlerCollector(conn *grpc.ClientConn, _ Options) (collector, error) {
	return &handlerCollector{
		client:       command.NewHandlerServiceClient(conn),
		inboundUsers: newDesc("inbound_users", "Number of users configured on the inbound", "inbound"),
		outboundInfo: newDesc("outbound_info", "Outbound and its protocol, always 1", "outbound", "protocol"),
	}, nil
}

func (c *handlerCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.inboundUsers
	ch <- c.outboundInfo
}

func (c *handlerCollector) Collect(ctx context.Context, ch chan<- prometheus.Metric) error {
	if err := c.collectInbounds(ctx, ch); err != nil {
		return err
	}
	return c.collectOutbounds(ctx, ch)
}

func (c *handlerCollector) collectInbounds(ctx context.Context, ch chan<- prometheus.Metric) error {
	// Only ask for tags: the full config would include every user of every inbound.
	resp, err := c.client.ListInbounds(ctx, &command.ListInboundsRequest{IsOnlyTags: true})
	if err != nil {
		return fmt.Errorf("failed to list inbounds: %w", err)
	}

	for _, in := range resp.GetInbounds() {
		tag := in.GetTag()
		if tag == "" {
			continue
		}

		count, err := c.client.GetInboundUsersCount(ctx, &command.GetInboundUserRequest{Tag: tag})
		if status.Code(err) == codes.Unimplemented {
			return fmt.Errorf("failed to count users of inbound %q: %w", tag, err)
		}
		if err != nil {
			// Inbounds without users, such as dokodemo-door, return an error.
			logrus.Debugf("Skipping user count of inbound %q: %s", tag, err)
			continue
		}

		ch <- prometheus.MustNewConstMetric(c.inboundUsers, prometheus.GaugeValue, float64(count.GetCount()), tag)
	}

	return nil
}

func (c *handlerCollector) collectOutbounds(ctx context.Context, ch chan<- prometheus.Metric) error {
	resp, err := c.client.ListOutbounds(ctx, &command.ListOutboundsRequest{})
	if err != nil {
		return fmt.Errorf("failed to list outbounds: %w", err)
	}

	for _, out := range resp.GetOutbounds() {
		if out.GetTag() == "" {
			continue
		}
		protocol := protocolName(out.GetProxySettings().GetType())
		ch <- prometheus.MustNewConstMetric(c.outboundInfo, prometheus.GaugeValue, 1, out.GetTag(), protocol)
	}

	return nil
}

// protocolName turns a proxy config type, such as
// "xray.proxy.vless.outbound.Config", into a protocol name, such as "vless".
func protocolName(configType string) string {
	rest, ok := strings.CutPrefix(configType, "xray.proxy.")
	if !ok {
		return configType
	}
	name, _, _ := strings.Cut(rest, ".")
	return name
}
