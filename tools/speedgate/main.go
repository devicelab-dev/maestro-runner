// Command speedgate fails when a command type got slower than the committed
// baseline.
//
// It reads the reports of several runs of the same suite on the same device
// and driver (TestHive auth-suite three times, say), takes the median duration
// of each command type over every passed step, and compares it with the
// baseline. A type regresses when its median is slower by more than both
// thresholds (by default 20% and 50 ms). Exit status: 0 no regression,
// 1 at least one regression, 2 bad usage or unreadable input.
//
//	go run ./tools/speedgate [flags] <report-dir|report.json>...
//
// Writing a baseline (needs real runs on a device, so it is done by hand):
//
//	for i in 1 2 3; do maestro-runner test --output /tmp/sg/$i suite.yaml; done
//	go run ./tools/speedgate --write-baseline /tmp/sg/1 /tmp/sg/2 /tmp/sg/3
//
// A baseline is only meaningful for the device, driver and suite it was
// written from; record which in the commit that adds or updates it.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
)

const defaultBaseline = "benchmarks/speed-baseline.json"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("speedgate", flag.ContinueOnError)
	fl.SetOutput(stderr)
	baselinePath := fl.String("baseline", defaultBaseline, "baseline JSON to compare against (or write)")
	write := fl.Bool("write-baseline", false, "write the baseline from the reports instead of comparing")
	pct := fl.Float64("threshold-pct", 20, "allowed slowdown of a command type's median, percent")
	absMs := fl.Int64("threshold-ms", 50, "allowed slowdown of a command type's median, milliseconds")
	minSamples := fl.Int("min-samples", 3, "command types with fewer passed steps are not judged or baselined")
	fl.Usage = func() {
		fmt.Fprintln(stderr, "usage: speedgate [flags] <report-dir|report.json>...")
		fl.PrintDefaults()
	}
	if err := fl.Parse(args); err != nil {
		return 2
	}
	if fl.NArg() == 0 {
		fl.Usage()
		return 2
	}

	samples, err := LoadSamples(fl.Args())
	if err != nil {
		fmt.Fprintln(stderr, "speedgate:", err)
		return 2
	}

	if *write {
		b := BuildBaseline(samples, *minSamples)
		if err := WriteBaseline(*baselinePath, b); err != nil {
			fmt.Fprintln(stderr, "speedgate:", err)
			return 2
		}
		fmt.Fprintf(stdout, "wrote %s: %d command types from %d run(s) (%s/%s)\n",
			*baselinePath, len(b.Commands), b.Runs, b.Platform, b.Driver)
		return 0
	}

	base, err := ReadBaseline(*baselinePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			fmt.Fprintf(stderr, "speedgate: no baseline at %s; write one with --write-baseline\n", *baselinePath)
		} else {
			fmt.Fprintln(stderr, "speedgate:", err)
		}
		return 2
	}
	if base.Driver != samples.Driver || base.Platform != samples.Platform {
		fmt.Fprintf(stderr, "speedgate: warning: baseline is %s/%s, reports are %s/%s\n",
			base.Platform, base.Driver, samples.Platform, samples.Driver)
	}

	results := Compare(base, samples, Thresholds{Pct: *pct, AbsMs: *absMs, MinSamples: *minSamples})
	PrintResults(stdout, results)
	if n := Regressions(results); n > 0 {
		fmt.Fprintf(stdout, "\n%d command type(s) regressed beyond +%.0f%% and +%dms\n", n, *pct, *absMs)
		return 1
	}
	fmt.Fprintf(stdout, "\nno regression (%d run(s), thresholds +%.0f%% and +%dms)\n", samples.Runs, *pct, *absMs)
	return 0
}
