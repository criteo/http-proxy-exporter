package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/criteo/http-proxy-exporter/proxyclient"
	"github.com/prometheus/client_golang/prometheus"

	log "github.com/sirupsen/logrus"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	appName      string
	buildVersion string
	buildNumber  string
	buildTime    string

	configFile   string
	printVersion bool

	config Config
)

var (
	proxyLookupSuccesses = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "proxy_lookup_successes_total",
		Help: "Number of successful DNS lookup.",
	}, []string{"proxy_url"})
	proxyLookupFailures = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "proxy_lookup_failure_total",
		Help: "Number of failed DNS lookup.",
	}, []string{"proxy_url"})
	proxyConnectionTentatives = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "proxy_connection_tentatives_total",
		Help: "Total number of tentatives (including proxy connection errors).",
	}, []string{"proxy_url"})
	proxyConnectionSuccesses = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "proxy_connection_successes_total",
		Help: "Number of successful connections towards proxy.",
	}, []string{"proxy_url"})
	proxyConnectionErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "proxy_connection_errors_total",
		Help: "Number of connection errors towards proxy.",
	}, []string{"proxy_url"})
	proxyRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "proxy_requests_total",
		Help: "Total number of requests sent to proxy",
	}, []string{"proxy_url", "resource_url"})
	proxyRequestsSuccesses = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "proxy_requests_successes_total",
		Help: "Number of successful requests.",
	}, []string{"proxy_url", "resource_url", "status_code"})
	proxyRequestsFailures = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "proxy_requests_failure_total",
		Help: "Number of failed requests.",
	}, []string{"proxy_url", "resource_url"})

	proxyRequestDurations = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "proxy_request_rtt_seconds",
		Help: "Gauge of round trip time for each request",
	}, []string{"proxy_url", "resource_url"})

	proxyRequestsDurations prometheus.HistogramVec
)

func init() {
	flag.BoolVar(&printVersion, "version", false, "Print version and exit.")
	flag.BoolVar(&config.Debug, "debug", false, "Enable debug logs.")
	flag.BoolVar(&config.HighPrecision, "high_precision", false, "Enable low latency precision mode.")
	flag.StringVar(&configFile, "config_file", "config.yml", "Path to configuration file.")
	flag.IntVar(&config.Interval, "interval", 10, "Delay between each request.")
	flag.IntVar(&config.ListenPort, "listen_port", 8000, "Prometheus HTTP server port.")

	prometheus.MustRegister(proxyLookupSuccesses)
	prometheus.MustRegister(proxyLookupFailures)
	prometheus.MustRegister(proxyConnectionTentatives)
	prometheus.MustRegister(proxyConnectionSuccesses)
	prometheus.MustRegister(proxyConnectionErrors)
	prometheus.MustRegister(proxyRequests)
	prometheus.MustRegister(proxyRequestsSuccesses)
	prometheus.MustRegister(proxyRequestsFailures)
	prometheus.MustRegister(proxyRequestDurations)
}

func initHistogram() {
	var bucket []float64
	if config.HighPrecision {
		bucket = []float64{.0025, .005, .0075, .01, .0125, .015, .0175, .02, .025, .035, .05, .075, .1, .2, 1}
	}

	proxyRequestsDurations = *prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "proxy_requests_rtt_seconds",
		Help:    "Histogram of requests durations.",
		Buckets: bucket,
	}, []string{"proxy_url", "resource_url"})

	prometheus.MustRegister(proxyRequestsDurations)
}

func main() {
	flag.Parse()

	if printVersion {
		fmt.Println(appName)
		fmt.Println("version:", buildVersion)
		fmt.Println("build:", buildNumber)
		fmt.Println("build time:", buildTime)
		os.Exit(0)
	}

	// load configuration
	config, err := loadConfig(configFile)
	if err != nil {
		log.Fatalf("error while loading config: %s", err)
	}

	initHistogram()

	// verify configuration
	errs := verifyConfig(config)
	if len(errs) > 1 {
		log.Error("configuration validation failed")
		for _, err := range errs {
			log.Error(err)
		}
	}

	if config.Debug {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}

	// FIXME: find a better way to handle multiple auth methods (once they exist)
	auth := &proxyclient.AuthMethod{}
	if len(config.AuthMethods) > 0 {
		auth = config.AuthMethods["basic"]
	}

	// initialize proxy-related counters
	for _, proxy := range config.Proxies {
		proxyLookupSuccesses.WithLabelValues(proxy).Add(0)
		proxyLookupFailures.WithLabelValues(proxy).Add(0)
		proxyConnectionErrors.WithLabelValues(proxy).Add(0)
		proxyConnectionTentatives.WithLabelValues(proxy).Add(0)
		proxyConnectionSuccesses.WithLabelValues(proxy).Add(0)
		for _, target := range config.Targets {
			proxyScheme, err := proxyclient.GetURLScheme(proxy)
			targetScheme, err := proxyclient.GetURLScheme(target.URL)
			if err == nil && proxyScheme == targetScheme {
				proxyRequests.WithLabelValues(proxy, target.URL).Add(0)
			}
		}
	}

	for _, target := range config.Targets {
		for _, proxy := range config.Proxies {
			// create 1 measurement goroutine by (target, proxy) tuple
			go func(target Target, proxy string) {
				var statusCode string

				firstMeasurement := true

				requestConfig := proxyclient.RequestConfig{
					Target:     target.URL,
					Proxy:      proxy,
					Auth:       auth,
					SourceAddr: config.SourceAddress,
					Insecure:   target.Insecure,
				}

				for {
					// sleep at the beginning of the loop as there are continues (avoids code duplication)
					if !firstMeasurement {
						time.Sleep(time.Duration(config.Interval) * time.Second)
					} else {
						firstMeasurement = false
					}

					preq, err := proxyclient.MakeClientAndRequest(requestConfig)
					if err != nil {
						log.Errorf("error while preparing request: %s", err)
					}

					startTime := time.Now()
					resp, err := preq.Client.Do(preq.Request)
					duration := float64(time.Now().Sub(startTime)) / float64(time.Second)

					if err != nil {
						if strings.Contains(err.Error(), "proxyconnect") {
							// proxyconnect regroups errors that indicates the proxy could not be reached
							proxyConnectionTentatives.WithLabelValues(proxy).Inc()
							proxyConnectionErrors.WithLabelValues(proxy).Inc()
							if strings.Contains(err.Error(), "lookup") {
								// catch DNS related errors
								log.Infof("could not resolve %s: %s", proxy, err)
								proxyLookupFailures.WithLabelValues(proxy).Inc()
							} else {
								log.Infof("could not connect to %s: %s", proxy, err)
							}
						} else {
							// the proxy replied but something bad happened
							log.Infof("an error happened trying to reach %s via %s: %s", target.URL, proxy, err)
							proxyConnectionTentatives.WithLabelValues(proxy).Inc()
							proxyConnectionSuccesses.WithLabelValues(proxy).Inc()
							proxyRequests.WithLabelValues(proxy, target.URL).Inc()
							proxyRequestsFailures.WithLabelValues(proxy, target.URL).Inc()
						}
						log.Error(err)
						continue
					}
					resp.Body.Close()
					statusCode = fmt.Sprintf("%d", resp.StatusCode)
					log.Debugf("%v: %v in %vs", target.URL, statusCode, duration)
					proxyLookupSuccesses.WithLabelValues(proxy).Inc()
					proxyConnectionTentatives.WithLabelValues(proxy).Inc()
					proxyConnectionSuccesses.WithLabelValues(proxy).Inc()
					proxyRequests.WithLabelValues(proxy, target.URL).Inc()
					proxyRequestsSuccesses.WithLabelValues(proxy, target.URL, statusCode).Inc()
					proxyRequestDurations.WithLabelValues(proxy, target.URL).Set(duration)
					proxyRequestsDurations.WithLabelValues(proxy, target.URL).Observe(duration)
				}
			}(target, proxy)
		}
	}

	// start HTTP server to expose metrics in a Prometheus-friendly format
	addr := fmt.Sprintf(":%v", config.ListenPort)
	log.Infof("Starting HTTP server on %s", addr)
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(addr, nil))
}
