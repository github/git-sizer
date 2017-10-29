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

func processObject(cache *sizes.SizeCache, spec string) {
	_, _, _, err := cache.ObjectSize(spec)
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

	cache, err := sizes.NewSizeCache(repo)
	if err != nil {
		log.Panicf("error: couldn't create SizeCache for %v", path)
	}

	for _, spec := range specs {
		processObject(cache, spec)
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
			_, err := cache.ReferenceSize(refOrError.Reference)
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
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			spec := scanner.Text()
			processObject(cache, spec)
		}
	}

	s, err := json.MarshalIndent(cache.HistorySize, "", "    ")
	if err != nil {
		fmt.Fprintf(
			os.Stderr, "error: could not convert %v to json: %v\n",
			cache.HistorySize, err,
		)
	}
	fmt.Printf("%s\n", s)
}
