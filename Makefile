PACKAGE := github.com/github/git-sizer
ROOTDIR := $(abspath $(CURDIR))
GOPATH := $(ROOTDIR)/.gopath
export GOPATH

GO := $(CURDIR)/script/go
GOFMT := $(CURDIR)/script/gofmt

BIN := bin

GOFLAGS := \
	--tags "static" \
	-ldflags "-X main.BuildVersion=$(shell git rev-parse HEAD) -X main.BuildDescribe=$(shell git describe --tags --always --dirty)"
GO_CMDS := \
	$(BIN)/git-sizer
GO_PKGS := $(shell cd .gopath/src; find github.com/github/git-sizer/ -type f -name '*.go' | xargs -n1 dirname | sort -u)
GO_SRCS := $(shell find src -type f -name '*.go')

.PHONY: all
all: $(GO_CMDS)

$(BIN)/%: $(GO_SRCS) | $(BIN)
	$(GO) build $(GOFLAGS) -o $@ $(PACKAGE)/$*

$(BIN):
	mkdir -p $(BIN)

.PHONY: test
test: gotest

.PHONY: gotest
gotest:
	$(GO) test -timeout 60s $(GOFLAGS) $(GO_PKGS)

.PHONY: gofmt
gofmt:
	find src test -name "*.go" -print0 | xargs -0 $(GOFMT) -l -w | sed -e 's/^/Fixing /'

.PHONY: goimports
goimports:
	find src -name "*.go" -print0 | xargs -0 goimports -l -w -e

.PHONY: govet
govet:
	$(GO) vet $(GO_PKGS)

.PHONY: clean
clean:
	rm -rf bin
