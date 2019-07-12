# http-proxy-exporter

http-proxy-exporter makes request to HTTP(S) targets via a proxy using [HTTP Basic authentication](https://en.wikipedia.org/wiki/Basic_access_authentication) and expose performance statistics in a Prometheus-friendly format.

## Getting started

You should have a working Golang environment [setup](https://golang.org/doc/install).

```
go get github.com/criteo/http-proxy-exporter
```

### Build

```
cd $GOPATH/src/github.com/criteo/http-proxy-exporter/
make build
```

### Install

```
cd $GOPATH/src/github.com/criteo/http-proxy-exporter/
make
```

### Running http-proxy-exporter

The application requires a configuration file (see the [example configuration file](config.example.yml)). By default, it will check for a `config.yml` file in the current directory.

```
./http-proxy-exporter -c $PATH_TO_CONFIG_FILE/config.yml
```

# License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.