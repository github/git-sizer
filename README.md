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

        bin/git-sizer [<opt>...] [<path-to-git-repository>]

        bin/git-sizer --help

    To get a summary of the current repository, all you need is

        bin/git-sizer

    You can also use the `--json` option to get output in JSON format, including the raw numbers. Note that if a value overflows its counter (which should only happen for malicious repositories), the corresponding value is truncated to 2³²-1 or 2⁶⁴-1, depending on the size of the counter.


## Example output

Here is the output for `torvalds/linux` as of this writing:

```
$ git-sizer linux
| Name                      | Value     | Level of concern               |
| ------------------------- | --------- | ------------------------------ |
| unique_commit_count       |   708 k   | *                              |
| unique_commit_size        |   513 MiB | **                             |
| max_commit_size           |  72.7 KiB | *                              |
| max_history_depth         |   134 k   |                                |
| max_parent_count          |    66     | ******                         |
| unique_tree_count         |  3.31 M   | **                             |
| unique_tree_size          |  8.78 GiB | ****                           |
| unique_tree_entries       |   257 M   | *****                          |
| max_tree_entries          |  1.63 k   |                                |
| unique_blob_count         |  1.60 M   | *                              |
| unique_blob_size          |  54.4 GiB | *****                          |
| max_blob_size             |  13.5 MiB | *                              |
| unique_tag_count          |   530     |                                |
| max_tag_depth             |     1     | *                              |
| reference_count           |   535     |                                |
| max_path_depth            |    14     | *                              |
| max_path_length           |   134 B   | *                              |
| expanded_tree_count       |  4.32 k   | **                             |
| expanded_blob_count       |  61.4 k   | *                              |
| expanded_blob_size        |   744 MiB |                                |
| expanded_link_count       |    40     |                                |
| expanded_submodule_count  |     0     |                                |
```

The `unique_*` numbers are counts of distinct objects, not including repetition. `unique_*_size` are the sums of the sizes of the corresponding objects in their uncompressed form, measured in bytes. (`unique_tag_count` refers to annotated tag objects, not tag references.)

The `max_*` numbers are the maximum values seen anywhere in the repository. `max_path_depth` is the largest number of path components seen, and `max_path_length` is the longest path in terms of bytes.

The `expanded_*` numbers are the largest numbers that would be seen for any single checkout of the repository.

The asterisks indicate values that seem unusually high. The more asterisks, the more trouble this value is expected to cause. Exclamation points indicate values that are extremely high (i.e., equivalent to more than 30 asterisks).

The Linux repository is large by most standards, and as you can see, it is pushing some of Git's limits. And indeed, some Git operations on the Linux repository (e.g., `git fsck`, `git gc`) take a while. But due to its mostly sane structure, none of its dimensions are wildly out of proportion to the size of the code base, so it can be managed successfully using Git.

Here is the output for one of the famous ["git bomb"](https://kate.io/blog/git-bomb/) repositories:

```
$ git-sizer --all test/data/git-bomb.git

| Name                      | Value     | Level of concern               |
| ------------------------- | --------- | ------------------------------ |
| unique_commit_count       |     3     |                                |
| unique_commit_size        |   606 B   |                                |
| max_commit_size           |   218 B   |                                |
| max_history_depth         |     3     |                                |
| max_parent_count          |     1     |                                |
| unique_tree_count         |    12     |                                |
| unique_tree_size          |  3.48 KiB |                                |
| unique_tree_entries       |   122     |                                |
| max_tree_entries          |    11     |                                |
| unique_blob_count         |     3     |                                |
| unique_blob_size          |  3.65 KiB |                                |
| max_blob_size             |  1.82 KiB |                                |
| unique_tag_count          |     0     |                                |
| max_tag_depth             |     0     |                                |
| reference_count           |     1     |                                |
| max_path_depth            |    11     | *                              |
| max_path_length           |    29 B   |                                |
| expanded_tree_count       |  1.11 G   | !!!!!!!!!!!!!!!!!!!!!!!!!!!!!! |
| expanded_blob_count       |     ∞     | !!!!!!!!!!!!!!!!!!!!!!!!!!!!!! |
| expanded_blob_size        |  83.8 GiB | !!!!!!!!!!!!!!!!!!!!!!!!!!!!!! |
| expanded_link_count       |     0     |                                |
| expanded_submodule_count  |     0     |                                |
```

This repository is mischievously constructed to have a pathological tree structure, with the same directories repeated over and over again. As a result, even though the entire repository is less than 20 kb in size, when checked out it would explode into over a billion directories containing over ten billion files. (`git-sizer` prints `∞` for the blob count because the true number has overflowed its 32-bit counter for that field.)
