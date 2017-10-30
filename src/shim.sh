#!/bin/bash

set -e

# This is a shim to sit in front of the binaries and load the
# appropriate environment variables for the environment set in
# GIT_SIZER_ENV.

GIT_SIZER_ENV=${GIT_SIZER_ENV:-"production"}

resolve_symlink() {
    local path="$1"
    while [ -h "$path" ]; do
        local target="$(readlink "$path")"
        local target_dir="$(dirname "$target")"
        local path_dir="$(dirname "$path")"
        local full_target_dir="$(cd -P "$path_dir" && cd -P "$target_dir" && pwd)"
        test -z "$full_target_dir" && break
        path="$full_target_dir/$(basename "$target")"
    done
    printf "%s" "$path"
}

self="$(resolve_symlink "$0")"

# The base directory, which includes bin/ and whatever else once deployed
base=$(cd -P "$(dirname "$self")/.."; pwd)

if [ -f "${base}/.app-config/${GIT_SIZER_ENV}.sh" ]; then
   source "${base}/.app-config/${GIT_SIZER_ENV}.sh"
fi

myname=$(basename "$self")
exec "${base}/libexec/${myname}" "$@"
