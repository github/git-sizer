_Happy Git repositories are all alike; every unhappy Git repository is unhappy in its own way._ —Linus Tolstoy

# git-sizer

Is your Git repository bursting at the seams?

`git-sizer` computes various size metrics for a local Git repository, flagging those that might cause you problems or inconvenience. For example:

* Is the repository too big overall? Ideally, Git repositories should be under 1 GiB, and (without special handling) they start to get unwieldy over 5 GiB. Big repositories take a long time to clone and repack, and take a lot of disk space. Suggestions:

    * Avoid storing generated files (e.g., compiler output, JAR files) in Git. It would be better to regenerate them when necessary, or store them in a package registry or even a fileserver.

    * Avoid storing large media assets in Git. You might want to look into [Git-LFS](https://git-lfs.github.com/) or [git-annex](http://git-annex.branchable.com/), which allow you to version your media assets in Git while actually storing them outside of your repository.

    * Avoid storing file archives (e.g., ZIP files, tarballs) in Git, especially if compressed. Different versions of such files don't delta well against each other, so Git can't store them efficiently. It would be better to store the individual files in your repository, or store the archive elsewhere.

* Does the repository have too many references (branches and/or tags)? They all have to be transferred to the client for every fetch, even if your clone is up-to-date. Try to limit them to a few tens of thousands at most. Suggestions:

    * Delete unneeded tags and branches.

    * Avoid pushing your "remote-tracking" branches to a shared repository.

    * Consider using ["git notes"](https://git-scm.com/docs/git-notes) rather than tags to attach auxiliary information to commits (for example, CI build results).

    * Perhaps store some of your rarely-needed tags and branches in a separate fork of your repository that is not fetched from by normal developers.

* Does the repository include too many objects? The more objects, the longer it takes for Git to traverse the repository's history, for example when garbage-collecting. Suggestions:

    * Think about whether you are storing very many tiny files that could easily be collected into a few bigger files.

    * Consider breaking your project up into multiple subprojects.

* Does the repository include gigantic blobs (files)? Git works best with small- to medium-sized files. It's OK to have a few files in the megabyte range, but they should generally be the exception. Suggestions:

    * Consider using [Git-LFS](https://git-lfs.github.com/) for storing your large files, especially those (e.g., media assets) that don't diff and merge usefully.

    * See also the section "Is the repository too big overall?"

* Does the repository include many, many versions of large text files, each one slightly changed from the one before? Such files delta very well, so they might not cause your repository to grow alarmingly. But it is expensive for Git to reconstruct the full files and to diff them, which it needs to do internally for many operations. Suggestions:

    * Avoid storing log files and database dumps in Git.

    * Avoid storing giant data files (e.g., enormous XML files) in Git, especially if they are modified frequently. Consider using a database instead.

* Does the repository include gigantic trees (directories)? Every time a file is modified, Git has to create a new copy of every tree (i.e., every directory in the path) leading to the file. Huge trees make this expensive. Moreover, it is very expensive to traverse through history that contains huge trees, for example for `git blame`. Suggestions:

    * Avoid creating directories with more than a couple of thousand entries each.

    * If you must store very many files, it is better to shard them into a hierarchy of multiple, smaller directories.

* Does the repository have the same (or very similar) files repeated over and over again at different paths in a single commit? If so, the repository might have a reasonable overall size, but when you check it out it balloons into an enormous working copy. (Taken to an extreme, this is called a "git bomb"; see below.) Suggestions:

    * Perhaps you can achieve your goals more effectively by using tags and branches or a build-time configuration system.

* Does the repository include absurdly long path names? That's probably not going to work well with other tools. One or two hundred characters should be enough, even if you're writing Java.

* Are there other bizarre and questionable things in the repository?

    * Annotated tags pointing at one another in long chains?

    * Octopus merges with dozens of parents?

    * Commits with gigantic log messages?

`git-sizer` computes many size-related statistics about your repository that can help reveal all of the problems described above. These practices are not wrong per se, but the more that you stretch Git beyond its sweet spot, the less you will be able to enjoy Git's legendary speed and performance. Especially if your Git repository statistics seem out of proportion to your project size, you might be able to make your life easier by adjusting how you use Git.


## Getting started

1.  Make sure that you have the [Git command-line client](https://git-scm.com/) installed, **version >= 2.6**. NOTE: `git-sizer` invokes `git` commands to examine the contents of your repository, so **it is required that the `git` command be in your `PATH`** when you run `git-sizer`.

2.  Install `git-sizer`. Either:

    a. Install a released version of `git-sizer`(recommended):
       1. Go to [the releases page](https://github.com/github/git-sizer/releases) and download the ZIP file corresponding to your platform.
       2. Unzip the file.
       3. Move the executable file (`git-sizer` or `git-sizer.exe`) into your `PATH`.

    b. Build and install from source. See the instructions in [`docs/BUILDING.md`](docs/BUILDING.md).

3.  Change to the directory containing a full, non-shallow clone of the Git repository that you'd like to analyze. Then run

        git-sizer [<option>...]

    No options are required. You can learn about available options by typing `git-sizer -h` or by reading on.

**Pro tip**: If you add `git-sizer` to your `PATH`, then you can run it by typing either `git-sizer` or `git sizer`. In the latter case, it is found and run for you by Git, and you can add extra Git options between the two words, like `git -C /path/to/my/repo sizer`. If you don't add `git-sizer` to your `PATH`, then of course you need to type its full path and filename to run it; e.g., `/path/to/bin/git-sizer`. In either case, the `git` executable *must* be in your `PATH`.


## Usage

By default, `git-sizer` outputs its results in tabular format. For example, let's use it to analyze [the Linux repository](https://github.com/torvalds/linux), using the `--verbose` option so that all statistics are output:

```
$ git-sizer --verbose
Processing blobs: 1652370
Processing trees: 3396199
Processing commits: 722647
Matching commits to trees: 722647
Processing annotated tags: 534
Processing references: 539
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
| Biggest objects              |           |                                |
| * Commits                    |           |                                |
|   * Maximum size         [1] |  72.7 KiB | *                              |
|   * Maximum parents      [2] |    66     | ******                         |
| * Trees                      |           |                                |
|   * Maximum entries      [3] |  1.68 k   | *                              |
| * Blobs                      |           |                                |
|   * Maximum size         [4] |  13.5 MiB | *                              |
|                              |           |                                |
| History structure            |           |                                |
| * Maximum history depth      |   136 k   |                                |
| * Maximum tag depth      [5] |     1     |                                |
|                              |           |                                |
| Biggest checkouts            |           |                                |
| * Number of directories  [6] |  4.38 k   | **                             |
| * Maximum path depth     [7] |    13     | *                              |
| * Maximum path length    [8] |   134 B   | *                              |
| * Number of files        [9] |  62.3 k   | *                              |
| * Total size of files    [9] |   747 MiB |                                |
| * Number of symlinks    [10] |    40     |                                |
| * Number of submodules       |     0     |                                |

[1]  91cc53b0c78596a73fa708cceb7313e7168bb146
[2]  2cde51fbd0f310c8a2c5f977e665c0ac3945b46d
[3]  4f86eed5893207aca2c2da86b35b38f2e1ec1fc8 (refs/heads/master:arch/arm/boot/dts)
[4]  a02b6794337286bc12c907c33d5d75537c240bd0 (refs/heads/master:drivers/gpu/drm/amd/include/asic_reg/vega10/NBIO/nbio_6_1_sh_mask.h)
[5]  5dc01c595e6c6ec9ccda4f6f69c131c0dd945f8c (refs/tags/v2.6.11)
[6]  1459754b9d9acc2ffac8525bed6691e15913c6e2 (589b754df3f37ca0a1f96fccde7f91c59266f38a^{tree})
[7]  78a269635e76ed927e17d7883f2d90313570fdbc (dae09011115133666e47c35673c0564b0a702db7^{tree})
[8]  ce5f2e31d3bdc1186041fdfd27a5ac96e728f2c5 (refs/heads/master^{tree})
[9]  532bdadc08402b7a72a4b45a2e02e5c710b7d626 (e9ef1fe312b533592e39cddc1327463c30b0ed8d^{tree})
[10] f29a5ea76884ac37e1197bef1941f62fda3f7b99 (f5308d1b83eba20e69df5e0926ba7257c8dd9074^{tree})
```

The output is a table showing the thing that was measured, its numerical value, and a rough indication of which values might be a cause for concern. In all cases, only objects that are reachable from references are included (i.e., not unreachable objects, nor objects that are reachable only from the reflogs).

The "Overall repository size" section includes repository-wide statistics about distinct objects, not including repetition. "Total size" is the sum of the sizes of the corresponding objects in their uncompressed form, measured in bytes. The overall uncompressed size of all objects is a good indication of how expensive commands like `git gc --aggressive` (and `git repack [-f|-F]` and `git pack-objects --no-reuse-delta`), `git fsck`, and `git log [-G|-S]` will be.  The uncompressed size of trees and commits is a good indication of how expensive reachability traversals will be, including clones and fetches and `git gc`.

The "Biggest objects" section provides information about the biggest single objects of each type, anywhere in the history.

In the "History structure" section, "maximum history depth" is the longest chain of commits in the history, and "maximum tag depth" reports the longest chain of annotated tags that point at other annotated tags.

The "Biggest checkouts" section is about the sizes of commits as checked out into a working copy. "Maximum path depth" is the largest number of path components for files in the working copy, and "maximum path length" is the longest path in terms of bytes. "Total size of files" is the sum of all file sizes in the single biggest commit, including multiplicities if the same file appears multiple times.

The "Value" column displays counts, using units "k" (thousand), "M" (million), "G" (billion) etc., and sizes, using units "B" (bytes), "KiB" (1024 bytes), "MiB" (1024 KiB), etc. Note that if a value overflows its counter (which should only happen for malicious repositories), the corresponding value is displayed as `∞` in tabular form, or truncated to 2³²-1 or 2⁶⁴-1 (depending on the size of the counter) in JSON mode.

The "Level of concern" column uses asterisks to indicate values that seem high compared with "typical" Git repositories. The more asterisks, the more inconvenience this aspect of your repository might be expected to cause. Exclamation points indicate values that are extremely high (i.e., equivalent to more than 30 asterisks).

The footnotes list the SHA-1s of the "biggest" objects referenced in the table, along with a more human-readable `<commit>:<path>` description of where that object is located in the repository's history. Given the name of a large object, you could, for example, type

    git cat-file -p <commit>:<path>

at the command line to view the contents of the object. (Use `--names=none` if you'd rather omit these footnotes.)

By default, only statistics above a minimal level of concern are reported. Use `--verbose` (as above) to request that all statistics be output. Use `--threshold=<value>` to suppress the reporting of statistics below a specified level of concern. (`<value>` is interpreted as a numerical value corresponding to the number of asterisks.) Use `--critical` to report only statistics with a critical level of concern (equivalent to `--threshold=30`).

If you'd like the output in machine-readable format, including exact numbers, use the `--json` option. You can use `--json-version=1` or `--json-version=2` to choose between old and new style JSON output.

To get a list of other options, run

    git-sizer -h

The Linux repository is large by most standards. As you can see, it is pushing some of Git's limits. And indeed, some Git operations on the Linux repository (e.g., `git fsck`, `git gc`) do take a while. But due to its sane structure, none of its dimensions are wildly out of proportion to the size of the code base, so the kernel project is managed successfully using Git.

Here is the non-verbose  output for one of the famous ["git bomb"](https://kate.io/blog/git-bomb/) repositories:

```
$ git-sizer
[...]
| Name                         | Value     | Level of concern               |
| ---------------------------- | --------- | ------------------------------ |
| Biggest checkouts            |           |                                |
| * Number of directories  [1] |  1.11 G   | !!!!!!!!!!!!!!!!!!!!!!!!!!!!!! |
| * Maximum path depth     [1] |    11     | *                              |
| * Number of files        [1] |     ∞     | !!!!!!!!!!!!!!!!!!!!!!!!!!!!!! |
| * Total size of files    [2] |  83.8 GiB | !!!!!!!!!!!!!!!!!!!!!!!!!!!!!! |

[1]  c1971b07ce6888558e2178a121804774c4201b17 (refs/heads/master^{tree})
[2]  d9513477b01825130c48c4bebed114c4b2d50401 (18ed56cbc5012117e24a603e7c072cf65d36d469^{tree})
```

This repository is mischievously constructed to have a pathological tree structure, with the same directories repeated over and over again. As a result, even though the entire repository is less than 20 kb in size, when checked out it would explode into over a billion directories containing over ten billion files. (`git-sizer` prints `∞` for the blob count because the true number has overflowed the 32-bit counter used for that field.)


## Contributing

`git-sizer` is in regular use and is still under active development. If you would like to help out, please see [`CONTRIBUTING.md`](CONTRIBUTING.md).
