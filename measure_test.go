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

type srvFunc func(*testing.T, int) (string, func())

func requireCounter(t *testing.T, counter *prometheus.CounterVec, labels prometheus.Labels, value float64) {
	c, err := counter.GetMetricWith(labels)
	require.NoError(t, err)

	pb := &dto.Metric{}
	c.Write(pb)
	v := pb.GetCounter().GetValue()
	require.Equal(t, value, v)
}

func runProxy(t *testing.T, code int) (string, func()) {
	proxyLis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	done := func() {
		proxyLis.Close()
	}

	proxy := goproxy.NewProxyHttpServer()
	proxy.Verbose = true

	if code != 200 {
		proxy.OnRequest().DoFunc(
			func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
				return r, goproxy.NewResponse(r, goproxy.ContentTypeText, code, "")
			})
	}

	go http.Serve(proxyLis, proxy)

	return fmt.Sprintf("http://%s", proxyLis.Addr().String()), done
}

func runProxyTLS(t *testing.T, code int) (string, func()) {
	proxyLis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	done := func() {
		proxyLis.Close()
	}

	proxyTLSLis := tls.NewListener(proxyLis, getTLSConfig(t))

	proxy := goproxy.NewProxyHttpServer()
	proxy.Verbose = true

	if code != 200 {
		proxy.OnRequest().DoFunc(
			func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
				return r, goproxy.NewResponse(r, goproxy.ContentTypeText, code, "")
			})
	}

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

func runOriginTLS(t *testing.T, resCode int) (string, func()) {
	originLis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	done := func() {
		originLis.Close()
	}

	originTLSLis := tls.NewListener(originLis, getTLSConfig(t))

	go http.Serve(originTLSLis, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(resCode)
		rw.Write([]byte("Hello from origin"))
	}))

	return fmt.Sprintf("https://%s", originTLSLis.Addr().String()), done
}

var testMatrix = []struct {
	proxyTLS  bool
	originTLS bool
}{
	{
		proxyTLS:  false,
		originTLS: false,
	},
	{
		proxyTLS:  true,
		originTLS: false,
	},
	{
		proxyTLS:  false,
		originTLS: true,
	},
	{
		proxyTLS:  true,
		originTLS: true,
	},
}

func testDoMatrix(t *testing.T, do func(
	t *testing.T,
	proxyFunc,
	originFunc srvFunc)) {
	for _, tc := range testMatrix {
		name := fmt.Sprintf("proxy_tls=%v origin_tls=%v", tc.proxyTLS, tc.originTLS)
		t.Run(name, func(t *testing.T) {
			resetMetrics()

			p := runProxy
			if tc.proxyTLS {
				p = runProxyTLS
			}

			o := runOrigin
			if tc.proxyTLS {
				o = runOriginTLS
			}

			do(t, p, o)
		})
	}
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
	testDoMatrix(t, func(t *testing.T, proxy, origin srvFunc) {
		proxyURL, done := runProxy(t, 200)
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
	})
}

func TestProxyOKAuth(t *testing.T) {
	testDoMatrix(t, func(t *testing.T, proxy, origin srvFunc) {
		proxyURL, done := runProxy(t, 200)
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
	})
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

	proxyURL := "http://i_do_not_exist.local"

	measureOne(proxyURL, Target{URL: "http://i_wont_get_called", Insecure: true}, &proxyclient.AuthMethod{})

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

func TestProxyBadProxy502(t *testing.T) {
	testDoMatrix(t, func(t *testing.T, proxy, origin srvFunc) {
		proxyURL, done := runProxy(t, 502)
		defer done()

		originURL, done := runOrigin(t, 200)
		defer done()

		measureOne(proxyURL, Target{URL: originURL, Insecure: true}, &proxyclient.AuthMethod{})

		requireCounter(t,
			proxyConnectionSuccesses,
			prometheus.Labels{"proxy_url": proxyURL},
			0,
		)
		requireCounter(t,
			proxyConnectionErrors,
			prometheus.Labels{"proxy_url": proxyURL, "cause": "proxy"},
			1,
		)
	})
}

func TestProxyBadOrigin502(t *testing.T) {
	testDoMatrix(t, func(t *testing.T, proxy, origin srvFunc) {
		proxyURL, done := runProxy(t, 200)
		defer done()

		originURL, done := runOrigin(t, 502)
		defer done()

		measureOne(proxyURL, Target{URL: originURL, Insecure: true}, &proxyclient.AuthMethod{})

		requireCounter(t,
			proxyConnectionSuccesses,
			prometheus.Labels{"proxy_url": proxyURL},
			0,
		)
		requireCounter(t,
			proxyConnectionErrors,
			prometheus.Labels{"proxy_url": proxyURL, "cause": "proxy"},
			1,
		)
	})
}

func TestProxyBadOrigin500(t *testing.T) {
	testDoMatrix(t, func(t *testing.T, proxy, origin srvFunc) {
		proxyURL, done := runProxy(t, 200)
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
	})
}

func TestProxyBadOriginRST(t *testing.T) {
	testDoMatrix(t, func(t *testing.T, proxy, origin srvFunc) {
		proxyURL, done := runProxy(t, 200)
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
	})
}
