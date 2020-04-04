package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/criteo/http-proxy-exporter/proxyclient"

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

func init() {
	flag.BoolVar(&printVersion, "version", false, "Print version and exit.")
	flag.BoolVar(&config.Debug, "debug", false, "Enable debug logs.")
	flag.BoolVar(&config.HighPrecision, "high_precision", false, "Enable low latency precision mode.")
	flag.StringVar(&configFile, "config_file", "config.yml", "Path to configuration file.")
	flag.IntVar(&config.Interval, "interval", 10, "Delay between each request.")
	flag.IntVar(&config.ListenPort, "listen_port", 8000, "Prometheus HTTP server port.")
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
				for range time.Tick(time.Duration(config.Interval) * time.Second) {
					measureOne(proxy, target, auth)
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

func measureOne(proxy string, target Target, auth *proxyclient.AuthMethod) {
	requestConfig := proxyclient.RequestConfig{
		Target:     target.URL,
		Proxy:      proxy,
		Auth:       auth,
		SourceAddr: config.SourceAddress,
		Insecure:   target.Insecure,
		Timeout:    time.Duration(config.Interval) * time.Second,
	}

	preq, err := proxyclient.MakeClientAndRequest(requestConfig)
	if err != nil {
		log.Errorf("error while preparing request: %s", err)
	}

	startTime := time.Now()
	resp, err := preq.Client.Do(preq.Request)
	duration := time.Now().Sub(startTime).Seconds()

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
		return
	}

	resp.Body.Close()
	statusCode := fmt.Sprintf("%d", resp.StatusCode)
	log.Debugf("%v: %v in %vs", target.URL, statusCode, duration)
	proxyLookupSuccesses.WithLabelValues(proxy).Inc()
	proxyConnectionTentatives.WithLabelValues(proxy).Inc()
	proxyConnectionSuccesses.WithLabelValues(proxy).Inc()
	proxyRequests.WithLabelValues(proxy, target.URL).Inc()
	proxyRequestsSuccesses.WithLabelValues(proxy, target.URL, statusCode).Inc()
	proxyRequestDurations.WithLabelValues(proxy, target.URL).Set(duration)

	if config.HighPrecision {
		proxyRequestsDurations.WithLabelValues(proxy, target.URL).Observe(duration)
	}
}
