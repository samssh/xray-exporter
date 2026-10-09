package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
)

var (
	buildVersion = "dev"
	buildCommit  = "none"
	buildDate    = "unknown"
)

var exporter *Exporter

func scrapeHandler(w http.ResponseWriter, r *http.Request) {
	promhttp.HandlerFor(
		exporter.registry, promhttp.HandlerOpts{ErrorHandling: promhttp.ContinueOnError},
	).ServeHTTP(w, r)
}

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
	for _, c := range collectors {
		def := "false"
		if c.enabledByDefault {
			def = "true"
		}
		collectorFlags[c.name] = app.Flag("collector."+c.name, fmt.Sprintf("Enable the %s collector: %s (default: %s)", c.name, c.help, def)).
			Default(def).Bool()
	}

	kingpin.MustParse(app.Parse(os.Args[1:]))

	level, _ := logrus.ParseLevel(*logLevel)
	logrus.SetLevel(level)

	fmt.Printf("Xray Exporter %v-%v (built %v)\n", buildVersion, buildCommit, buildDate)

	var enabled []string
	for _, c := range collectors {
		if *collectorFlags[c.name] {
			enabled = append(enabled, c.name)
		}
	}
	logrus.Infof("Enabled collectors: %v", enabled)

	var err error
	scrapeTimeout := time.Duration(*scrapeTimeoutInSeconds) * time.Second
	exporter, err = NewExporter(*endpoint, scrapeTimeout, enabled)
	if err != nil {
		logrus.Fatal(err)
	}

	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc(*metricsPath, scrapeHandler)
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
