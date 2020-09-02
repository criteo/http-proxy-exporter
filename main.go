package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
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
	proxyURL, insecure, err := resolveProxy(proxy)

	proxyConnectionTentatives.WithLabelValues(proxyURL.Redacted()).Inc()

	if err != nil {
		log.Errorf("error while resolving proxy address: %s", err)
		proxyLookupFailures.WithLabelValues(proxyURL.Redacted()).Inc()
		return
	} else {
		proxyLookupSuccesses.WithLabelValues(proxyURL.Redacted()).Inc()
	}

	requestConfig := proxyclient.RequestConfig{
		Target:     target.URL,
		Proxy:      proxyURL.String(),
		Auth:       auth,
		SourceAddr: config.SourceAddress,
		Insecure:   target.Insecure || insecure,
		Timeout:    time.Duration(config.Interval) * time.Second,
	}

	preq, err := proxyclient.MakeClientAndRequest(requestConfig)
	if err != nil {
		log.Errorf("error while preparing request: %s", err)
	}

	startTime := time.Now()
	resp, err := preq.Client.Do(preq.Request)
	if err == nil {
		defer resp.Body.Close()
	}
	duration := time.Now().Sub(startTime).Seconds()

	connectError := false
	requestError := false
	if err != nil {
		if strings.Contains(err.Error(), "proxyconnect") {
			// general error connecting to the proxy (conn reset, timeout...)
			connectError = true
		} else if strings.Contains(err.Error(), "Proxy Authentication Required") {
			// auth error in CONNECT mode
			connectError = true
		} else {
			// should not be related to the proxy, but to the origin
			requestError = true
		}
	} else {
		if resp.StatusCode == http.StatusProxyAuthRequired {
			// auth error in GET mode
			connectError = true
			err = fmt.Errorf("Proxy Authentication Required")
		}
	}

	if connectError {
		log.Errorf("req to %q via %q: connect error: %s", target.URL, proxyURL.Redacted(), err)
		proxyConnectionErrors.WithLabelValues(proxyURL.Redacted()).Inc()
	} else if requestError {
		log.Warnf("req to %q via %q: request error: %s", target.URL, proxyURL.Redacted(), err)
		proxyConnectionSuccesses.WithLabelValues(proxyURL.Redacted()).Inc()
		proxyRequests.WithLabelValues(proxyURL.Redacted(), target.URL).Inc()
		proxyRequestsFailures.WithLabelValues(proxyURL.Redacted(), target.URL).Inc()
	} else {
		log.Debugf("req to %q via %q: OK (%d)", target.URL, proxyURL.Redacted(), resp.StatusCode)
		proxyConnectionSuccesses.WithLabelValues(proxyURL.Redacted()).Inc()
		proxyRequests.WithLabelValues(proxyURL.Redacted(), target.URL).Inc()
		proxyRequestsSuccesses.WithLabelValues(proxyURL.Redacted(), target.URL, fmt.Sprint(resp.StatusCode)).Inc()
		proxyRequestDurations.WithLabelValues(proxyURL.Redacted(), target.URL).Set(duration)

		if config.HighPrecision {
			proxyRequestsDurations.WithLabelValues(proxyURL.Redacted(), target.URL).Observe(duration)
		}
	}
}

func resolveProxy(proxy string) (*url.URL, bool, error) {
	if proxy == "" {
		return nil, false, nil
	}

	// parse the url to extract host
	proxyURL, err := url.Parse(proxy)
	if err != nil {
		panic(fmt.Sprintf("bad proxy url given %q: %s", proxy, err))
	}
	hostPort := proxyURL.Host

	// parse ip:port if there is a port
	host, port, err := net.SplitHostPort(hostPort)
	if err == nil {
		hostPort = host
	}

	// if the host is an IP, do not attempt to resolve
	ip := net.ParseIP(hostPort)
	if ip != nil {
		return proxyURL, false, nil
	}

	addrs, err := net.LookupHost(proxyURL.Host)
	if err != nil {
		return proxyURL, false, nil
	}

	outHost := addrs[0]
	if port != "" {
		outHost = net.JoinHostPort(addrs[0], port)
	}

	proxyURL.Host = outHost

	return proxyURL, proxyURL.Scheme == "https", nil
}
