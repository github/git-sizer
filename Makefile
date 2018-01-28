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
GO_CMDS := $(BIN)/git-sizer
GO_PKGS := $(shell cd .gopath/src && find github.com/github/git-sizer/ -type f -name '*.go' | xargs -n1 dirname | grep -v '^github.com/github/git-sizer/vendor/' | sort -u)
GO_SRCS := $(shell find . -type f -name '*.go' | grep -v '^\./vendor/' | sort -u)

.PHONY: all
all: $(GO_CMDS)

$(BIN)/%: $(GO_SRCS) | $(BIN)
	$(GO) build $(GOFLAGS) -o $@ $(PACKAGE)/$*

$(BIN):
	mkdir -p $(BIN)

.PHONY: test
test: $(GO_CMDS) gotest

.PHONY: gotest
gotest:
	$(GO) test -timeout 60s $(GOFLAGS) $(GO_PKGS)

.PHONY: gofmt
gofmt:
	$(GOFMT) -l -w $(GO_SRCS) | sed -e 's/^/Fixing /'

.PHONY: goimports
goimports:
	goimports -l -w -e $(GO_SRCS)

.PHONY: govet
govet:
	$(GO) vet $(GO_PKGS)

.PHONY: clean
clean:
	rm -rf bin

.PHONY: srcs
srcs:
	@printf "%s\n" $(GO_SRCS)
