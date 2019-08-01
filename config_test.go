package main

import (
	"testing"

	"github.com/criteo/http-proxy-exporter/proxyclient"
	"github.com/stretchr/testify/assert"
)

func TestLoadConfig(t *testing.T) {
	var err error

	_, err = loadConfig("config.example.yml")
	assert.Nil(t, err, "loadConfig is expected to work on example file, got : %s", err)

	_, err = loadConfig("idontexist.yml")
	assert.NotNil(t, err, "loadConfig should have returned an error")
}

func TestVerifyConfig(t *testing.T) {
	var errs []error

	proxies := []string{
		"http://proxy/",
		"https://securedproxy/",
	}

	auth := &proxyclient.AuthMethod{
		Type: "basic",
		Params: map[string]string{
			"Username": "janedoe",
			"Password": "Who am I ?",
		},
	}

	targets := []string{"https://www.example.com"}

	config := Config{
		AuthMethods: map[string]*proxyclient.AuthMethod{
			"basic": auth,
		},
		Proxies: proxies,
		Targets: targets,
	}

	errs = verifyConfig(&config)
	assert.Len(t, errs, 0)

	noProxies := config
	noProxies.Proxies = []string{}
	errs = verifyConfig(&noProxies)
	assert.Len(t, errs, 1)

	noTargets := config
	noTargets.Targets = []string{}
	errs = verifyConfig(&noTargets)
	assert.Len(t, errs, 1)

	errs = verifyConfig(&Config{})
	assert.Len(t, errs, 3)
}
