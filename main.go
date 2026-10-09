package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"

	"github.com/samssh/xray-exporter/internal/collector"
)

var (
	buildVersion = "dev"
	buildCommit  = "none"
	buildDate    = "unknown"
)

func main() {
	app := kingpin.New("xray-exporter", "A Prometheus exporter for Xray-core metrics.")
	app.Version(fmt.Sprintf("Xray Exporter %v-%v (built %v)", buildVersion, buildCommit, buildDate))
	app.HelpFlag.Short('h')

	listen := app.Flag("listen", "Listen address").Short('l').Envar("XRAY_EXPORTER_LISTEN").
		Default(":9550").String()
	metricsPath := app.Flag("metrics-path", "Path that serves Xray metrics").Short('m').Envar("XRAY_EXPORTER_METRICS_PATH").
		Default("/scrape").String()
	endpoint := app.Flag("xray-endpoint", "Xray API endpoint").Short('e').Envar("XRAY_EXPORTER_XRAY_ENDPOINT").
		Default("127.0.0.1:8080").String()
	scrapeTimeoutInSeconds := app.Flag("scrape-timeout", "The timeout in seconds for every individual scrape").Short('t').
		Envar("XRAY_EXPORTER_SCRAPE_TIMEOUT").Default("3").Int64()
	logLevel := app.Flag("log.level", "Log level: debug, info, warn or error").Envar("XRAY_EXPORTER_LOG_LEVEL").
		Default("info").Enum("debug", "info", "warn", "error")

	collectorFlags := map[string]*bool{}
	for _, c := range collector.All() {
		def := "false"
		if c.EnabledByDefault {
			def = "true"
		}
		collectorFlags[c.Name] = app.Flag("collector."+c.Name, fmt.Sprintf("Enable the %s collector: %s (default: %s)", c.Name, c.Help, def)).
			Envar("XRAY_EXPORTER_COLLECTOR_" + strings.ToUpper(c.Name)).Default(def).Bool()
	}
	onlineIPs := app.Flag("collector.online.ips", "Export one series per online user and IP (high cardinality)").
		Envar("XRAY_EXPORTER_COLLECTOR_ONLINE_IPS").Bool()
	balancerTags := app.Flag("collector.balancer.tag", "Tags of balancers to report on, comma-separated or repeated").
		Envar("XRAY_EXPORTER_COLLECTOR_BALANCER_TAG").PlaceHolder("TAG").Strings()

	kingpin.MustParse(app.Parse(os.Args[1:]))

	level, _ := logrus.ParseLevel(*logLevel)
	logrus.SetLevel(level)

	fmt.Printf("Xray Exporter %v-%v (built %v)\n", buildVersion, buildCommit, buildDate)

	var enabled []string
	for _, c := range collector.All() {
		if *collectorFlags[c.Name] {
			enabled = append(enabled, c.Name)
		}
	}
	logrus.Infof("Enabled collectors: %v", enabled)

	exporter, err := collector.New(collector.Options{
		Endpoint:      *endpoint,
		ScrapeTimeout: time.Duration(*scrapeTimeoutInSeconds) * time.Second,
		Collectors:    enabled,
		OnlineIPs:     *onlineIPs,
		BalancerTags:  splitCommas(*balancerTags),
	})
	if err != nil {
		logrus.Fatal(err)
	}

	http.Handle("/metrics", promhttp.Handler())
	http.Handle(*metricsPath, exporter.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte(`<html>
<head><title>Xray Exporter</title></head>
<body>
<h1>Xray Exporter ` + buildVersion + `</h1>
<p><a href='/metrics'>Exporter Metrics</a></p>
<p><a href='` + *metricsPath + `'>Scrape Xray Metrics</a></p>
</body>
</html>
`))
		if err != nil {
			logrus.Debugf("Write() err: %s", err)
		}
	})

	logrus.Infof("Server is ready to handle incoming scrape requests.")
	if err := http.ListenAndServe(*listen, nil); err != nil {
		logrus.Error(err)
		_ = exporter.Close()
		os.Exit(1)
	}
}

// splitCommas splits each value on commas and drops empty parts.
func splitCommas(values []string) []string {
	var out []string
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}
