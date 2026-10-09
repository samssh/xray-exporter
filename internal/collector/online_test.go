package collector

import (
	"context"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/samssh/xray-exporter/internal/xrayapi/app/stats/command"
)

func (s *fakeStatsServer) GetUsersStats(context.Context, *command.GetUsersStatsRequest) (*command.GetUsersStatsResponse, error) {
	if s.legacy {
		return nil, status.Error(codes.Unimplemented, "unknown method GetUsersStats")
	}
	resp := &command.GetUsersStatsResponse{}
	for email, ips := range s.users {
		u := &command.UserStat{Email: email}
		for ip, lastSeen := range ips {
			u.Ips = append(u.Ips, &command.OnlineIPEntry{Ip: ip, LastSeen: lastSeen})
		}
		resp.Users = append(resp.Users, u)
	}
	return resp, nil
}

func (s *fakeStatsServer) GetAllOnlineUsers(context.Context, *command.GetAllOnlineUsersRequest) (*command.GetAllOnlineUsersResponse, error) {
	resp := &command.GetAllOnlineUsersResponse{}
	for email := range s.users {
		resp.Users = append(resp.Users, "user>>>"+email+">>>online")
	}
	// A user who goes offline between the two calls.
	resp.Users = append(resp.Users, "user>>>gone@example.com>>>online")
	return resp, nil
}

func (s *fakeStatsServer) GetStatsOnlineIpList(_ context.Context, req *command.GetStatsRequest) (*command.GetStatsOnlineIpListResponse, error) {
	email := strings.TrimSuffix(strings.TrimPrefix(req.GetName(), "user>>>"), ">>>online")
	ips, ok := s.users[email]
	if !ok {
		return nil, status.Error(codes.NotFound, req.GetName()+" not found.")
	}
	return &command.GetStatsOnlineIpListResponse{Name: req.GetName(), Ips: ips}, nil
}

func TestOnlineCollector(t *testing.T) {
	users := map[string]map[string]int64{
		"foo@example.com": {"1.1.1.1": 1700000000, "2.2.2.2": 1700000100},
		"bar@example.com": {"3.3.3.3": 1700000200},
	}

	expected := `
# HELP xray_online_users Number of users with at least one online IP
# TYPE xray_online_users gauge
xray_online_users 2
# HELP xray_user_online_ip_last_seen_timestamp_seconds When the user last opened a connection from the IP, in Unix time
# TYPE xray_user_online_ip_last_seen_timestamp_seconds gauge
xray_user_online_ip_last_seen_timestamp_seconds{ip="1.1.1.1",user="foo@example.com"} 1.7e+09
xray_user_online_ip_last_seen_timestamp_seconds{ip="2.2.2.2",user="foo@example.com"} 1.7000001e+09
xray_user_online_ip_last_seen_timestamp_seconds{ip="3.3.3.3",user="bar@example.com"} 1.7000002e+09
# HELP xray_user_online_ips Number of online IPs of the user
# TYPE xray_user_online_ips gauge
xray_user_online_ips{user="bar@example.com"} 1
xray_user_online_ips{user="foo@example.com"} 2
`

	for _, legacy := range []bool{false, true} {
		name := "GetUsersStats"
		if legacy {
			name = "fallback"
		}
		t.Run(name, func(t *testing.T) {
			e := newTestExporter(t, withStats(&fakeStatsServer{legacy: legacy, users: users}),
				Options{Collectors: []string{"online"}, OnlineIPs: true})

			err := testutil.GatherAndCompare(e.registry, strings.NewReader(expected),
				"xray_online_users", "xray_user_online_ips", "xray_user_online_ip_last_seen_timestamp_seconds")
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestOnlineCollectorWithoutPerIPSeries(t *testing.T) {
	e := newTestExporter(t, withStats(&fakeStatsServer{users: map[string]map[string]int64{
		"foo@example.com": {"1.1.1.1": 1700000000},
	}}), Options{Collectors: []string{"online"}})

	if n := testutil.CollectAndCount(e, "xray_user_online_ip_last_seen_timestamp_seconds"); n != 0 {
		t.Fatalf("got %d per-IP series, want none", n)
	}
	if n := testutil.CollectAndCount(e, "xray_user_online_ips"); n != 1 {
		t.Fatalf("got %d per-user series, want 1", n)
	}
}
