# Building `git-sizer` from source

Most people can just install a released version of `git-sizer`, [as described in the `README.md`](../README.md#getting-started). However, if you want to test a non-release version, or if you might want to contribute to `git-sizer`, you can also build it from source.


## Build and install using `go get`

1.  Make sure that you have a recent version of the [Go language toolchain](https://golang.org/doc/install) installed and that you have set `GOPATH`.

2.  Get `git-sizer` using `go get`:

        go get github.com/github/git-sizer

    This should fetch and compile the source code and write the executable file to `$GOPATH/bin/`.

3.  Either add `$GOPATH/bin` to your `PATH`, or copy the executable file (`git-sizer` or `git-sizer.exe`) to a directory that is already in your `PATH`.


## Build using `make`

This procedure is intended for experts and people who want to help develop `git-sizer`. It should work on Linux or OS X. On other Unix-like systems, this procedure is also likely to work, provided you first [install Go manually](https://golang.org/doc/install).

1.  Clone the `git-sizer` Git repository and switch to that directory:

        git clone https://github.com/github/git-sizer.git
        cd git-sizer

2.  Install Go if necessary and create and prepare a project-local `GOPATH`:

        script/bootstrap

3.  (Optional) Run the automated tests:

        make test

4.  Build `git-sizer`:

        make

    If you have a C toolchain set up, you can enable support for `isatty()` (which turns off `--progress` by default if output is not to a TTY) by running

        make USE_ISATTY=true

5.  Copy the resulting executable file (`bin/git-sizer`) to a directory in your `PATH`.

It is also possible to cross-compile for other platforms that are supported by Go. See the comments in the `Makefile` for more information.

Note that this procedure uses a project-local `GOPATH`. This means that you can clone the repository anywhere. The disadvantage is that Go tools need to know about this `GOPATH`. The `Makefile` and the scripts under `scripts/` take care of this automatically. But if you want to run `go` commands by hand, either first set your `GOPATH`:

    export GOPATH="$(pwd)/.gopath"

Or use `script/go` and `script/gofmt` rather than `go` and `gofmt`, respectively.

Unfortunately, some Go tools get confused by the symlink that is used to make the project-local `GOPATH` work. If you have this problem, it sometimes helps to run such commands from `.gopath/src/github.com/github/git-sizer/`. Alternatively, clone the project into the traditional place in your normal `GOPATH`.


## Making a release

See [`RELEASING.md`](RELEASING.md).
