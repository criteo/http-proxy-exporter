package main

import (
	"github.com/prometheus/client_golang/prometheus"
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

	proxyRequestsDurations = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "proxy_requests_rtt_seconds",
		Help:    "Histogram of requests durations.",
		Buckets: []float64{.0025, .005, .0075, .01, .0125, .015, .0175, .02, .025, .035, .05, .075, .1, .2, .5, 1},
	}, []string{"proxy_url", "resource_url"})
)

func init() {
	prometheus.MustRegister(proxyLookupSuccesses)
	prometheus.MustRegister(proxyLookupFailures)
	prometheus.MustRegister(proxyConnectionTentatives)
	prometheus.MustRegister(proxyConnectionSuccesses)
	prometheus.MustRegister(proxyConnectionErrors)
	prometheus.MustRegister(proxyRequests)
	prometheus.MustRegister(proxyRequestsSuccesses)
	prometheus.MustRegister(proxyRequestsFailures)
	prometheus.MustRegister(proxyRequestDurations)
	prometheus.MustRegister(proxyRequestsDurations)
}
