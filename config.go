package main

import (
	"errors"
	"io/ioutil"
	"log"

	"github.com/criteo/http-proxy-exporter/proxyclient"
	yaml "gopkg.in/yaml.v2"
)

// Config is a configuration file
type Config struct {
	AuthMethods   map[string]*proxyclient.AuthMethod `yaml:"auth_methods,omitempty"`
	Proxies       []string                           `yaml:"proxies"`
	Targets       []string                           `yaml:"targets"`
	SourceAddress string                             `yaml:"source_address,omitempty"`
	ListenPort    int                                `yaml:"listen_port,omitempty"`
	Interval      int                                `yaml:"interval,omitempty"`
	Insecure      bool                               `yaml:"insecure,omitempty"`
	Debug         bool                               `yaml:"debug,omitempty"`
}

// loadConfig loads a configuration file and returns the corresponding struct pointer
func loadConfig(filename string) (*Config, error) {
	file, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Printf("could not open config file : %s", err)
		return &config, err
	}
	err = yaml.Unmarshal(file, &config)
	if err != nil {
		log.Printf("error while parsing config file : %s", err)
		return &config, err
	}
	// fill the type of AuthMethod using the key in configuration
	for authName := range config.AuthMethods {
		config.AuthMethods[authName].Type = authName
	}
	return &config, nil
}

// verifyConfig ensure that the provided configuration is valid
func verifyConfig(config *Config) []error {
	var errs []error

	if len(config.Proxies) < 1 {
		errs = append(errs, errors.New("at least one proxy must be provided"))
	}
	if len(config.Targets) < 1 {
		errs = append(errs, errors.New("at least one target must be provided"))
	}
	return errs
}
