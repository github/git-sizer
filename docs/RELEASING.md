# Creating releases of `git-sizer`

1.  Create a release tag and push it to GitHub:

        VERSION=1.2.3
        git tag -as v$VERSION
        git push origin v$VERSION

2.  Build the release for the major platforms:

        make releases VERSION=$VERSION

    The output is a bunch of ZIP files written to directory `releases/`.

3.  Go to the [releases page](https://github.com/github/git-sizer/releases).

4.  Click on "Draft a new release".

5.  Select the tag, add the required information, and upload the zip files.

6.  Click "Publish release".
