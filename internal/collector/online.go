package collector

import (
	"context"
	"fmt"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/samssh/xray-exporter/internal/xrayapi/app/stats/command"
)

type onlineCollector struct {
	client     command.StatsServiceClient
	perIP      bool
	users      *prometheus.Desc
	userIPs    *prometheus.Desc
	ipLastSeen *prometheus.Desc
}

func newOnlineCollector(conn *grpc.ClientConn, opts Options) (collector, error) {
	return &onlineCollector{
		client:  command.NewStatsServiceClient(conn),
		perIP:   opts.OnlineIPs,
		users:   newDesc("online_users", "Number of users with at least one online IP"),
		userIPs: newDesc("user_online_ips", "Number of online IPs of the user", "user"),
		ipLastSeen: newDesc("user_online_ip_last_seen_timestamp_seconds",
			"When the user last opened a connection from the IP, in Unix time", "user", "ip"),
	}, nil
}

func (c *onlineCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.users
	ch <- c.userIPs
	if c.perIP {
		ch <- c.ipLastSeen
	}
}

func (c *onlineCollector) Collect(ctx context.Context, ch chan<- prometheus.Metric) error {
	users, err := c.fetch(ctx)
	if err != nil {
		return err
	}

	ch <- prometheus.MustNewConstMetric(c.users, prometheus.GaugeValue, float64(len(users)))
	for user, ips := range users {
		ch <- prometheus.MustNewConstMetric(c.userIPs, prometheus.GaugeValue, float64(len(ips)), user)
		if c.perIP {
			for ip, lastSeen := range ips {
				ch <- prometheus.MustNewConstMetric(c.ipLastSeen, prometheus.GaugeValue, float64(lastSeen), user, ip)
			}
		}
	}

	return nil
}

// fetch returns the online IPs of every online user, keyed by email, with
// each IP's last-seen Unix time.
func (c *onlineCollector) fetch(ctx context.Context) (map[string]map[string]int64, error) {
	// GetUsersStats returns everything in one call, but only exists since Xray v26.4.13.
	resp, err := c.client.GetUsersStats(ctx, &command.GetUsersStatsRequest{})
	if status.Code(err) == codes.Unimplemented {
		return c.fetchPerUser(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get users stats: %w", err)
	}

	users := map[string]map[string]int64{}
	for _, u := range resp.GetUsers() {
		if len(u.GetIps()) == 0 {
			continue
		}
		ips := map[string]int64{}
		for _, ip := range u.GetIps() {
			ips[ip.GetIp()] = ip.GetLastSeen()
		}
		users[u.GetEmail()] = ips
	}

	return users, nil
}

// fetchPerUser is the fallback for Xray v26.1.13 and later releases before v26.4.13, using one call
// to list online users and one call per user for their IPs.
func (c *onlineCollector) fetchPerUser(ctx context.Context) (map[string]map[string]int64, error) {
	resp, err := c.client.GetAllOnlineUsers(ctx, &command.GetAllOnlineUsersRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to get online users: %w", err)
	}

	users := map[string]map[string]int64{}
	for _, name := range resp.GetUsers() {
		// example value: user>>>foo@example.com>>>online
		email, ok := strings.CutPrefix(name, "user>>>")
		email, ok2 := strings.CutSuffix(email, ">>>online")
		if !ok || !ok2 {
			continue
		}

		ipList, err := c.client.GetStatsOnlineIpList(ctx, &command.GetStatsRequest{Name: name})
		if status.Code(err) == codes.NotFound {
			continue // The user went offline in the meantime.
		}
		if err != nil {
			return nil, fmt.Errorf("failed to get online IPs of %q: %w", email, err)
		}
		if len(ipList.GetIps()) > 0 {
			users[email] = ipList.GetIps()
		}
	}

	return users, nil
}
