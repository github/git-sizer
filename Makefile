PACKAGE := github.com/github/git-sizer
GO111MODULES := 1
export GO111MODULES

GO := $(CURDIR)/script/go

GO_LDFLAGS := -X main.BuildVersion=$(shell git describe --tags --always --dirty || echo unknown)
GOFLAGS := -ldflags "$(GO_LDFLAGS)"

ifdef USE_ISATTY
GOFLAGS := $(GOFLAGS) --tags isatty
endif

.PHONY: all
all: bin/git-sizer

.PHONY: bin/git-sizer
bin/git-sizer:
	mkdir -p bin
	$(GO) build $(GOFLAGS) -o $@ .

# Cross-compile for a bunch of common platforms. Note that this
# doesn't work with USE_ISATTY:
.PHONY: common-platforms
common-platforms:

# Create releases for a bunch of common platforms. Note that this
# doesn't work with USE_ISATTY, and VERSION must be set on the command
# line; e.g.,
#
#     make releases VERSION=1.2.3
.PHONY: releases
releases:

# Define rules for a bunch of common platforms that are supported by go; see
#     https://golang.org/doc/install/source#environment
# You can compile for any other platform in that list by running
#     make GOOS=foo GOARCH=bar

define PLATFORM_template =
.PHONY: bin/git-sizer-$(1)-$(2)$(3)
bin/git-sizer-$(1)-$(2)$(3):
	mkdir -p bin
	GOOS=$(1) GOARCH=$(2) $$(GO) build $$(GOFLAGS) -ldflags "-X main.ReleaseVersion=$$(VERSION)" -o $$@ .
common-platforms: bin/git-sizer-$(1)-$(2)$(3)

# Note that releases don't include code from vendor (they're only used
# for testing), so no license info is needed from those projects.
.PHONY: releases/git-sizer-$$(VERSION)-$(1)-$(2).zip
releases/git-sizer-$$(VERSION)-$(1)-$(2).zip: bin/git-sizer-$(1)-$(2)$(3)
	if test -z "$$(VERSION)"; then echo "Please set VERSION to make releases"; exit 1; fi
	mkdir -p releases/tmp-$$(VERSION)-$(1)-$(2)
	cp README.md LICENSE.md releases/tmp-$$(VERSION)-$(1)-$(2)
	cp bin/git-sizer-$(1)-$(2)$(3) releases/tmp-$$(VERSION)-$(1)-$(2)/git-sizer$(3)
	cp $$$$($$(GO) list -f '{{.Dir}}' github.com/spf13/pflag)/LICENSE \
		releases/tmp-$$(VERSION)-$(1)-$(2)/LICENSE-spf13-pflag
	rm -f $$@
	zip -j $$@ releases/tmp-$$(VERSION)-$(1)-$(2)/*
	rm -rf releases/tmp-$$(VERSION)-$(1)-$(2)
releases: releases/git-sizer-$$(VERSION)-$(1)-$(2).zip
endef

$(eval $(call PLATFORM_template,linux,amd64))
$(eval $(call PLATFORM_template,linux,386))

$(eval $(call PLATFORM_template,darwin,amd64))
$(eval $(call PLATFORM_template,darwin,arm64))

$(eval $(call PLATFORM_template,windows,amd64,.exe))
$(eval $(call PLATFORM_template,windows,386,.exe))

.PHONY: test
test: bin/git-sizer gotest

.PHONY: gotest
gotest:
	$(GO) test -timeout 60s $(GOFLAGS) ./...

.PHONY: clean
clean:
	rm -rf bin
