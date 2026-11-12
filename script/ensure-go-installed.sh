# This file is meant to be sourced with ROOTDIR set.

if [ -z "$ROOTDIR" ]; then
    echo 1>&2 'ensure-go-installed.sh invoked without ROOTDIR set!'
fi

# Is go installed, and at least 1.26.10 (or 1.27.3+)?
go_ok() {
    set -- $(go version 2>/dev/null |
                sed -n 's/.*go\([0-9][0-9]*\)\.\([0-9][0-9]*\)\.\([0-9][0-9]*\).*/\1 \2 \3/p' |
                head -n 1)
    if [ $# -ne 3 ] || [ "$1" -ne 1 ]; then
        return 1
    fi
    [ "$2" -gt 27 ] ||
        { [ "$2" -eq 27 ] && [ "$3" -ge 3 ]; } ||
        { [ "$2" -eq 26 ] && [ "$3" -ge 10 ]; }
}

# If a local go is installed, use it.
set_up_vendored_go() {
    GO_VERSION=go1.27.3
    VENDORED_GOROOT="$ROOTDIR/vendor/$GO_VERSION/go"
    if [ -x "$VENDORED_GOROOT/bin/go" ]; then
        export GOROOT="$VENDORED_GOROOT"
        export PATH="$GOROOT/bin:$PATH"
    fi
}

set_up_vendored_go

if ! go_ok; then
    script/install-vendored-go >/dev/null || exit 1
    set_up_vendored_go
fi
