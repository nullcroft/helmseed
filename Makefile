BINARY := helmseed
VERSION := v0.1.1
BUILD_DIR := ./bin

.PHONY: all build test lint clean purge tidy

all: clean test lint build

build:
	@mkdir -p $(BUILD_DIR)
	go build -ldflags="-X 'github.com/nullcroft/helmseed/cmd.version=$(VERSION)'" -o $(BUILD_DIR)/$(BINARY) .


test:
	go test -v ./... $(ARGS)


lint:
	@which golangci-lint > /dev/null || (echo "golangci-lint not found, install from https://golangci-lint.run" && exit 1)
	golangci-lint run ./...


tidy:
	go mod tidy


clean:
	rm -rf $(BUILD_DIR)


purge: clean
	rm -rf ./.helm
	rm -rf "$(if $(XDG_CACHE_HOME),$(XDG_CACHE_HOME),$(HOME)/.cache)/helmseed"
