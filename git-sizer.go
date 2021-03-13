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
	var jsonVersion uint
	var threshold sizes.Threshold = 1
	var progress bool
	var version bool

	flags := pflag.NewFlagSet("", pflag.ContinueOnError)

	flags.BoolVar(&processBranches, "branches", false, "process all branches")
	flags.BoolVar(&processTags, "tags", false, "process all tags")
	flags.BoolVar(&processRemotes, "remotes", false, "process all remote-tracking branches")

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
