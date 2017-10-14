ROOTDIR := $(abspath $(CURDIR))
export ROOTDIR

GO := $(CURDIR)/script/go
GOFMT := $(CURDIR)/script/gofmt

BIN := libexec

GOBIN := $(abspath $(BIN))
export GOBIN

GOFLAGS := \
	--tags "static" \
	-ldflags "-X main.BuildVersion=$(shell git rev-parse HEAD) -X main.BuildDescribe=$(shell git describe --tags --always --dirty)"
GO_CMDS := \
	$(BIN)/git-sizer
GO_PKGS := $(shell cd .gopath/src; find github.com/github/git-sizer/ -type f -name '*.go' | xargs -n1 dirname | sort -u)
SHELL_CMDS :=
RUBY_CMDS :=
DEPS := \
	github.com/stretchr/testify
GO_SRCS := $(shell find src -type f -name '*.go')

TEST_SH_RUNNERS := 2

.PHONY: all
all: $(GO_CMDS) $(SHELL_CMDS) $(RUBY_CMDS)

libexec/%: bin/% $(GO_SRCS)
	$(GO) install $(GOFLAGS) github.com/github/git-sizer/$*

.PRECIOUS: bin/%
bin/%: src/shim.sh
	mkdir -p bin
	cp $< bin/$*
	chmod +x bin/$*

.PHONY: $(SHELL_CMDS)
$(SHELL_CMDS): $(BIN)/%: bin/%.sh
	cp $< $@

.PHONY: $(RUBY_CMDS)
$(RUBY_CMDS): $(BIN)/%: bin/%.rb
	cp $< $@

.PHONY: deps
deps:
	$(GO) get $(DEPS)

.PHONY: test
test: gotest shtest

.PHONY: gotest
gotest:
	$(GO) test -timeout 60s $(GOFLAGS) $(GO_PKGS)

.PHONY: shtest
shtest: libexec/git-sizer
	ls -1 test/test-*.sh | xargs -I % -P $(TEST_SH_RUNNERS) -n 1 $(SHELL) % --batch

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
	rm -rf bin libexec
