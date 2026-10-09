package collector

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"google.golang.org/grpc"

	"github.com/samssh/xray-exporter/internal/xrayapi/app/router/command"
)

type fakeRoutingServer struct {
	command.UnimplementedRoutingServiceServer
}

func (fakeRoutingServer) GetBalancerInfo(_ context.Context, req *command.GetBalancerInfoRequest) (*command.GetBalancerInfoResponse, error) {
	switch req.GetTag() {
	case "least-ping":
		return &command.GetBalancerInfoResponse{Balancer: &command.BalancerMsg{
			Override:        &command.OverrideInfo{},
			PrincipleTarget: &command.PrincipleTargetInfo{Tag: []string{"proxy-a"}},
		}}, nil
	case "overridden":
		return &command.GetBalancerInfoResponse{Balancer: &command.BalancerMsg{
			Override: &command.OverrideInfo{Target: "proxy-b"},
		}}, nil
	default:
		return nil, errors.New("cannot find tag")
	}
}

func withRouting(s *grpc.Server) { command.RegisterRoutingServiceServer(s, fakeRoutingServer{}) }

func TestBalancerCollector(t *testing.T) {
	e := newTestExporter(t, withRouting,
		Options{Collectors: []string{"balancer"}, BalancerTags: []string{"least-ping", "overridden"}})

	expected := `
# HELP xray_balancer_override Outbound the balancer is manually overridden to, always 1
# TYPE xray_balancer_override gauge
xray_balancer_override{balancer="overridden",outbound="proxy-b"} 1
# HELP xray_balancer_selected Outbounds the balancer's strategy currently prefers, always 1 (leastPing and leastLoad only)
# TYPE xray_balancer_selected gauge
xray_balancer_selected{balancer="least-ping",outbound="proxy-a"} 1
`
	err := testutil.GatherAndCompare(e.registry, strings.NewReader(expected), "xray_balancer_override", "xray_balancer_selected")
	if err != nil {
		t.Fatal(err)
	}
}

func TestBalancerCollectorUnknownBalancer(t *testing.T) {
	e := newTestExporter(t, withRouting,
		Options{Collectors: []string{"balancer"}, BalancerTags: []string{"missing"}})

	expected := `
# HELP xray_collector_up Whether the collector succeeded
# TYPE xray_collector_up gauge
xray_collector_up{collector="balancer"} 0
`
	if err := testutil.GatherAndCompare(e.registry, strings.NewReader(expected), "xray_collector_up"); err != nil {
		t.Fatal(err)
	}
}

func TestBalancerCollectorNeedsTags(t *testing.T) {
	if _, err := New(Options{Endpoint: "127.0.0.1:1", Collectors: []string{"balancer"}}); err == nil {
		t.Fatal("expected an error without balancer tags")
	}
}
