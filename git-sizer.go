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

var ReleaseVersion string
var BuildVersion string

type NegatedBoolValue struct {
	value *bool
}

func (b *NegatedBoolValue) Set(s string) error {
	v, err := strconv.ParseBool(s)
	*b.value = !v
	return err
}

func (b *NegatedBoolValue) Get() interface{} {
	return !*b.value
}

func (b *NegatedBoolValue) String() string {
	if b == nil || b.value == nil {
		return "true"
	} else {
		return strconv.FormatBool(!*b.value)
	}
}

func (v *NegatedBoolValue) Type() string {
	return "bool"
}

func main() {
	err := mainImplementation()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
}

func mainImplementation() error {
	var processBranches bool
	var processTags bool
	var processRemotes bool
	var nameStyle sizes.NameStyle = sizes.NameStyleFull
	var cpuprofile string
	var jsonOutput bool
	var threshold sizes.Threshold = 1
	var progress bool
	var version bool

	pflag.BoolVar(&processBranches, "branches", false, "process all branches")
	pflag.BoolVar(&processTags, "tags", false, "process all tags")
	pflag.BoolVar(&processRemotes, "remotes", false, "process all remote-tracking branches")

	pflag.VarP(
		sizes.NewThresholdFlagValue(&threshold, 0),
		"verbose", "v", "report all statistics, whether concerning or not",
	)
	pflag.Lookup("verbose").NoOptDefVal = "true"

	pflag.Var(
		&threshold, "threshold",
		"minimum level of concern (i.e., number of stars) that should be\n"+
			"                              reported",
	)

	pflag.Var(
		sizes.NewThresholdFlagValue(&threshold, 30),
		"critical", "only report critical statistics",
	)
	pflag.Lookup("critical").NoOptDefVal = "true"

	pflag.Var(
		&nameStyle, "names",
		"display names of large objects in the specified `style`:\n"+
			"        --names=none            omit footnotes entirely\n"+
			"        --names=hash            show only the SHA-1s of objects\n"+
			"        --names=full            show full names",
	)

	pflag.BoolVarP(&jsonOutput, "json", "j", false, "output results in JSON format")

	atty, err := isatty.Isatty(os.Stderr.Fd())
	if err != nil {
		atty = false
	}
	pflag.BoolVar(&progress, "progress", atty, "report progress to stderr")
	pflag.BoolVar(&version, "version", false, "report the git-sizer version number")
	pflag.Var(&NegatedBoolValue{&progress}, "no-progress", "suppress progress output")
	pflag.Lookup("no-progress").NoOptDefVal = "true"

	pflag.StringVar(&cpuprofile, "cpuprofile", "", "write cpu profile to file")
	pflag.CommandLine.MarkHidden("cpuprofile")

	pflag.CommandLine.SortFlags = false

	pflag.Parse()

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

	args := pflag.Args()

	if len(args) != 0 {
		return errors.New("excess arguments")
	}

	repo, err := git.NewRepository(".")
	if err != nil {
		return fmt.Errorf("couldn't open Git repository: %s", err)
	}
	defer repo.Close()

	var historySize sizes.HistorySize

	var filter git.ReferenceFilter
	if processBranches || processTags || processRemotes {
		var filters []git.ReferenceFilter
		if processBranches {
			filters = append(filters, git.BranchesFilter)
		}
		if processTags {
			filters = append(filters, git.TagsFilter)
		}
		if processRemotes {
			filters = append(filters, git.RemotesFilter)
		}
		filter = git.OrFilter(filters...)
	} else {
		filter = git.AllReferencesFilter
	}

	historySize, err = sizes.ScanRepositoryUsingGraph(repo, filter, nameStyle, progress)
	if err != nil {
		return fmt.Errorf("error scanning repository: %s", err)
	}

	if jsonOutput {
		s, err := json.MarshalIndent(historySize, "", "    ")
		if err != nil {
			return fmt.Errorf("could not convert %v to json: %s", historySize, err)
		}
		fmt.Printf("%s\n", s)
	} else {
		io.WriteString(os.Stdout, historySize.TableString(threshold, nameStyle))
	}

	return nil
}
