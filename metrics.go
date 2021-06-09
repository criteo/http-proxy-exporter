package main

import (
	"net/url"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	proxyConnectionErrorCauseLookup = "lookup"
	proxyConnectionErrorCauseProxy  = "proxy"
)

var (
	proxyConnectionTentatives = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "proxy_connection_tentatives_total",
		Help: "Total number of tentatives (including proxy connection errors).",
	}, []string{"proxy_url", "resource_url"})
	proxyConnectionSuccesses = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "proxy_connection_successes_total",
		Help: "Number of successful connections towards proxy.",
	}, []string{"proxy_url", "resource_url"})
	proxyConnectionErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "proxy_connection_errors_total",
		Help: "Number of connection errors towards proxy.",
	}, []string{"proxy_url", "cause", "resource_url"})
	proxyRequestTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
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

	proxyRequestsDurations = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "proxy_requests_rtt_seconds",
		Help:    "Histogram of requests durations.",
		Buckets: []float64{.0025, .005, .0075, .01, .0125, .015, .0175, .02, .025, .035, .05, .075, .1, .2, .5, 1},
	}, []string{"proxy_url", "resource_url"})
)

func init() {
	prometheus.MustRegister(proxyConnectionTentatives)
	prometheus.MustRegister(proxyConnectionSuccesses)
	prometheus.MustRegister(proxyConnectionErrors)
	prometheus.MustRegister(proxyRequestTotal)
	prometheus.MustRegister(proxyRequestsSuccesses)
	prometheus.MustRegister(proxyRequestsFailures)
	prometheus.MustRegister(proxyRequestsDurations)
}

func initMetrics(proxyURLs []string) error {
	for _, p := range proxyURLs {
		url, err := url.Parse(p)
		if err != nil {
			return err
		}

		url.User = nil
		proxyURL := url.String()

		proxyConnectionTentatives.WithLabelValues(proxyURL).Add(0)
		proxyConnectionSuccesses.WithLabelValues(proxyURL).Add(0)

		proxyConnectionErrors.WithLabelValues(proxyURL, proxyConnectionErrorCauseLookup).Add(0)
		proxyConnectionErrors.WithLabelValues(proxyURL, proxyConnectionErrorCauseProxy).Add(0)
	}

	return nil
}
