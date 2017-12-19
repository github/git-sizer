_Happy Git repositories are all alike; every unhappy Git repository is unhappy in its own way._ —Linus Tolstoy

# git-sizer

Is your Git repository busting at the seams?

* Is it too big overall? Ideally, Git repositories should be under 1 GiB, and (without special handling) they really start to get unwieldy over 5 GiB. Big repositories take a long time to clone and repack, and take a lot of disk space.

* Does it have too many references? They all have to be transferred to the client for every fetch, even if your clone is up-to-date. Try to limit them to a few tens of thousands at most.

* Does it include too many objects? The more objects, the longer it takes for Git to traverse the repository's history, for example when garbage-collecting.

* Does it include gigantic blobs? Git works best with small files. It's OK to have a few files in the megabyte range, but they should generally be the exception. Consider using [Git-LFS](https://git-lfs.github.com/) for storing your large files, especially those (e.g., media assets) that don't diff and merge usefully.

* Does it include many, many versions of large files, each one slightly changed from the one before? (Git is terrible at storing logfiles or database dumps!) The problem is that all of the full files often need to be reconstructed, which is very expensive.

* Does it include gigantic trees? Every time a file is modified, Git has to create a new copy of every tree (i.e., every directory in the path) leading to the file. Huge trees make this expensive. Moreover, it is very expensive to iterate through history that contains huge trees. It's best to avoid directories with more than a couple of thousand entries. If you must store many files, it is better to shard them into multiple, smaller directories if possible.

* Does it have the same (or very similar) files repeated over and over again at different paths in a single commit? If so, your repository might have a reasonable overall size, but when you check it out it balloons into an enormous working copy. (Taken to an extreme, this is called a "git bomb"; see below.) It also makes some Git operations, like `fsck`, very expensive. Perhaps you can achieve your goals more effectively by using tags and branches or a build-time configuration system.

* Does it include absurdly long path names? That's probably not going to work well with other tools. One or two hundred characters should be enough, even if you're writing Java.

* Are there other bizarre and questionable things in your repository?

    * Annotated tags pointing at one another in long chains?

    * Octopus merges with dozens of parents?

    * Commits with gigantic log messages?

`git-sizer` computes a bunch of statistics about your repository that can help reveal all of the problems described above.


## Getting started

1.  Build:

        script/bootstrap
        make

    The executable file is written to `bin/git-sizer`. If copy it to your `PATH` and you have Git installed, you can run the program by typing `git sizer`; otherwise, you need to type the full path and filename to run it; e.g., `bin/git-sizer`.

2.  Run:

        git sizer [<opt>...] [<path-to-git-repository>]

    To get a summary of the current repository, all you need is

        git sizer

    Use the `--json` option to get output in JSON format, which includes the raw numbers.

    Note that if a value overflows its counter (which should only happen for malicious repositories), the corresponding value is displayed as `∞` in tabular form, or truncated to 2³²-1 or 2⁶⁴-1 (depending on the size of the counter) in JSON mode.

    To get a list of other options, run

        git sizer --help


## Example output

Here is the output for [the Linux repository](https://github.com/torvalds/linux) as of this writing:

```
$ git-sizer
| Name                         | Value     | Level of concern               |
| ---------------------------- | --------- | ------------------------------ |
| Overall repository size      |           |                                |
| * Commits                    |           |                                |
|   * Count                    |   723 k   | *                              |
|   * Total size               |   525 MiB | **                             |
| * Trees                      |           |                                |
|   * Count                    |  3.40 M   | **                             |
|   * Total size               |  9.00 GiB | ****                           |
|   * Total tree entries       |   264 M   | *****                          |
| * Blobs                      |           |                                |
|   * Count                    |  1.65 M   | *                              |
|   * Total size               |  55.8 GiB | *****                          |
| * Annotated tags             |           |                                |
|   * Count                    |   534     |                                |
| * References                 |           |                                |
|   * Count                    |   539     |                                |
|                              |           |                                |
| Biggest commit objects       |           |                                |
| * Maximum size           [1] |  72.7 KiB | *                              |
| * Maximum parents        [2] |    66     | ******                         |
|                              |           |                                |
| Biggest tree objects         |           |                                |
| * Maximum tree entries   [3] |  1.68 k   |                                |
|                              |           |                                |
| Biggest blob objects         |           |                                |
| * Maximum size           [4] |  13.5 MiB | *                              |
|                              |           |                                |
| History structure            |           |                                |
| * Maximum history depth      |   136 k   |                                |
| * Maximum tag depth      [5] |     1     | *                              |
|                              |           |                                |
| Biggest checkouts            |           |                                |
| * Number of directories  [6] |  4.38 k   | **                             |
| * Maximum path depth     [7] |    14     | *                              |
| * Maximum path length    [8] |   134 B   | *                              |
| * Number of files        [9] |  62.3 k   | *                              |
| * Total size of files    [9] |   747 MiB |                                |
| * Number of symlinks    [10] |    40     |                                |
| * Number of submodules       |     0     |                                |

[1]  91cc53b0c78596a73fa708cceb7313e7168bb146 (91cc53b0c78596a73fa708cceb7313e7168bb146)
[2]  2cde51fbd0f310c8a2c5f977e665c0ac3945b46d (2cde51fbd0f310c8a2c5f977e665c0ac3945b46d)
[3]  4f86eed5893207aca2c2da86b35b38f2e1ec1fc8 (refs/heads/master:arch/arm/boot/dts)
[4]  a02b6794337286bc12c907c33d5d75537c240bd0 (refs/heads/master:drivers/gpu/drm/amd/include/asic_reg/vega10/NBIO/nbio_6_1_sh_mask.h)
[5]  5dc01c595e6c6ec9ccda4f6f69c131c0dd945f8c (refs/tags/v2.6.11)
[6]  1459754b9d9acc2ffac8525bed6691e15913c6e2 (589b754df3f37ca0a1f96fccde7f91c59266f38a^{tree})
[7]  78a269635e76ed927e17d7883f2d90313570fdbc (dae09011115133666e47c35673c0564b0a702db7^{tree})
[8]  ce5f2e31d3bdc1186041fdfd27a5ac96e728f2c5 (refs/heads/master^{tree})
[9]  532bdadc08402b7a72a4b45a2e02e5c710b7d626 (e9ef1fe312b533592e39cddc1327463c30b0ed8d^{tree})
[10] f29a5ea76884ac37e1197bef1941f62fda3f7b99 (f5308d1b83eba20e69df5e0926ba7257c8dd9074^{tree})
```

The section "Overall repository size" include repository-wide statistics about distinct objects, not including repetition. "Total size" is the sum of the sizes of the corresponding objects in their uncompressed form, measured in bytes.

The "Biggest objects" sections provide information about the biggest single objects of each type, anywhere in the repository.

In the "History structure" section, "maximum history depth" is the longest chain of commits in the history, and "maximum tag depth" reports the longest chain of annotated tags that point at other annotated tags.

The "Biggest checkouts" section is about the sizes of commits as checked out into a working copy. "Maximum path depth" is the largest number of path components within the repository, and "maximum path length" is the longest path in terms of bytes. "Total size of files" is the sum of all file sizes in a single commit, in bytes.

The asterisks indicate values that seem unusually high. The more asterisks, the more trouble this value is expected to cause. Exclamation points indicate values that are extremely high (i.e., equivalent to more than 30 asterisks).

The footnotes list the SHA-1s of the "biggest" objects referenced in the table, along with a more human-readable `<commit>:<path>` description of where that object is located in the repository's history.

The Linux repository is large by most standards, and as you can see, it is pushing some of Git's limits. And indeed, some Git operations on the Linux repository (e.g., `git fsck`, `git gc`) take a while. But due to its sane structure, none of its dimensions are wildly out of proportion to the size of the code base, so it can be managed successfully using Git.

Here is the output for one of the famous ["git bomb"](https://kate.io/blog/git-bomb/) repositories:

```
$ git-sizer test/data/git-bomb.git
| Name                         | Value     | Level of concern               |
| ---------------------------- | --------- | ------------------------------ |
| Overall repository size      |           |                                |
| * Commits                    |           |                                |
|   * Count                    |     3     |                                |
|   * Total size               |   606 B   |                                |
| * Trees                      |           |                                |
|   * Count                    |    12     |                                |
|   * Total size               |  3.48 KiB |                                |
|   * Total tree entries       |   122     |                                |
| * Blobs                      |           |                                |
|   * Count                    |     3     |                                |
|   * Total size               |  3.65 KiB |                                |
| * Annotated tags             |           |                                |
|   * Count                    |     0     |                                |
| * References                 |           |                                |
|   * Count                    |     1     |                                |
|                              |           |                                |
| Biggest commit objects       |           |                                |
| * Maximum size           [1] |   218 B   |                                |
| * Maximum parents        [1] |     1     |                                |
|                              |           |                                |
| Biggest tree objects         |           |                                |
| * Maximum tree entries   [2] |    11     |                                |
|                              |           |                                |
| Biggest blob objects         |           |                                |
| * Maximum size           [3] |  1.82 KiB |                                |
|                              |           |                                |
| History structure            |           |                                |
| * Maximum history depth      |     3     |                                |
| * Maximum tag depth          |     0     |                                |
|                              |           |                                |
| Biggest checkouts            |           |                                |
| * Number of directories  [2] |  1.11 G   | !!!!!!!!!!!!!!!!!!!!!!!!!!!!!! |
| * Maximum path depth     [2] |    11     | *                              |
| * Maximum path length    [2] |    29 B   |                                |
| * Number of files        [2] |     ∞     | !!!!!!!!!!!!!!!!!!!!!!!!!!!!!! |
| * Total size of files    [4] |  83.8 GiB | !!!!!!!!!!!!!!!!!!!!!!!!!!!!!! |
| * Number of symlinks         |     0     |                                |
| * Number of submodules       |     0     |                                |

[1]  7af99c9e7d4768fa681f4fe4ff61259794cf719b (refs/heads/master)
[2]  c1971b07ce6888558e2178a121804774c4201b17 (refs/heads/master^{tree})
[3]  dacaac6d3b2cf39ec8078dfb0bd3ce691e92557f (18ed56cbc5012117e24a603e7c072cf65d36d469:README.md)
[4]  d9513477b01825130c48c4bebed114c4b2d50401 (18ed56cbc5012117e24a603e7c072cf65d36d469^{tree})
```

This repository is mischievously constructed to have a pathological tree structure, with the same directories repeated over and over again. As a result, even though the entire repository is less than 20 kb in size, when checked out it would explode into over a billion directories containing over ten billion files. (`git-sizer` prints `∞` for the blob count because the true number has overflowed the 32-bit counter used for that field.)
