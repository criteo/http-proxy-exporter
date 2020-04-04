package proxyclient

import (
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

// RequestConfig is used to build a request
type RequestConfig struct {
	Target     string
	Proxy      string
	Auth       *AuthMethod
	SourceAddr string
	Insecure   bool
	Timeout    time.Duration
}

// AuthMethod represent a method to authenticate with a proxy
type AuthMethod struct {
	Type   string            `yaml:"type"`
	Params map[string]string `yaml:"params"`
}

// PreparedRequest contains everything needed to make a request through a proxy
type PreparedRequest struct {
	Client   *http.Client
	Request  *http.Request
	ProxyURL *url.URL
}

func basicAuth(username, password string) string {
	auth := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", username, password)))
	return fmt.Sprintf("Basic %s", auth)
}

func proxifiedTransport(proxyURL *url.URL, targetScheme string, sourceAddr string, insecure bool) (*http.Transport, error) {
	var tlsConfig *tls.Config
	if proxyURL.Scheme == "https" || targetScheme == "https" {
		tlsConfig = &tls.Config{
			InsecureSkipVerify: insecure,
		}
	}
	if proxyURL.String() == "" {
		// override to nil if no proxy has been set (will use system proxy)
		proxyURL = nil
	}

	localAddr, err := net.ResolveIPAddr("ip", sourceAddr)
	if err != nil {
		return nil, err
	}

	localTCPAddr := net.TCPAddr{
		IP: localAddr.IP,
	}

	tr := &http.Transport{
		Proxy:           http.ProxyURL(proxyURL),
		TLSClientConfig: tlsConfig,
		DialContext: (&net.Dialer{
			LocalAddr: &localTCPAddr,
			DualStack: true,
		}).DialContext,
		MaxIdleConnsPerHost: 1,
		DisableKeepAlives:   true,
	}
	return tr, nil
}

func clientByAuthType(scheme string, auth *AuthMethod, tr *http.Transport, timeout time.Duration) *http.Client {
	if auth.Type == "basic" {
		if scheme == "https" {
			// basicauth
			auth := basicAuth(auth.Params["username"], auth.Params["password"])
			tr.ProxyConnectHeader = http.Header{}
			tr.ProxyConnectHeader.Set("Proxy-Authorization", auth)
		}
	}
	return &http.Client{
		Transport: tr,
		Timeout:   timeout,
	}
}

func requestByAuthType(target string, auth *AuthMethod) (*http.Request, error) {
	scheme, err := GetURLScheme(target)
	if err != nil {
		return nil, errors.New("error while getting target scheme")
	}

	if auth.Type == "basic" {
		req, err := http.NewRequest("GET", target, nil)
		if err != nil {
			return nil, fmt.Errorf("error while creating request: %s", err)
		}
		if scheme == "http" {
			auth := basicAuth(auth.Params["username"], auth.Params["password"])
			req.Header.Add("Proxy-Authorization", auth)
		}
		return req, nil
	}
	if auth.Type == "" {
		return http.NewRequest("GET", target, nil)
	}
	return nil, fmt.Errorf("unknown or unsupported authType: %s", auth.Type)
}

// MakeClientAndRequest prepares a client and a request
func MakeClientAndRequest(rc RequestConfig) (*PreparedRequest, error) {
	// detect target URL scheme
	scheme, err := GetURLScheme(rc.Target)
	if err != nil {
		return nil, fmt.Errorf("could not detect scheme for %s: %s", rc.Target, err)
	}

	// get the right proxy based on target scheme, create the associated transport
	proxyURL, err := url.Parse(rc.Proxy)
	if err != nil {
		return nil, fmt.Errorf("could not parse proxy URL: %s", err)
	}
	tr, err := proxifiedTransport(proxyURL, scheme, rc.SourceAddr, rc.Insecure)
	if err != nil {
		return nil, err
	}

	// create the client and the actual request
	client := clientByAuthType(scheme, rc.Auth, tr, rc.Timeout)
	req, err := requestByAuthType(rc.Target, rc.Auth)
	if err != nil {
		return nil, fmt.Errorf("error during request creation: %s", err)
	}
	return &PreparedRequest{Client: client, Request: req, ProxyURL: proxyURL}, nil
}
