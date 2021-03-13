package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/pprof"
	"strconv"

	"github.com/github/git-sizer/git"
	"github.com/github/git-sizer/isatty"
	"github.com/github/git-sizer/sizes"

	"github.com/spf13/pflag"
)

const Usage = `usage: git-sizer [OPTS]

  -v, --verbose                report all statistics, whether concerning or not
      --threshold threshold    minimum level of concern (i.e., number of stars)
                               that should be reported. Default:
                               '--threshold=1'.
      --critical               only report critical statistics
      --names=[none|hash|full] display names of large objects in the specified
                               style: 'none' (omit footnotes entirely), 'hash'
                               (show only the SHA-1s of objects), or 'full'
                               (show full names). Default is '--names=full'.
  -j, --json                   output results in JSON format
      --json-version=[1|2]     choose which JSON format version to output.
                               Default: --json-version=1.
      --[no-]progress          report (don't report) progress to stderr.
      --version                only report the git-sizer version number

 Reference selection:

 By default, git-sizer processes all Git objects that are reachable from any
 reference. The following options can be used to limit which references to
 include. The last rule matching a reference determines whether that reference
 is processed:

      --branches               process branches
      --tags                   process tags
      --remotes                process remote refs
      --include prefix         process references with the specified prefix
                               (e.g., '--include=refs/remotes/origin')
      --exclude prefix         don't process references with the specified
                               prefix (e.g., '--exclude=refs/notes')

`

var ReleaseVersion string
var BuildVersion string

type NegatedBoolValue struct {
	value *bool
}

func (v *NegatedBoolValue) Set(s string) error {
	b, err := strconv.ParseBool(s)
	*v.value = !b
	return err
}

func (v *NegatedBoolValue) Get() interface{} {
	return !*v.value
}

func (v *NegatedBoolValue) String() string {
	if v == nil || v.value == nil {
		return "true"
	} else {
		return strconv.FormatBool(!*v.value)
	}
}

func (v *NegatedBoolValue) Type() string {
	return "bool"
}

type filterValue struct {
	filter   *git.IncludeExcludeFilter
	polarity git.Polarity
	prefix   string
}

func (v *filterValue) Set(s string) error {
	var prefix string
	var polarity git.Polarity

	if v.prefix == "" {
		prefix = s
		polarity = v.polarity
	} else {
		prefix = v.prefix
		// Allow a boolean value to alter the polarity:
		b, err := strconv.ParseBool(s)
		if err != nil {
			return err
		}
		if b {
			polarity = git.Include
		} else {
			polarity = git.Exclude
		}
	}

	switch polarity {
	case git.Include:
		v.filter.Include(git.PrefixFilter(prefix))
	case git.Exclude:
		v.filter.Exclude(git.PrefixFilter(prefix))
	}

	return nil
}

func (v *filterValue) Get() interface{} {
	return nil
}

func (v *filterValue) String() string {
	return ""
}

func (v *filterValue) Type() string {
	if v.prefix == "" {
		return "prefix"
	} else {
		return ""
	}
}

func main() {
	err := mainImplementation()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
}

func mainImplementation() error {
	var nameStyle sizes.NameStyle = sizes.NameStyleFull
	var cpuprofile string
	var jsonOutput bool
	var jsonVersion uint
	var threshold sizes.Threshold = 1
	var progress bool
	var version bool
	var filter git.IncludeExcludeFilter

	flags := pflag.NewFlagSet("git-sizer", pflag.ContinueOnError)
	flags.Usage = func() {
		fmt.Print(Usage)
	}

	flags.Var(&filterValue{&filter, git.Include, ""}, "include", "include specified references")
	flags.Var(&filterValue{&filter, git.Exclude, ""}, "exclude", "exclude specified references")

	flag := flags.VarPF(
		&filterValue{&filter, git.Include, "refs/heads/"}, "branches", "",
		"process all branches",
	)
	flag.NoOptDefVal = "true"

	flag = flags.VarPF(
		&filterValue{&filter, git.Include, "refs/tags/"}, "tags", "",
		"process all tags",
	)
	flag.NoOptDefVal = "true"

	flag = flags.VarPF(
		&filterValue{&filter, git.Include, "refs/remotes/"}, "remotes", "",
		"process all remotes",
	)
	flag.NoOptDefVal = "true"

	flags.VarP(
		sizes.NewThresholdFlagValue(&threshold, 0),
		"verbose", "v", "report all statistics, whether concerning or not",
	)
	flags.Lookup("verbose").NoOptDefVal = "true"

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
	flags.UintVar(&jsonVersion, "json-version", 1, "JSON format version to output (1 or 2)")

	atty, err := isatty.Isatty(os.Stderr.Fd())
	if err != nil {
		atty = false
	}
	flags.BoolVar(&progress, "progress", atty, "report progress to stderr")
	flags.BoolVar(&version, "version", false, "report the git-sizer version number")
	flags.Var(&NegatedBoolValue{&progress}, "no-progress", "suppress progress output")
	flags.Lookup("no-progress").NoOptDefVal = "true"

	flags.StringVar(&cpuprofile, "cpuprofile", "", "write cpu profile to file")
	flags.MarkHidden("cpuprofile")

	flags.SortFlags = false

	err = flags.Parse(os.Args[1:])
	if err != nil {
		if err == pflag.ErrHelp {
			return nil
		}
		return err
	}

	if jsonOutput && !(jsonVersion == 1 || jsonVersion == 2) {
		return fmt.Errorf("JSON version must be 1 or 2")
	}

	if cpuprofile != "" {
		f, err := os.Create(cpuprofile)
		if err != nil {
			return fmt.Errorf("couldn't set up cpuprofile file: %s", err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	if version {
		if ReleaseVersion != "" {
			fmt.Printf("git-sizer release %s\n", ReleaseVersion)
		} else {
			fmt.Printf("git-sizer build %s\n", BuildVersion)
		}
		return nil
	}

	args := flags.Args()

	if len(args) != 0 {
		return errors.New("excess arguments")
	}

	repo, err := git.NewRepository(".")
	if err != nil {
		return fmt.Errorf("couldn't open Git repository: %s", err)
	}
	defer repo.Close()

	var historySize sizes.HistorySize

	historySize, err = sizes.ScanRepositoryUsingGraph(repo, filter.Filter, nameStyle, progress)
	if err != nil {
		return fmt.Errorf("error scanning repository: %s", err)
	}

	if jsonOutput {
		var j []byte
		var err error
		switch jsonVersion {
		case 1:
			j, err = json.MarshalIndent(historySize, "", "    ")
		case 2:
			j, err = historySize.JSON(threshold, nameStyle)
		default:
			return fmt.Errorf("JSON version must be 1 or 2")
		}
		if err != nil {
			return fmt.Errorf("could not convert %v to json: %s", historySize, err)
		}
		fmt.Printf("%s\n", j)
	} else {
		io.WriteString(os.Stdout, historySize.TableString(threshold, nameStyle))
	}

	return nil
}
