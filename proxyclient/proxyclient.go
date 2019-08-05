package proxyclient

import (
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

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

func basicAuth(username, password string) []string {
	auth := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", username, password)))
	return []string{fmt.Sprintf("Basic %s", auth)}
}

func proxifiedTransport(proxyURL *url.URL, insecure bool) *http.Transport {
	var tlsConfig *tls.Config
	if proxyURL.Scheme == "https" {
		tlsConfig = &tls.Config{
			InsecureSkipVerify: insecure,
		}
	}
	if proxyURL.String() == "" {
		// override to nil if no proxy has been set (will use system proxy)
		proxyURL = nil
	}
	tr := &http.Transport{
		Proxy:           http.ProxyURL(proxyURL),
		TLSClientConfig: tlsConfig,
	}
	return tr
}

func clientByAuthType(scheme string, auth *AuthMethod, tr *http.Transport) *http.Client {
	if auth.Type == "basic" {
		if scheme == "https" {
			// basicauth
			auth := basicAuth(auth.Params["username"], auth.Params["password"])
			proxyHeader := map[string][]string{
				"Proxy-Authorization": auth,
			}
			tr.ProxyConnectHeader = proxyHeader
		}
	}
	return &http.Client{Transport: tr}
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
			auth := basicAuth(auth.Params["username"], auth.Params["password"])[0]
			req.Header.Add("Proxy-Authorization", auth)
		}
		return req, nil
	}
	return nil, fmt.Errorf("unknown or unsupported authType: %s", auth.Type)
}

// MakeClientAndRequest prepares a client and a request
func MakeClientAndRequest(target string, proxy string, auth *AuthMethod, insecure bool) (*PreparedRequest, error) {
	// detect target URL scheme
	scheme, err := GetURLScheme(target)
	if err != nil {
		return nil, fmt.Errorf("could not detect scheme for %s: %s", target, err)
	}

	// get the right proxy based on target scheme, create the associated transport
	proxyURL, err := url.Parse(proxy)
	if err != nil {
		return nil, fmt.Errorf("could not parse proxy URL: %s", err)
	}
	tr := proxifiedTransport(proxyURL, insecure)

	// create the client and the actual request
	client := clientByAuthType(scheme, auth, tr)
	req, err := requestByAuthType(target, auth)
	if err != nil {
		return nil, fmt.Errorf("error during request creation: %s", err)
	}
	return &PreparedRequest{Client: client, Request: req, ProxyURL: proxyURL}, nil
}
