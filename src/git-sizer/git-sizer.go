package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/pprof"

	"github.com/github/git-sizer/sizes"
)

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
	var cpuprofile string
	var jsonOutput bool

	flag.BoolVar(&processBranches, "branches", false, "process all branches")
	flag.BoolVar(&processTags, "tags", false, "process all tags")
	flag.BoolVar(&processRemotes, "remotes", false, "process all remote-tracking branches")
	flag.BoolVar(&jsonOutput, "json", false, "output results in JSON format")
	flag.BoolVar(&jsonOutput, "j", false, "output results in JSON format")

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
	if len(args) == 0 {
		return errors.New("path argument(s) required")
	}
	path := args[0]
	args = args[1:]

	repo, err := sizes.NewRepository(path)
	if err != nil {
		return fmt.Errorf("couldn't open %v: %s", path, err)
	}
	defer repo.Close()

	var historySize sizes.HistorySize

	if len(args) > 0 {
		return errors.New("excess arguments")
	}

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

	historySize, err = sizes.ScanRepositoryUsingGraph(repo, filter)
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
		io.WriteString(os.Stdout, historySize.TableString())
	}

	return nil
}
