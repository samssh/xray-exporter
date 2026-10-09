package collector

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"google.golang.org/grpc"

	"github.com/samssh/xray-exporter/internal/xrayapi/app/observatory"
	"github.com/samssh/xray-exporter/internal/xrayapi/app/observatory/command"
)

type fakeObservatoryServer struct {
	command.UnimplementedObservatoryServiceServer
}

func (fakeObservatoryServer) GetOutboundStatus(context.Context, *command.GetOutboundStatusRequest) (*command.GetOutboundStatusResponse, error) {
	return &command.GetOutboundStatusResponse{Status: &observatory.ObservationResult{Status: []*observatory.OutboundStatus{
		// observatory: a live and a dead outbound.
		{OutboundTag: "alive", Alive: true, Delay: 150, LastSeenTime: 1700000000, LastTryTime: 1700000000},
		{OutboundTag: "dead", Alive: false, Delay: 99999999, LastSeenTime: 1690000000, LastTryTime: 1700000000},
		// burstObservatory: health ping results, RTTs in nanoseconds.
		{OutboundTag: "burst", Alive: true, Delay: 200, HealthPing: &observatory.HealthPingMeasurementResult{
			All: 10, Fail: 1,
			Average:   int64(200 * time.Millisecond),
			Min:       int64(100 * time.Millisecond),
			Max:       int64(400 * time.Millisecond),
			Deviation: int64(50 * time.Millisecond),
		}},
		// burstObservatory with every probe failed: RTTs are zero and must be skipped.
		{OutboundTag: "burst-dead", Alive: false, HealthPing: &observatory.HealthPingMeasurementResult{All: 3, Fail: 3}},
	}}}, nil
}

func TestObservatoryCollector(t *testing.T) {
	e := newTestExporter(t, func(s *grpc.Server) { command.RegisterObservatoryServiceServer(s, fakeObservatoryServer{}) },
		Options{Collectors: []string{"observatory"}})

	expected := `
# HELP xray_observatory_outbound_up Whether the outbound's last probe succeeded
# TYPE xray_observatory_outbound_up gauge
xray_observatory_outbound_up{outbound="alive"} 1
xray_observatory_outbound_up{outbound="burst"} 1
xray_observatory_outbound_up{outbound="burst-dead"} 0
xray_observatory_outbound_up{outbound="dead"} 0
# HELP xray_observatory_outbound_delay_seconds Probe delay of the outbound. With burstObservatory, the average RTT. Only set while the outbound is up
# TYPE xray_observatory_outbound_delay_seconds gauge
xray_observatory_outbound_delay_seconds{outbound="alive"} 0.15
xray_observatory_outbound_delay_seconds{outbound="burst"} 0.2
# HELP xray_observatory_outbound_last_seen_timestamp_seconds When a probe of the outbound last succeeded, in Unix time (observatory only)
# TYPE xray_observatory_outbound_last_seen_timestamp_seconds gauge
xray_observatory_outbound_last_seen_timestamp_seconds{outbound="alive"} 1.7e+09
xray_observatory_outbound_last_seen_timestamp_seconds{outbound="dead"} 1.69e+09
# HELP xray_observatory_health_ping_failed_probes Number of failed probes in the burstObservatory sampling window
# TYPE xray_observatory_health_ping_failed_probes gauge
xray_observatory_health_ping_failed_probes{outbound="burst"} 1
xray_observatory_health_ping_failed_probes{outbound="burst-dead"} 3
# HELP xray_observatory_health_ping_rtt_max_seconds Maximum RTT in the burstObservatory sampling window
# TYPE xray_observatory_health_ping_rtt_max_seconds gauge
xray_observatory_health_ping_rtt_max_seconds{outbound="burst"} 0.4
`
	err := testutil.GatherAndCompare(e.registry, strings.NewReader(expected),
		"xray_observatory_outbound_up", "xray_observatory_outbound_delay_seconds",
		"xray_observatory_outbound_last_seen_timestamp_seconds",
		"xray_observatory_health_ping_failed_probes", "xray_observatory_health_ping_rtt_max_seconds")
	if err != nil {
		t.Fatal(err)
	}
}
