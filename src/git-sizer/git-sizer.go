package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime/pprof"

	"github.com/github/git-sizer/sizes"
)

func processObject(scanner *sizes.SizeScanner, spec string) {
	_, _, _, err := scanner.ObjectSize(spec)
	if err != nil {
		fmt.Fprintf(
			os.Stderr, "error: could not compute object size for '%s': %v\n",
			spec, err,
		)
		return
	}
}

func main() {
	var processAll bool
	var stdin bool
	var cpuprofile string

	flag.BoolVar(&processAll, "all", false, "process all references")
	flag.BoolVar(&stdin, "stdin", false, "read objects from stdin, one per line")

	flag.StringVar(&cpuprofile, "cpuprofile", "", "write cpu profile to file")

	flag.Parse()

	if cpuprofile != "" {
		f, err := os.Create(cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	args := flag.Args()
	if len(args) == 0 {
		log.Fatal("path argument(s) required")
	}
	path := args[0]
	specs := args[1:]

	repo, err := sizes.NewRepository(path)
	if err != nil {
		log.Panicf("error: couldn't open %v", path)
	}
	defer repo.Close()

	scanner, err := sizes.NewSizeScanner(repo)
	if err != nil {
		log.Panicf("error: couldn't create SizeScanner for %v", path)
	}

	for _, spec := range specs {
		processObject(scanner, spec)
	}

	if processAll {
		done := make(chan interface{})
		refOrErrors, err := repo.ForEachRef(done)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s", err)
			return
		}
		for refOrError := range refOrErrors {
			if refOrError.Error != nil {
				fmt.Fprintf(os.Stderr, "error reading references: %s", err)
				return
			}
			_, err := scanner.ReferenceSize(refOrError.Reference)
			if err != nil {
				fmt.Fprintf(
					os.Stderr, "error: could not compute object size for '': %v\n",
					refOrError.Reference.Refname, err,
				)
				return
			}
		}
	}

	if stdin {
		input := bufio.NewScanner(os.Stdin)
		for input.Scan() {
			spec := input.Text()
			processObject(scanner, spec)
		}
	}

	s, err := json.MarshalIndent(scanner.HistorySize, "", "    ")
	if err != nil {
		fmt.Fprintf(
			os.Stderr, "error: could not convert %v to json: %v\n",
			scanner.HistorySize, err,
		)
	}
	fmt.Printf("%s\n", s)
}
