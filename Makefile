# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOINSTALL=$(GOCMD) install

APPNAME=$(shell basename "$(PWD)")
VERSION=$(shell git describe --tags || echo "0")
TIME=$(shell TZ='UTC' date)
BUILD=$(shell git rev-parse HEAD || echo "0")

BINARY_NAME=$(APPNAME)-$(VERSION)

# Use linker flags to provide version/build settings
LDFLAGS=-ldflags '-s -w -X "main.appName=$(APPNAME)" -X "main.buildVersion=$(VERSION)" -X "main.buildNumber=$(BUILD)" -X "main.buildTime=$(TIME)"'

all: test install
build:
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) -v
build-travis:
	$(GOBUILD) $(LDFLAGS) -o $(APPNAME) -v
test:
	$(GOTEST) -v
clean:
	$(GOCLEAN)
	rm -f $(APPNAME)-*
run:
	$(GOBUILD) -o $(BINARY_NAME) -v ./...
	./$(BINARY_NAME)
install:
	$(GOINSTALL)
