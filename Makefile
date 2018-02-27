PACKAGE := github.com/github/git-sizer
ROOTDIR := $(abspath $(CURDIR))
GOPATH := $(ROOTDIR)/.gopath
export GOPATH

GO := $(CURDIR)/script/go
GOFMT := $(CURDIR)/script/gofmt

GOFLAGS := \
	-ldflags "-X main.BuildVersion=$(shell git rev-parse HEAD) -X main.BuildDescribe=$(shell git describe --tags --always --dirty)"

ifdef USE_ISATTY
GOFLAGS := $(GOFLAGS) --tags isatty
endif

GO_SRCS := $(shell cd $(GOPATH)/src/$(PACKAGE) && $(GO) list -f '{{$$ip := .ImportPath}}{{range .GoFiles}}{{printf ".gopath/src/%s/%s\n" $$ip .}}{{end}}{{range .CgoFiles}}{{printf ".gopath/src/%s/%s\n" $$ip .}}{{end}}{{range .TestGoFiles}}{{printf ".gopath/src/%s/%s\n" $$ip .}}{{end}}{{range .XTestGoFiles}}{{printf ".gopath/src/%s/%s\n" $$ip .}}{{end}}' ./...)

.PHONY: all
all: bin/git-sizer

.PHONY: bin/git-sizer
bin/git-sizer:
	mkdir -p bin
	cd $(GOPATH)/src/$(PACKAGE) && $(GO) build $(GOFLAGS) -o $(ROOTDIR)/$@ $(PACKAGE)

# Cross-compile for a bunch of common platforms. Note that this
# doesn't work with USE_ISATTY:
.PHONY: common-platforms
common-platforms: \
	bin/git-sizer-linux-amd64 \
	bin/git-sizer-linux-386 \
	bin/git-sizer-darwin-amd64 \
	bin/git-sizer-darwin-386 \
	bin/git-sizer-windows-amd64.exe \
	bin/git-sizer-windows-386.exe

# Define rules for a bunch of common platforms that are supported by go; see
#     https://golang.org/doc/install/source#environment
# You can compile for any other platform in that list by running
#     make GOOS=foo GOARCH=bar

define PLATFORM_template =
.PHONY: bin/git-sizer-$(1)-$(2)$(3)
bin/git-sizer-$(1)-$(2)$(3):
	mkdir -p bin
	cd $$(GOPATH)/src/$$(PACKAGE) && GOOS=$(1) GOARCH=$(2) $$(GO) build $$(GOFLAGS) -o $$(ROOTDIR)/$$@ $$(PACKAGE)
endef

$(eval $(call PLATFORM_template,linux,amd64))
$(eval $(call PLATFORM_template,linux,386))

$(eval $(call PLATFORM_template,darwin,386))
$(eval $(call PLATFORM_template,darwin,amd64))

$(eval $(call PLATFORM_template,windows,amd64,.exe))
$(eval $(call PLATFORM_template,windows,386,.exe))

.PHONY: test
test: bin/git-sizer gotest

.PHONY: gotest
gotest:
	cd $(GOPATH)/src/$(PACKAGE) && $(GO) test -timeout 60s $(GOFLAGS) ./...

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
