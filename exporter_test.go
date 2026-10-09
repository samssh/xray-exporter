package main

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/v2fly/v2ray-core/v4/app/stats/command"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
)

type fakeStatsServer struct {
	command.UnimplementedStatsServiceServer
	stats []*command.Stat
}

func (s *fakeStatsServer) QueryStats(context.Context, *command.QueryStatsRequest) (*command.QueryStatsResponse, error) {
	return &command.QueryStatsResponse{Stat: s.stats}, nil
}

func (s *fakeStatsServer) GetSysStats(context.Context, *command.SysStatsRequest) (*command.SysStatsResponse, error) {
	return &command.SysStatsResponse{Uptime: 42, NumGoroutine: 7}, nil
}

// newTestExporter starts an in-process gRPC server backed by srv and returns
// an exporter connected to it. A nil srv simulates Xray being unreachable.
func newTestExporter(t *testing.T, srv command.StatsServiceServer) *Exporter {
	t.Helper()

	lis := bufconn.Listen(1 << 20)
	if srv != nil {
		s := grpc.NewServer()
		command.RegisterStatsServiceServer(s, srv)
		go func() { _ = s.Serve(lis) }()
		t.Cleanup(s.Stop)
	} else {
		_ = lis.Close()
	}

	e, err := NewExporter("passthrough:///bufnet", time.Second,
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close() })

	return e
}

func TestCollect(t *testing.T) {
	e := newTestExporter(t, &fakeStatsServer{stats: []*command.Stat{
		{Name: "inbound>>>vless-in>>>traffic>>>uplink", Value: 100},
		{Name: "user>>>foo@example.com>>>traffic>>>downlink", Value: 200},
		// Names that don't have four parts must be skipped, not panic.
		{Name: "user>>>foo@example.com>>>online", Value: 1},
	}})

	expected := `
# HELP xray_traffic_downlink_bytes_total Number of received bytes
# TYPE xray_traffic_downlink_bytes_total counter
xray_traffic_downlink_bytes_total{dimension="user",target="foo@example.com"} 200
# HELP xray_traffic_uplink_bytes_total Number of transmitted bytes
# TYPE xray_traffic_uplink_bytes_total counter
xray_traffic_uplink_bytes_total{dimension="inbound",target="vless-in"} 100
# HELP xray_up Indicate scrape succeeded or not
# TYPE xray_up gauge
xray_up 1
# HELP xray_uptime_seconds Xray uptime in seconds
# TYPE xray_uptime_seconds gauge
xray_uptime_seconds 42
`
	err := testutil.GatherAndCompare(e.registry, strings.NewReader(expected),
		"xray_traffic_downlink_bytes_total", "xray_traffic_uplink_bytes_total", "xray_up", "xray_uptime_seconds")
	if err != nil {
		t.Fatal(err)
	}
}

func TestCollectWhenXrayIsDown(t *testing.T) {
	e := newTestExporter(t, nil)

	expected := `
# HELP xray_up Indicate scrape succeeded or not
# TYPE xray_up gauge
xray_up 0
`
	if err := testutil.GatherAndCompare(e.registry, strings.NewReader(expected), "xray_up"); err != nil {
		t.Fatal(err)
	}
}
