package collector

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"google.golang.org/grpc"

	"github.com/samssh/xray-exporter/internal/xrayapi/app/proxyman/command"
	"github.com/samssh/xray-exporter/internal/xrayapi/common/serial"
	"github.com/samssh/xray-exporter/internal/xrayapi/core"
)

type fakeHandlerServer struct {
	command.UnimplementedHandlerServiceServer
}

func (fakeHandlerServer) ListInbounds(_ context.Context, req *command.ListInboundsRequest) (*command.ListInboundsResponse, error) {
	if !req.GetIsOnlyTags() {
		return nil, errors.New("the collector should only ask for tags")
	}
	return &command.ListInboundsResponse{Inbounds: []*core.InboundHandlerConfig{
		{Tag: "vless-in"}, {Tag: "dokodemo-in"}, {Tag: ""},
	}}, nil
}

func (fakeHandlerServer) GetInboundUsersCount(_ context.Context, req *command.GetInboundUserRequest) (*command.GetInboundUsersCountResponse, error) {
	if req.GetTag() != "vless-in" {
		return nil, errors.New("proxy is not a UserManager")
	}
	return &command.GetInboundUsersCountResponse{Count: 3}, nil
}

func (fakeHandlerServer) ListOutbounds(context.Context, *command.ListOutboundsRequest) (*command.ListOutboundsResponse, error) {
	return &command.ListOutboundsResponse{Outbounds: []*core.OutboundHandlerConfig{
		{Tag: "direct", ProxySettings: &serial.TypedMessage{Type: "xray.proxy.freedom.Config"}},
		{Tag: "proxy", ProxySettings: &serial.TypedMessage{Type: "xray.proxy.vless.outbound.Config"}},
	}}, nil
}

func TestHandlerCollector(t *testing.T) {
	e := newTestExporter(t, func(s *grpc.Server) { command.RegisterHandlerServiceServer(s, fakeHandlerServer{}) },
		Options{Collectors: []string{"handler"}})

	expected := `
# HELP xray_collector_up Whether the collector succeeded
# TYPE xray_collector_up gauge
xray_collector_up{collector="handler"} 1
# HELP xray_inbound_users Number of users configured on the inbound
# TYPE xray_inbound_users gauge
xray_inbound_users{inbound="vless-in"} 3
# HELP xray_outbound_info Outbound and its protocol, always 1
# TYPE xray_outbound_info gauge
xray_outbound_info{outbound="direct",protocol="freedom"} 1
xray_outbound_info{outbound="proxy",protocol="vless"} 1
`
	err := testutil.GatherAndCompare(e.registry, strings.NewReader(expected),
		"xray_collector_up", "xray_inbound_users", "xray_outbound_info")
	if err != nil {
		t.Fatal(err)
	}
}

func TestProtocolName(t *testing.T) {
	for in, want := range map[string]string{
		"xray.proxy.vless.outbound.Config": "vless",
		"xray.proxy.freedom.Config":        "freedom",
		"xray.app.reverse.Config":          "xray.app.reverse.Config",
		"":                                 "",
	} {
		if got := protocolName(in); got != want {
			t.Errorf("protocolName(%q) = %q, want %q", in, got, want)
		}
	}
}
