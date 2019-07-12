package proxyclient

import (
	"errors"
	"fmt"
	"net/url"
)

// GetURLScheme returns the scheme of a URL
func GetURLScheme(rawURL string) (string, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil || parsedURL.Scheme == "" {
		return "", errors.New("could not detect URL scheme")
	}
	return parsedURL.Scheme, nil
}

func proxyByScheme(scheme string, proxiesList ProxiesList) (*url.URL, error) {
	var u string
	if scheme == "http" {
		u = proxiesList.HTTP
	} else if scheme == "https" {
		u = proxiesList.HTTPS
	} else {
		return nil, fmt.Errorf("scheme %s is not supported", scheme)
	}
	return url.Parse(u)
}

// ProxiesArray returns a flat list of all proxies in a ProxiesList
func ProxiesArray(proxies ProxiesList) []string {
	var plist []string
	if proxies.HTTP != "" {
		plist = append(plist, proxies.HTTP)
	}
	if proxies.HTTPS != "" {
		plist = append(plist, proxies.HTTPS)
	}
	return plist
}
