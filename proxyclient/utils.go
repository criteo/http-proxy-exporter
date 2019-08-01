package proxyclient

import (
	"errors"
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
