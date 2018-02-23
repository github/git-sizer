package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/pprof"
	"strconv"

	"github.com/github/git-sizer/isatty"
	"github.com/github/git-sizer/sizes"
)

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

func (v *NegatedBoolValue) IsBoolFlag() bool {
	return true
}

func main() {
	err := mainImplementation()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
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

	flag.BoolVar(&processBranches, "branches", false, "process all branches")
	flag.BoolVar(&processTags, "tags", false, "process all tags")
	flag.BoolVar(&processRemotes, "remotes", false, "process all remote-tracking branches")
	flag.Var(
		&threshold, "threshold",
		"minimum level of concern (i.e., number of stars) that should be\n"+
			"        reported",
	)
	flag.Var(
		sizes.NewThresholdFlagValue(&threshold, 30),
		"critical", "only report critical statistics",
	)
	flag.Var(
		sizes.NewThresholdFlagValue(&threshold, 0),
		"verbose", "report all statistics, whether concerning or not",
	)
	flag.Var(
		&nameStyle, "names",
		"display names of large objects in the specified `style`:\n"+
			"            --names=none        omit footnotes entirely\n"+
			"            --names=hash        show only the SHA-1s of objects\n"+
			"            --names=full        show full names",
	)
	flag.BoolVar(&jsonOutput, "json", false, "output results in JSON format")
	flag.BoolVar(&jsonOutput, "j", false, "output results in JSON format")

	atty, err := isatty.Isatty(os.Stderr.Fd())
	if err != nil {
		atty = false
	}

	flag.BoolVar(&progress, "progress", atty, "report progress to stderr")
	flag.Var(&NegatedBoolValue{&progress}, "no-progress", "suppress progress output")

	flag.StringVar(&cpuprofile, "cpuprofile", "", "write cpu profile to file")

	flag.Parse()

	if cpuprofile != "" {
		f, err := os.Create(cpuprofile)
		if err != nil {
			return fmt.Errorf("couldn't set up cpuprofile file: %s", err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	args := flag.Args()

	if len(args) != 0 {
		return errors.New("excess arguments")
	}

	repo, err := sizes.NewRepository(".")
	if err != nil {
		return fmt.Errorf("couldn't open Git repository: %s", err)
	}
	defer repo.Close()

	var historySize sizes.HistorySize

	var filter sizes.ReferenceFilter
	if processBranches || processTags || processRemotes {
		var filters []sizes.ReferenceFilter
		if processBranches {
			filters = append(filters, sizes.BranchesFilter)
		}
		if processTags {
			filters = append(filters, sizes.TagsFilter)
		}
		if processRemotes {
			filters = append(filters, sizes.RemotesFilter)
		}
		filter = sizes.OrFilter(filters...)
	} else {
		filter = sizes.AllReferencesFilter
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
