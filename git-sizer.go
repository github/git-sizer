package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/pprof"
	"strconv"
	"time"

	"github.com/spf13/pflag"

	"github.com/github/git-sizer/git"
	"github.com/github/git-sizer/internal/refopts"
	"github.com/github/git-sizer/isatty"
	"github.com/github/git-sizer/meter"
	"github.com/github/git-sizer/sizes"
)

const usage = `usage: git-sizer [OPTS] [ROOT...]

 Scan objects in your Git repository and emit statistics about them.

      --threshold THRESHOLD    minimum level of concern (i.e., number of stars)
                               that should be reported. Default:
                               '--threshold=1'. Can be set via gitconfig:
                               'sizer.threshold'.
  -v, --verbose                report all statistics, whether concerning or
                               not; equivalent to '--threshold=0
      --no-verbose             equivalent to '--threshold=1'
      --critical               only report critical statistics; equivalent
                               to '--threshold=30'
      --names=[none|hash|full] display names of large objects in the specified
                               style. Values:
                               * 'none' - omit footnotes entirely
                               * 'hash' - show only the SHA-1s of objects
                               * 'full' - show full names
                               Default is '--names=full'. Can be set via
                               gitconfig: 'sizer.names'.
  -j, --json                   output results in JSON format
      --json-version=[1|2]     choose which JSON format version to output.
                               Default: --json-version=1. Can be set via
                               gitconfig: 'sizer.jsonVersion'.
      --[no-]progress          report (don't report) progress to stderr. Can
                               be set via gitconfig: 'sizer.progress'.
      --version                only report the git-sizer version number

 Object selection:

 git-sizer traverses through your Git history to find objects to
 process. By default, it processes all objects that are reachable from
 any reference. You can tell it to process only some of your
 references; see "Reference selection" below.

 If explicit ROOTs are specified on the command line, each one should
 be a string that 'git rev-parse' can convert into a single Git object
 ID, like 'main', 'main~:src', or an abbreviated SHA-1. See
 git-rev-parse(1) for details. In that case, git-sizer also treats
 those objects as starting points for its traversal, and also includes
 the Git objects that are reachable from those roots in the analysis.

 As a special case, if one or more ROOTs are specified on the command
 line but _no_ reference selection options, then _only_ the specified
 ROOTs are traversed, and no references.

 Reference selection:

 The following options can be used to limit which references to
 process. The last rule matching a reference determines whether that
 reference is processed.

      --[no-]branches          process [don't process] branches
      --[no-]tags              process [don't process] tags
      --[no-]remotes           process [don't process] remote-tracking
                               references
      --[no-]notes             process [don't process] git-notes references
      --[no-]stash             process [don't process] refs/stash
      --include PREFIX, --exclude PREFIX
                               process [don't process] references with the
                               specified PREFIX (e.g.,
                               '--include=refs/remotes/origin')
      --include /REGEXP/, --exclude /REGEXP/
                               process [don't process] references matching the
                               specified regular expression (e.g.,
                               '--include=refs/tags/release-.*')
      --include @REFGROUP, --exclude @REFGROUP
                               process [don't process] references in the
                               specified reference group (see below)
      --show-refs              show which refs are being included/excluded

 PREFIX must match at a boundary; for example 'refs/foo' matches
 'refs/foo' and 'refs/foo/bar' but not 'refs/foobar'.

 REGEXP patterns must match the full reference name.

 REFGROUP can be the name of a predefined reference group ('branches',
 'tags', 'remotes', 'pulls', 'changes', 'notes', or 'stash'), or one
 defined via gitconfig settings like the following (the
 include/exclude settings can be repeated):

   * 'refgroup.REFGROUP.name=NAME'
   * 'refgroup.REFGROUP.include=PREFIX'
   * 'refgroup.REFGROUP.includeRegexp=REGEXP'
   * 'refgroup.REFGROUP.exclude=PREFIX'
   * 'refgroup.REFGROUP.excludeRegexp=REGEXP'

`

var ReleaseVersion string
var BuildVersion string

func main() {
	ctx := context.Background()

	err := mainImplementation(ctx, os.Stdout, os.Stderr, os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
}

func mainImplementation(ctx context.Context, stdout, stderr io.Writer, args []string) error {
	var nameStyle sizes.NameStyle = sizes.NameStyleFull
	var cpuprofile string
	var jsonOutput bool
	var jsonVersion int
	var threshold sizes.Threshold = 1
	var progress bool
	var version bool
	var showRefs bool

	// Try to open the repository, but it's not an error yet if this
	// fails, because the user might only be asking for `--help`.
	repo, repoErr := git.NewRepositoryFromPath(".")

	flags := pflag.NewFlagSet("git-sizer", pflag.ContinueOnError)
	flags.Usage = func() {
		fmt.Fprint(stdout, usage)
	}

	flags.VarP(
		sizes.NewThresholdFlagValue(&threshold, 0),
		"verbose", "v", "report all statistics, whether concerning or not",
	)
	flags.Lookup("verbose").NoOptDefVal = "true"

	flags.Var(
		sizes.NewThresholdFlagValue(&threshold, 1),
		"no-verbose", "report statistics that are at all concerning",
	)
	flags.Lookup("no-verbose").NoOptDefVal = "true"

	flags.Var(
		&threshold, "threshold",
		"minimum level of concern (i.e., number of stars) that should be\n"+
			"                              reported",
	)

	flags.Var(
		sizes.NewThresholdFlagValue(&threshold, 30),
		"critical", "only report critical statistics",
	)
	flags.Lookup("critical").NoOptDefVal = "true"

	flags.Var(
		&nameStyle, "names",
		"display names of large objects in the specified `style`:\n"+
			"        --names=none            omit footnotes entirely\n"+
			"        --names=hash            show only the SHA-1s of objects\n"+
			"        --names=full            show full names",
	)

	flags.BoolVarP(&jsonOutput, "json", "j", false, "output results in JSON format")
	flags.IntVar(&jsonVersion, "json-version", 1, "JSON format version to output (1 or 2)")

	defaultProgress := false
	if f, ok := stderr.(*os.File); ok {
		atty, err := isatty.Isatty(f.Fd())
		if err == nil && atty {
			defaultProgress = true
		}
	}

	flags.BoolVar(&progress, "progress", defaultProgress, "report progress to stderr")
	flags.BoolVar(&version, "version", false, "report the git-sizer version number")
	flags.Var(&NegatedBoolValue{&progress}, "no-progress", "suppress progress output")
	flags.Lookup("no-progress").NoOptDefVal = "true"

	flags.StringVar(&cpuprofile, "cpuprofile", "", "write cpu profile to file")
	if err := flags.MarkHidden("cpuprofile"); err != nil {
		return fmt.Errorf("marking option hidden: %w", err)
	}

	var configger refopts.Configger
	if repo != nil {
		configger = repo
	}

	rgb, err := refopts.NewRefGroupBuilder(configger)
	if err != nil {
		return err
	}

	rgb.AddRefopts(flags)

	flags.BoolVar(&showRefs, "show-refs", false, "list the references being processed")

	flags.SortFlags = false

	err = flags.Parse(args)
	if err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			return nil
		}
		return err
	}

	if cpuprofile != "" {
		f, err := os.Create(cpuprofile)
		if err != nil {
			return fmt.Errorf("couldn't set up cpuprofile file: %w", err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			return fmt.Errorf("starting CPU profiling: %w", err)
		}
		defer pprof.StopCPUProfile()
	}

	if version {
		if ReleaseVersion != "" {
			fmt.Fprintf(stdout, "git-sizer release %s\n", ReleaseVersion)
		} else {
			fmt.Fprintf(stdout, "git-sizer build %s\n", BuildVersion)
		}
		return nil
	}

	if repoErr != nil {
		return fmt.Errorf("couldn't open Git repository: %w", repoErr)
	}

	if jsonOutput {
		if !flags.Changed("json-version") {
			v, err := repo.ConfigIntDefault("sizer.jsonVersion", jsonVersion)
			if err != nil {
				return err
			}
			jsonVersion = v
			if !(jsonVersion == 1 || jsonVersion == 2) {
				return fmt.Errorf("JSON version (read from gitconfig) must be 1 or 2")
			}
		} else if !(jsonVersion == 1 || jsonVersion == 2) {
			return fmt.Errorf("JSON version must be 1 or 2")
		}
	}

	if !flags.Changed("threshold") &&
		!flags.Changed("verbose") &&
		!flags.Changed("no-verbose") &&
		!flags.Changed("critical") {
		s, err := repo.ConfigStringDefault("sizer.threshold", fmt.Sprintf("%g", threshold))
		if err != nil {
			return err
		}
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return fmt.Errorf("parsing gitconfig value for 'sizer.threshold': %w", err)
		}
		threshold = sizes.Threshold(v)
	}

	if !flags.Changed("names") {
		s, err := repo.ConfigStringDefault("sizer.names", "full")
		if err != nil {
			return err
		}
		err = nameStyle.Set(s)
		if err != nil {
			return fmt.Errorf("parsing gitconfig value for 'sizer.names': %w", err)
		}
	}

	if !flags.Changed("progress") && !flags.Changed("no-progress") {
		v, err := repo.ConfigBoolDefault("sizer.progress", progress)
		if err != nil {
			return fmt.Errorf("parsing gitconfig value for 'sizer.progress': %w", err)
		}
		progress = v
	}

	rg, err := rgb.Finish(len(flags.Args()) == 0)
	if err != nil {
		return err
	}

	if showRefs {
		fmt.Fprintf(stderr, "References (included references marked with '+'):\n")
		rg = refopts.NewShowRefGrouper(rg, stderr)
	}

	var progressMeter meter.Progress = meter.NoProgressMeter
	if progress {
		progressMeter = meter.NewProgressMeter(stderr, 100*time.Millisecond)
	}

	refRoots, err := sizes.CollectReferences(ctx, repo, rg)
	if err != nil {
		return fmt.Errorf("determining which reference to scan: %w", err)
	}

	roots := make([]sizes.Root, 0, len(refRoots)+len(flags.Args()))
	for _, refRoot := range refRoots {
		roots = append(roots, refRoot)
	}

	for _, arg := range flags.Args() {
		oid, err := repo.ResolveObject(arg)
		if err != nil {
			return fmt.Errorf("resolving command-line argument %q: %w", arg, err)
		}
		roots = append(roots, sizes.NewExplicitRoot(arg, oid))
	}

	historySize, err := sizes.ScanRepositoryUsingGraph(
		ctx, repo, roots, nameStyle, progressMeter,
	)
	if err != nil {
		return fmt.Errorf("error scanning repository: %w", err)
	}

	if jsonOutput {
		var j []byte
		var err error
		switch jsonVersion {
		case 1:
			j, err = json.MarshalIndent(historySize, "", "    ")
		case 2:
			j, err = historySize.JSON(rg.Groups(), threshold, nameStyle)
		default:
			return fmt.Errorf("JSON version must be 1 or 2")
		}
		if err != nil {
			return fmt.Errorf("could not convert %v to json: %w", historySize, err)
		}
		fmt.Fprintf(stdout, "%s\n", j)
	} else {
		if _, err := io.WriteString(
			stdout, historySize.TableString(rg.Groups(), threshold, nameStyle),
		); err != nil {
			return fmt.Errorf("writing output: %w", err)
		}
	}

	return nil
}
