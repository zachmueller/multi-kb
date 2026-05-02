BINARY_NAME := multi-kb
MODULE := github.com/zmueller/multi-kb
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -s -w -X '$(MODULE)/internal/cmd.version=$(VERSION)'
CGO_ENABLED := 0

PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

.PHONY: build test lint clean build-all

build:
	CGO_ENABLED=$(CGO_ENABLED) go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME) ./cmd/multi-kb/

test:
	go test ./...

test-race:
	go test -race ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/ dist/

build-all: clean
	$(foreach platform,$(PLATFORMS),\
		$(eval OS := $(word 1,$(subst /, ,$(platform))))\
		$(eval ARCH := $(word 2,$(subst /, ,$(platform))))\
		$(eval EXT := $(if $(filter windows,$(OS)),.exe,))\
		CGO_ENABLED=$(CGO_ENABLED) GOOS=$(OS) GOARCH=$(ARCH) \
			go build -ldflags "$(LDFLAGS)" \
			-o dist/$(BINARY_NAME)-$(OS)-$(ARCH)$(EXT) ./cmd/multi-kb/ ;)
