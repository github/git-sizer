PACKAGE := github.com/github/git-sizer
ROOTDIR := $(abspath $(CURDIR))
GOPATH := $(ROOTDIR)/.gopath
export GOPATH

GO := $(CURDIR)/script/go
GOFMT := $(CURDIR)/script/gofmt

GOFLAGS := \
	--tags "static" \
	-ldflags "-X main.BuildVersion=$(shell git rev-parse HEAD) -X main.BuildDescribe=$(shell git describe --tags --always --dirty)"
GO_PKGS := $(PACKAGE) \
	$(PACKAGE)/isatty \
	$(PACKAGE)/meter \
	$(PACKAGE)/sizes
GO_SRCS := $(shell $(GO) list -f '{{$$ip := .ImportPath}}{{range .GoFiles}}{{printf ".gopath/src/%s/%s\n" $$ip .}}{{end}}{{range .CgoFiles}}{{printf ".gopath/src/%s/%s\n" $$ip .}}{{end}}{{range .TestGoFiles}}{{printf ".gopath/src/%s/%s\n" $$ip .}}{{end}}{{range .XTestGoFiles}}{{printf ".gopath/src/%s/%s\n" $$ip .}}{{end}}' $(GO_PKGS))

.PHONY: all
all: bin/git-sizer

.PHONY: bin/git-sizer
bin/git-sizer:
	mkdir -p bin
	$(GO) build $(GOFLAGS) -o $@ $(PACKAGE)

.PHONY: test
test: bin/git-sizer gotest

.PHONY: gotest
gotest:
	$(GO) test -timeout 60s $(GOFLAGS) ./...

.PHONY: gofmt
gofmt:
	$(GOFMT) -l -w $(GO_SRCS) | sed -e 's/^/Fixing /'

.PHONY: goimports
goimports:
	goimports -l -w -e $(GO_SRCS)

.PHONY: govet
govet:
	$(GO) vet ./...

.PHONY: clean
clean:
	rm -rf bin

.PHONY: srcs
srcs:
	@printf "%s\n" $(GO_SRCS)
