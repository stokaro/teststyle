package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/stokaro/teststyle"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	flags := flag.NewFlagSet("teststyle", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	baselinePath := flags.String("baseline", ".teststyle-baseline.json", "Path to the committed test-style baseline")
	writeBaseline := flags.Bool("write-baseline", false, "Rewrite the baseline from current findings")
	root := flags.String("root", ".", "Repository root to scan")
	disabledRules := flags.String("disable", "", "Comma-separated rule IDs to disable")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	config := teststyle.Config{DisabledRules: splitCommaList(*disabledRules)}
	current, err := teststyle.ScanWithConfig(*root, config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "teststyle: scan failed: %v\n", err)
		return 1
	}
	if *writeBaseline {
		if err := teststyle.WriteBaseline(*baselinePath, current); err != nil {
			fmt.Fprintf(os.Stderr, "teststyle: write baseline failed: %v\n", err)
			return 1
		}
		_, _ = fmt.Fprintf(os.Stdout, "teststyle: wrote %s (%d conditional groups, %d white-box files)\n",
			*baselinePath, len(current.TestConditionals), len(current.WhiteBoxFiles))
		return 0
	}

	baseline, err := teststyle.ReadBaseline(*baselinePath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "teststyle: read baseline failed: %v\n", err)
			return 1
		}
	}
	if diff := teststyle.Diff(baseline, current); diff != "" {
		fmt.Fprintf(os.Stderr, "teststyle: %v\n%s\n\nRun go tool teststyle -write-baseline after intentional cleanup.\n",
			teststyle.ErrBaselineMismatch, diff)
		return 1
	}
	_, _ = fmt.Fprintf(os.Stdout, "teststyle: OK (%d conditional groups, %d white-box files)\n",
		len(current.TestConditionals), len(current.WhiteBoxFiles))
	return 0
}

func splitCommaList(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		result = append(result, part)
	}
	return result
}
