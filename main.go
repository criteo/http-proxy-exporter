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

	err = initMetrics(config.Proxies)
	if err != nil {
		log.Fatal(err)
	}

	// FIXME: find a better way to handle multiple auth methods (once they exist)
	auth := &proxyclient.AuthMethod{}
	if len(config.AuthMethods) > 0 {
		auth = config.AuthMethods["basic"]
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
	proxyURLForMetrics := ""
	if proxy != "" {
		url, err := url.Parse(proxy)
		if err != nil {
			// do not log the faulty url here in case it contains a password
			log.Fatal("could not parse proxy url")
		}
		url.User = nil
		proxyURLForMetrics = url.String()
	}

	proxyURL, insecure, err := resolveProxy(proxy)

	if err != nil {
		onLookupFailure(proxyURLForMetrics, err)
		return
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

	connectionFailure := false
	originFailure := false
	if err != nil {
		if strings.Contains(err.Error(), "proxyconnect") {
			// general error connecting to the proxy (conn reset, timeout...)
			connectionFailure = true
		} else if strings.Contains(err.Error(), "Proxy Authentication Required") {
			// auth error in CONNECT mode
			connectionFailure = true
		} else {
			// should not be related to the proxy, but to the origin
			originFailure = true
		}
	} else {
		switch resp.StatusCode {
		// auth error in GET mode
		case http.StatusProxyAuthRequired:
			connectionFailure = true
			err = fmt.Errorf("Proxy Authentication Required")

		// this will also catch origin 502 but we prefer false positives to false negatives
		case http.StatusBadGateway:
			connectionFailure = true
			err = fmt.Errorf("Bad gateway")
		}
	}

	if connectionFailure {
		onConnectionFailure(proxyURLForMetrics, target.URL, err)
	} else if originFailure {
		onConnectionSuccessWithOriginFailure(proxyURLForMetrics, target.URL, err)
	} else {
		onConnectionSuccessWithOriginSuccess(
			proxyURLForMetrics,
			target.URL,
			resp.StatusCode,
			time.Since(startTime),
		)
	}
}

func onLookupFailure(proxyURL string, err error) {
	log.Errorf("error while resolving proxy address: %s", err)

	proxyConnectionTentatives.WithLabelValues(proxyURL).Inc()
	proxyConnectionErrors.WithLabelValues(proxyURL, proxyConnectionErrorCauseLookup).Inc()
}

func onConnectionFailure(proxyURL, targetURL string, err error) {
	log.Errorf("req to %q via %q: connect error: %s", targetURL, proxyURL, err)

	proxyConnectionTentatives.WithLabelValues(proxyURL).Inc()
	proxyConnectionErrors.WithLabelValues(proxyURL, proxyConnectionErrorCauseProxy).Inc()
}

func onConnectionSuccessWithOriginFailure(proxyURL, targetURL string, err error) {
	log.Warnf("req to %q via %q: request error: %s", targetURL, proxyURL, err)

	proxyConnectionTentatives.WithLabelValues(proxyURL).Inc()

	proxyConnectionSuccesses.WithLabelValues(proxyURL).Inc()

	proxyRequestTotal.WithLabelValues(proxyURL, targetURL).Inc()
	proxyRequestsFailures.WithLabelValues(proxyURL, targetURL).Inc()
}

func onConnectionSuccessWithOriginSuccess(proxyURL, targetURL string, statusCode int, duration time.Duration) {
	log.Debugf("req to %q via %q: OK (%d)", targetURL, proxyURL, statusCode)

	proxyConnectionTentatives.WithLabelValues(proxyURL).Inc()
	proxyConnectionSuccesses.WithLabelValues(proxyURL).Inc()

	proxyRequestTotal.WithLabelValues(proxyURL, targetURL).Inc()
	proxyRequestsSuccesses.WithLabelValues(proxyURL, targetURL, fmt.Sprint(statusCode)).Inc()

	proxyRequestsDurations.WithLabelValues(proxyURL, targetURL).Observe(duration.Seconds())
}

func resolveProxy(proxy string) (*url.URL, bool, error) {
	if proxy == "" {
		return &url.URL{}, false, nil
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
		return proxyURL, false, err
	}

	outHost := addrs[0]
	if port != "" {
		outHost = net.JoinHostPort(addrs[0], port)
	}

	proxyURL.Host = outHost

	return proxyURL, proxyURL.Scheme == "https", nil
}
