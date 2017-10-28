package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime/pprof"
	"strconv"

	"github.com/github/git-sizer/sizes"
)

type outputFunction func(oid sizes.Oid, objectType sizes.Type, size sizes.Size)

func outputNothing(oid sizes.Oid, objectType sizes.Type, size sizes.Size) {
}

func outputLine(oid sizes.Oid, objectType sizes.Type, size sizes.Size) {
	fmt.Printf("%s %s %s\n", oid, objectType, size)
}

func outputJSON(oid sizes.Oid, objectType sizes.Type, size sizes.Size) {
	results := [...]interface{}{oid.String(), objectType, size}
	s, err := json.MarshalIndent(results, "", "    ")
	if err != nil {
		fmt.Fprintf(
			os.Stderr, "error: could not convert %v to json: %v\n",
			results, err,
		)
	}
	fmt.Printf("%s\n", s)
}

func processObject(cache *sizes.SizeCache, spec string, output outputFunction) {
	oid, objectType, objectSize, err := cache.ObjectSize(spec)
	if err != nil {
		fmt.Fprintf(
			os.Stderr, "error: could not compute object size for '%s': %v\n",
			spec, err,
		)
		return
	}

	output(oid, objectType, objectSize)
}

func processSpec(
	repo *sizes.Repository, cache *sizes.SizeCache,
	spec string, output outputFunction,
) {
	processObject(cache, spec, output)
}

type outputValue struct {
	p         *outputFunction
	value     outputFunction
	defaultOn bool
}

func (v outputValue) IsBoolFlag() bool {
	return true
}

func (v outputValue) String() string {
	return strconv.FormatBool(v.defaultOn)
}

func (v outputValue) Set(_ string) error {
	*v.p = v.value
	return nil
}

func main() {
	var output outputFunction = outputLine
	var stdin bool
	var cpuprofile string

	flag.Var(outputValue{&output, outputLine, true}, "line", "linewise output")
	flag.Var(outputValue{&output, outputLine, true}, "l", "linewise output")

	flag.Var(outputValue{&output, outputNothing, false}, "quiet", "suppress output")
	flag.Var(outputValue{&output, outputNothing, false}, "q", "suppress output")

	flag.Var(outputValue{&output, outputJSON, false}, "json", "output results as JSON")
	flag.Var(outputValue{&output, outputJSON, false}, "j", "output results as JSON")

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
		processSpec(repo, cache, spec, output)
	}

	if stdin {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			spec := scanner.Text()
			processObject(cache, spec, output)
		}
	}
}
