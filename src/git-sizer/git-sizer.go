package main

import (
	"bufio"
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
	var processAll bool
	var processBranches bool
	var processTags bool
	var processRemotes bool
	var processStdin bool
	var cpuprofile string
	var jsonOutput bool

	flag.BoolVar(&processAll, "all", false, "process all references")
	flag.BoolVar(&processBranches, "branches", false, "process all branches")
	flag.BoolVar(&processTags, "tags", false, "process all tags")
	flag.BoolVar(&processRemotes, "remotes", false, "process all remote-tracking branches")
	flag.BoolVar(&processStdin, "stdin", false, "read objects from stdin, one per line")
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
	specs := args[1:]

	repo, err := sizes.NewRepository(path)
	if err != nil {
		return fmt.Errorf("couldn't open %v: %s", path, err)
	}
	defer repo.Close()

	var historySize sizes.HistorySize

	if processAll || processBranches || processTags || processRemotes {
		if processStdin || len(specs) > 0 {
			return errors.New("--all must not be used together with other specs")
		}

		var filter sizes.ReferenceFilter
		if processAll {
			filter = sizes.AllReferencesFilter
		} else {
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
		}

		historySize, err = sizes.ScanRepository(repo, filter)
		if err != nil {
			return fmt.Errorf("error scanning repository: %s", err)
		}
	} else {
		scanner, err := sizes.NewSizeScanner(repo)
		if err != nil {
			return fmt.Errorf("couldn't create SizeScanner for %v: %s", path, err)
		}

		err = scanner.PreloadBlobs()
		if err != nil {
			return fmt.Errorf("couldn't preload blobs: %s", err)
		}

		foundSpec := false

		for _, spec := range specs {
			_, _, _, err := scanner.ObjectSize(spec)
			if err != nil {
				return fmt.Errorf("error processing object %v: %s", spec, err)
			}
			foundSpec = true
		}

		if processStdin {
			input := bufio.NewScanner(os.Stdin)
			for input.Scan() {
				spec := input.Text()
				_, _, _, err := scanner.ObjectSize(spec)
				if err != nil {
					return fmt.Errorf("error processing object %v: %s", spec, err)
				}
				foundSpec = true
			}
		}

		if !foundSpec {
			return errors.New("no objects specified")
		}

		historySize = scanner.HistorySize
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
