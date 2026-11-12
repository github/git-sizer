# Creating releases of `git-sizer`

1.  Create a release tag and push it to GitHub:

        VERSION=1.2.3
        git tag -as v$VERSION
        git push origin v$VERSION

    Pushing the tag triggers `.github/workflows/release.yml`, which builds the release archives for all supported platforms and creates a draft release on GitHub.

2.  Go to the [releases page](https://github.com/github/git-sizer/releases).

3.  Open the generated draft release, add the release notes, and click "Publish release".
