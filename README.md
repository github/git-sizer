_Happy Git repositories are all alike; every unhappy Git repository is unhappy in its own way._ —Linus Tolstoy

# git-sizer

Is your Git repository busting at the seams?

* Is it too big overall? Ideally, Git repositories should be under 1 GiB, and they really start to get unwieldy over 5 GiB. Big repositories take a long time to clone and repack, and take a lot of disk space.

* Does it have too many references? They all have to be transferred to the client for every fetch, no matter how small. Try to limit them to a few tens of thousands at most.

* Does it include too many objects? The more objects, the longer it takes for Git to traverse the repository's history, for example when garbage-collecting.

* Does it include gigantic blobs? Git works best with small files. It's OK to have a few files in the megabyte range, but they should generally be the exception. Consider using [Git-LFS](https://git-lfs.github.com/) for storing your large files, especially those (e.g., media assets) that don't diff and merge usefully.

* Does it include many, many versions of large files, each one slightly changed from the one before? (Git is terrible at storing logfiles or database dumps!) The problem is that the full file often needs to be reconstructed, which is very expensive.

* Does it include gigantic trees? Every time a file is modified, Git has to create a new copy of every tree (i.e., every directory in the path) leading to the file. Huge trees make this expensive. Moreover, it is very expensive to iterate through history that contains huge trees. It's best to avoid directories with more than a couple of thousand entries. If you must store many files, it is better to shard them into smaller directories if possible.

* Does it have the same (or very similar) files repeated over and over again at different paths in a single commit? This can mean that even though your repository is a reasonable size overall, when you check it out it balloons into an enormous working copy. (Taken to an extreme, this is called a "git bomb".) It also makes some Git operations, like `fsck`, very expensive. It might be that you need to work more effectively with tags and branches.

* Does it include absurdly long path names? That's probably not going to work well with other tools. One or two hundred characters should be more than enough, even if you're writing Java.

* Are there other bizarre and questionable things in your repository?

    * Annotated tags pointing at one another in long chains?
    * Octopus merges with dozens of parents?
    * Commits with gigantic log messages?


## Getting started

1. Build:

        script/bootstrap
        make

2. Run:

        bin/git-sizer [<opt>...] <path-to-git-repository> [<object>...]

        bin/git-sizer --help

    For example, to get a summary of a whole repository,

        bin/git-sizer --all <path-to-git-repository>
