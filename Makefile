# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get

APPNAME=$(shell basename "$(PWD)")
VERSION=$(shell git describe --tags || echo "0")
TIME=$(shell TZ='UTC' date)
BUILD=$(shell git rev-parse HEAD || echo "0")

# Use linker flags to provide version/build settings
LDFLAGS=-ldflags '-s -w -X "main.appName=$(APPNAME)" -X "main.buildVersion=$(VERSION)" -X "main.buildNumber=$(BUILD)" -X "main.buildTime=$(TIME)"'

all: test build
build:
	$(GOBUILD) $(LDFLAGS) -o $(APPNAME)-$(VERSION) -v
test:
	$(GOTEST) -v
clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_UNIX)
run:
	$(GOBUILD) -o $(BINARY_NAME) -v ./...
	./$(BINARY_NAME)
