package proxyclient

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetURLScheme(t *testing.T) {
	var scheme string
	var err error

	scheme, err = GetURLScheme("https://www.google.com/")
	assert.Equal(t, "https", scheme)
	assert.Nil(t, err)

	_, err = GetURLScheme("this is not a valid URL")
	assert.NotNil(t, err)
}

func TestProxyByScheme(t *testing.T) {

	proxiesList := ProxiesList{
		HTTP:  "http://proxy-http/",
		HTTPS: "http://proxy-https/",
	}

	var u *url.URL
	var e *url.URL
	var err error

	u, err = proxyByScheme("http", proxiesList)
	e, _ = url.Parse(proxiesList.HTTP)
	assert.Nil(t, err)
	assert.Equal(t, e, u)

	u, err = proxyByScheme("https", proxiesList)
	e, _ = url.Parse(proxiesList.HTTPS)
	assert.Nil(t, err)
	assert.Equal(t, e, u)

	u, err = proxyByScheme("toto", proxiesList)
	e, _ = url.Parse(proxiesList.HTTPS)
	assert.Nil(t, u)
	assert.NotNil(t, e)
}
