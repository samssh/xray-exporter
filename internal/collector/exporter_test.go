package collector

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"

	observatory "github.com/samssh/xray-exporter/internal/xrayapi/app/observatory/command"
	proxyman "github.com/samssh/xray-exporter/internal/xrayapi/app/proxyman/command"
	"github.com/samssh/xray-exporter/internal/xrayapi/app/stats/command"
)

type fakeStatsServer struct {
	command.UnimplementedStatsServiceServer
	stats []*command.Stat
	// users are the online users, as email -> IP -> last seen. They are served
	// through GetUsersStats, or through the older per-user calls when legacy is set.
	users  map[string]map[string]int64
	legacy bool
}

func (s *fakeStatsServer) QueryStats(context.Context, *command.QueryStatsRequest) (*command.QueryStatsResponse, error) {
	return &command.QueryStatsResponse{Stat: s.stats}, nil
}

func (s *fakeStatsServer) GetSysStats(context.Context, *command.SysStatsRequest) (*command.SysStatsResponse, error) {
	return &command.SysStatsResponse{Uptime: 42, NumGoroutine: 7}, nil
}

// newTestExporter starts an in-process gRPC server and returns an exporter
// connected to it with opts. register adds services to the server; a nil
// register simulates Xray being unreachable.
func newTestExporter(t *testing.T, register func(*grpc.Server), opts Options) *Exporter {
	t.Helper()

	lis := bufconn.Listen(1 << 20)
	if register != nil {
		s := grpc.NewServer()
		register(s)
		go func() { _ = s.Serve(lis) }()
		t.Cleanup(s.Stop)
	} else {
		_ = lis.Close()
	}

	opts.Endpoint = "passthrough:///bufnet"
	opts.ScrapeTimeout = time.Second
	opts.DialOptions = []grpc.DialOption{
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
	}
	e, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close() })

	return e
}

func withStats(srv command.StatsServiceServer) func(*grpc.Server) {
	return func(s *grpc.Server) { command.RegisterStatsServiceServer(s, srv) }
}

func TestCollect(t *testing.T) {
	e := newTestExporter(t, withStats(&fakeStatsServer{stats: []*command.Stat{
		{Name: "inbound>>>vless-in>>>traffic>>>uplink", Value: 100},
		{Name: "user>>>foo@example.com>>>traffic>>>downlink", Value: 200},
		// Names that aren't traffic counters must be skipped, not panic.
		{Name: "user>>>foo@example.com>>>online", Value: 1},
		{Name: "inbound>>>vless-in>>>something>>>else", Value: 1},
	}}), Options{Collectors: []string{"traffic", "runtime"}})

	expected := `
# HELP xray_collector_up Whether the collector succeeded
# TYPE xray_collector_up gauge
xray_collector_up{collector="runtime"} 1
xray_collector_up{collector="traffic"} 1
# HELP xray_traffic_downlink_bytes_total Number of received bytes
# TYPE xray_traffic_downlink_bytes_total counter
xray_traffic_downlink_bytes_total{dimension="user",target="foo@example.com"} 200
# HELP xray_traffic_uplink_bytes_total Number of transmitted bytes
# TYPE xray_traffic_uplink_bytes_total counter
xray_traffic_uplink_bytes_total{dimension="inbound",target="vless-in"} 100
# HELP xray_up Whether all enabled collectors succeeded
# TYPE xray_up gauge
xray_up 1
# HELP xray_uptime_seconds Xray uptime in seconds
# TYPE xray_uptime_seconds gauge
xray_uptime_seconds 42
`
	err := testutil.GatherAndCompare(e.registry, strings.NewReader(expected),
		"xray_collector_up", "xray_traffic_downlink_bytes_total", "xray_traffic_uplink_bytes_total", "xray_up", "xray_uptime_seconds")
	if err != nil {
		t.Fatal(err)
	}
}

func TestCollectOnlyEnabledCollectors(t *testing.T) {
	e := newTestExporter(t, withStats(&fakeStatsServer{}), Options{Collectors: []string{"runtime"}})

	expected := `
# HELP xray_collector_up Whether the collector succeeded
# TYPE xray_collector_up gauge
xray_collector_up{collector="runtime"} 1
`
	if err := testutil.GatherAndCompare(e.registry, strings.NewReader(expected), "xray_collector_up"); err != nil {
		t.Fatal(err)
	}
}

func TestCollectWhenServiceIsMissing(t *testing.T) {
	// The server is reachable but doesn't serve StatsService, like an Xray
	// whose api.services doesn't list it.
	e := newTestExporter(t, func(*grpc.Server) {}, Options{Collectors: []string{"traffic"}})

	expected := `
# HELP xray_collector_up Whether the collector succeeded
# TYPE xray_collector_up gauge
xray_collector_up{collector="traffic"} 0
# HELP xray_up Whether all enabled collectors succeeded
# TYPE xray_up gauge
xray_up 0
`
	if err := testutil.GatherAndCompare(e.registry, strings.NewReader(expected), "xray_collector_up", "xray_up"); err != nil {
		t.Fatal(err)
	}
}

func TestCollectWhenXrayIsDown(t *testing.T) {
	e := newTestExporter(t, nil, Options{Collectors: []string{"traffic", "runtime"}})

	expected := `
# HELP xray_up Whether all enabled collectors succeeded
# TYPE xray_up gauge
xray_up 0
`
	if err := testutil.GatherAndCompare(e.registry, strings.NewReader(expected), "xray_up"); err != nil {
		t.Fatal(err)
	}
}

func TestUnknownCollector(t *testing.T) {
	if _, err := New(Options{Endpoint: "127.0.0.1:1", Collectors: []string{"nope"}}); err == nil {
		t.Fatal("expected an error for an unknown collector")
	}
}

// Every collector must describe all the metrics it sends, or the registry
// rejects them at scrape time.
func TestAllCollectorsDescribeTheirMetrics(t *testing.T) {
	var all []string
	for _, c := range collectors {
		all = append(all, c.Name)
	}
	e := newTestExporter(t, func(s *grpc.Server) {
		withStats(&fakeStatsServer{
			stats: []*command.Stat{{Name: "outbound>>>direct>>>traffic>>>uplink", Value: 1}},
			users: map[string]map[string]int64{"foo@example.com": {"1.1.1.1": 1700000000}},
		})(s)
		withRouting(s)
		proxyman.RegisterHandlerServiceServer(s, fakeHandlerServer{})
		observatory.RegisterObservatoryServiceServer(s, fakeObservatoryServer{})
	}, Options{Collectors: all, OnlineIPs: true, BalancerTags: []string{"least-ping", "overridden"}})

	expected := `
# HELP xray_up Whether all enabled collectors succeeded
# TYPE xray_up gauge
xray_up 1
`
	// GatherAndCompare fails on any metric that wasn't described.
	if err := testutil.GatherAndCompare(e.registry, strings.NewReader(expected), "xray_up"); err != nil {
		t.Fatal(err)
	}
}
