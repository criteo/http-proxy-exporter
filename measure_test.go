package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"testing"

	"github.com/criteo/http-proxy-exporter/proxyclient"
	"github.com/elazarl/goproxy"
	"github.com/phayes/freeport"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

func requireCounter(t *testing.T, counter *prometheus.CounterVec, labels prometheus.Labels, value float64) {
	c, err := counter.GetMetricWith(labels)
	require.NoError(t, err)

	pb := &dto.Metric{}
	c.Write(pb)
	v := pb.GetCounter().GetValue()
	require.Equal(t, value, v)
}

func runProxy(t *testing.T) (string, func()) {
	proxyLis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	done := func() {
		proxyLis.Close()
	}

	proxy := goproxy.NewProxyHttpServer()
	proxy.Verbose = true
	go http.Serve(proxyLis, proxy)

	return fmt.Sprintf("http://%s", proxyLis.Addr().String()), done
}

func runProxyTLS(t *testing.T) (string, func()) {
	proxyLis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	done := func() {
		proxyLis.Close()
	}

	proxyTLSLis := tls.NewListener(proxyLis, getTLSConfig(t))

	proxy := goproxy.NewProxyHttpServer()
	proxy.Verbose = true

	go http.Serve(proxyTLSLis, proxy)

	return fmt.Sprintf("https://%s", proxyLis.Addr().String()), done
}

func runOrigin(t *testing.T, resCode int) (string, func()) {
	originLis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	done := func() {
		originLis.Close()
	}

	go http.Serve(originLis, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(resCode)
		rw.Write([]byte("Hello from origin"))
	}))

	return fmt.Sprintf("http://%s", originLis.Addr().String()), done
}

func resetMetrics() {
	proxyConnectionTentatives.Reset()
	proxyConnectionSuccesses.Reset()
	proxyConnectionErrors.Reset()
	proxyRequestTotal.Reset()
	proxyRequestsSuccesses.Reset()
	proxyRequestsFailures.Reset()
	proxyRequestsDurations.Reset()
}

func TestProxyOK(t *testing.T) {
	resetMetrics()

	proxyURL, done := runProxy(t)
	defer done()

	originURL, done := runOrigin(t, 200)
	defer done()

	measureOne(proxyURL, Target{URL: originURL, Insecure: true}, &proxyclient.AuthMethod{})

	requireCounter(t,
		proxyConnectionSuccesses,
		prometheus.Labels{"proxy_url": proxyURL},
		1,
	)
	requireCounter(t,
		proxyConnectionErrors,
		prometheus.Labels{"proxy_url": proxyURL, "cause": proxyConnectionErrorCauseProxy},
		0,
	)
	requireCounter(t,
		proxyRequestTotal,
		prometheus.Labels{"proxy_url": proxyURL, "resource_url": originURL},
		1,
	)
	requireCounter(t,
		proxyRequestsSuccesses,
		prometheus.Labels{"proxy_url": proxyURL, "resource_url": originURL, "status_code": "200"},
		1,
	)
	requireCounter(t,
		proxyRequestsFailures,
		prometheus.Labels{"proxy_url": proxyURL, "resource_url": originURL},
		0,
	)
}

func TestProxyOKTLS(t *testing.T) {
	resetMetrics()

	proxyURL, done := runProxyTLS(t)
	defer done()

	originURL, done := runOrigin(t, 200)
	defer done()

	measureOne(proxyURL, Target{URL: originURL, Insecure: true}, &proxyclient.AuthMethod{})

	requireCounter(t,
		proxyConnectionSuccesses,
		prometheus.Labels{"proxy_url": proxyURL},
		1,
	)
	requireCounter(t,
		proxyConnectionErrors,
		prometheus.Labels{"proxy_url": proxyURL, "cause": proxyConnectionErrorCauseProxy},
		0,
	)
	requireCounter(t,
		proxyRequestTotal,
		prometheus.Labels{"proxy_url": proxyURL, "resource_url": originURL},
		1,
	)
	requireCounter(t,
		proxyRequestsSuccesses,
		prometheus.Labels{"proxy_url": proxyURL, "resource_url": originURL, "status_code": "200"},
		1,
	)
	requireCounter(t,
		proxyRequestsFailures,
		prometheus.Labels{"proxy_url": proxyURL, "resource_url": originURL},
		0,
	)
}

func TestProxyOKAuth(t *testing.T) {
	resetMetrics()

	proxyURL, done := runProxy(t)
	defer done()

	originURL, done := runOrigin(t, 200)
	defer done()

	u, _ := url.Parse(proxyURL)
	u.User = url.UserPassword("user", "secret_pass")
	proxyURL = u.String()
	u.User = nil
	proxyURLMetrics := u.String()

	measureOne(proxyURL, Target{URL: originURL, Insecure: true}, &proxyclient.AuthMethod{})

	requireCounter(t,
		proxyConnectionSuccesses,
		prometheus.Labels{"proxy_url": proxyURLMetrics},
		1,
	)
	requireCounter(t,
		proxyConnectionErrors,
		prometheus.Labels{"proxy_url": proxyURLMetrics, "cause": proxyConnectionErrorCauseProxy},
		0,
	)
	requireCounter(t,
		proxyRequestTotal,
		prometheus.Labels{"proxy_url": proxyURLMetrics, "resource_url": originURL},
		1,
	)
	requireCounter(t,
		proxyRequestsSuccesses,
		prometheus.Labels{"proxy_url": proxyURLMetrics, "resource_url": originURL, "status_code": "200"},
		1,
	)
	requireCounter(t,
		proxyRequestsFailures,
		prometheus.Labels{"proxy_url": proxyURLMetrics, "resource_url": originURL},
		0,
	)
}

func TestProxyNoProxy(t *testing.T) {
	resetMetrics()

	originURL, done := runOrigin(t, 200)
	defer done()

	proxyURL := ""

	measureOne(proxyURL, Target{URL: originURL, Insecure: true}, &proxyclient.AuthMethod{})

	requireCounter(t,
		proxyConnectionSuccesses,
		prometheus.Labels{"proxy_url": proxyURL},
		1,
	)
	requireCounter(t,
		proxyConnectionErrors,
		prometheus.Labels{"proxy_url": proxyURL, "cause": proxyConnectionErrorCauseProxy},
		0,
	)
	requireCounter(t,
		proxyRequestTotal,
		prometheus.Labels{"proxy_url": proxyURL, "resource_url": originURL},
		1,
	)
	requireCounter(t,
		proxyRequestsSuccesses,
		prometheus.Labels{"proxy_url": proxyURL, "resource_url": originURL, "status_code": "200"},
		1,
	)
	requireCounter(t,
		proxyRequestsFailures,
		prometheus.Labels{"proxy_url": proxyURL, "resource_url": originURL},
		0,
	)
}

func TestProxyResolveFailure(t *testing.T) {
	resetMetrics()

	originURL, done := runOrigin(t, 200)
	defer done()

	proxyURL := "http://i_do_not_exist.local"

	measureOne(proxyURL, Target{URL: originURL, Insecure: true}, &proxyclient.AuthMethod{})

	requireCounter(t,
		proxyConnectionSuccesses,
		prometheus.Labels{"proxy_url": proxyURL},
		0,
	)
	requireCounter(t,
		proxyConnectionErrors,
		prometheus.Labels{"proxy_url": proxyURL, "cause": proxyConnectionErrorCauseLookup},
		1,
	)
}

func TestProxyBadOrigin500(t *testing.T) {
	resetMetrics()

	proxyURL, done := runProxy(t)
	defer done()

	originURL, done := runOrigin(t, 500)
	defer done()

	measureOne(proxyURL, Target{URL: originURL, Insecure: true}, &proxyclient.AuthMethod{})

	requireCounter(t,
		proxyConnectionSuccesses,
		prometheus.Labels{"proxy_url": proxyURL},
		1,
	)
	requireCounter(t,
		proxyConnectionErrors,
		prometheus.Labels{"proxy_url": proxyURL, "cause": proxyConnectionErrorCauseProxy},
		0,
	)
	requireCounter(t,
		proxyRequestTotal,
		prometheus.Labels{"proxy_url": proxyURL, "resource_url": originURL},
		1,
	)
	requireCounter(t,
		proxyRequestsSuccesses,
		prometheus.Labels{"proxy_url": proxyURL, "resource_url": originURL, "status_code": "500"},
		1,
	)
	requireCounter(t,
		proxyRequestsFailures,
		prometheus.Labels{"proxy_url": proxyURL, "resource_url": originURL},
		0,
	)
}

func TestProxyBadOriginRST(t *testing.T) {
	resetMetrics()

	proxyURL, done := runProxy(t)
	defer done()

	originPort, err := freeport.GetFreePort()
	require.NoError(t, err)

	originURL := fmt.Sprintf("http://127.0.0.1:%d", originPort)

	measureOne(proxyURL, Target{URL: originURL, Insecure: true}, &proxyclient.AuthMethod{})

	requireCounter(t,
		proxyConnectionSuccesses,
		prometheus.Labels{"proxy_url": proxyURL},
		1,
	)
	requireCounter(t,
		proxyConnectionErrors,
		prometheus.Labels{"proxy_url": proxyURL, "cause": proxyConnectionErrorCauseProxy},
		0,
	)
	requireCounter(t,
		proxyRequestTotal,
		prometheus.Labels{"proxy_url": proxyURL, "resource_url": originURL},
		1,
	)
	// this particular proxy returns a 500 on origin RST
	requireCounter(t,
		proxyRequestsSuccesses,
		prometheus.Labels{"proxy_url": proxyURL, "resource_url": originURL, "status_code": "500"},
		1,
	)
	requireCounter(t,
		proxyRequestsFailures,
		prometheus.Labels{"proxy_url": proxyURL, "resource_url": originURL},
		0,
	)
}

func TestProxyBadOriginRSTTLS(t *testing.T) {
	resetMetrics()

	proxyURL, done := runProxyTLS(t)
	defer done()

	originPort, err := freeport.GetFreePort()
	require.NoError(t, err)

	originURL := fmt.Sprintf("http://127.0.0.1:%d", originPort)

	measureOne(proxyURL, Target{URL: originURL, Insecure: true}, &proxyclient.AuthMethod{})

	requireCounter(t,
		proxyConnectionSuccesses,
		prometheus.Labels{"proxy_url": proxyURL},
		1,
	)
	requireCounter(t,
		proxyConnectionErrors,
		prometheus.Labels{"proxy_url": proxyURL, "cause": proxyConnectionErrorCauseProxy},
		0,
	)
	requireCounter(t,
		proxyRequestTotal,
		prometheus.Labels{"proxy_url": proxyURL, "resource_url": originURL},
		1,
	)
	// this particular proxy returns a 500 on origin RST
	requireCounter(t,
		proxyRequestsSuccesses,
		prometheus.Labels{"proxy_url": proxyURL, "resource_url": originURL, "status_code": "500"},
		1,
	)
	requireCounter(t,
		proxyRequestsFailures,
		prometheus.Labels{"proxy_url": proxyURL, "resource_url": originURL},
		0,
	)
}
