// Command testsummary runs go test -json and prints a concise failure summary at the end.
//
// Usage:
//
//	go run ./scripts/ci/testsummary [go test args...]
//
// Set TESTSUMMARY_QUIET=1 (or CI_QUIET_TESTS=1) to suppress per-test log noise in CI.
// Passing packages print a single "ok  import/path" line. Failures still print full output.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
)

type testEvent struct {
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Output  string  `json:"Output"`
	Elapsed float64 `json:"Elapsed"`
}

func main() {
	os.Exit(run())
}

func quietMode() bool {
	return os.Getenv("TESTSUMMARY_QUIET") != "" || os.Getenv("CI_QUIET_TESTS") != ""
}

// childEnv builds the environment for the go test child. TESTSUMMARY_GOOS
// and TESTSUMMARY_GOARCH are applied only to the child so this host binary
// can still be built and run when targeting js/wasm tests.
func childEnv() []string {
	goos := os.Getenv("TESTSUMMARY_GOOS")
	goarch := os.Getenv("TESTSUMMARY_GOARCH")
	if goos == "" && goarch == "" {
		return nil
	}
	out := make([]string, 0, 32)
	for _, e := range os.Environ() {
		switch {
		case strings.HasPrefix(e, "GOOS="),
			strings.HasPrefix(e, "GOARCH="),
			strings.HasPrefix(e, "TESTSUMMARY_GOOS="),
			strings.HasPrefix(e, "TESTSUMMARY_GOARCH="):
			continue
		}
		out = append(out, e)
	}
	if goos != "" {
		out = append(out, "GOOS="+goos)
	}
	if goarch != "" {
		out = append(out, "GOARCH="+goarch)
	}
	return out
}

// goTestArgs builds go test argv. The go tool requires -C to be the first
// flag, so any user -C is placed before -json.
func goTestArgs(user []string) []string {
	out := make([]string, 0, len(user)+3)
	out = append(out, "test")
	rest := user
	switch {
	case len(rest) >= 2 && rest[0] == "-C":
		out = append(out, "-C", rest[1])
		rest = rest[2:]
	case len(rest) >= 1 && strings.HasPrefix(rest[0], "-C="):
		out = append(out, rest[0])
		rest = rest[1:]
	}
	out = append(out, "-json")
	out = append(out, rest...)
	return out
}

func run() int {
	cmd := exec.Command("go", goTestArgs(os.Args[1:])...)
	if env := childEnv(); env != nil {
		cmd.Env = env
	}

	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "testsummary: %v\n", err)
		return 1
	}
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "testsummary: %v\n", err)
		return 1
	}

	quiet := quietMode()
	failedTests := make(map[string]map[string]struct{})
	testOutputs := make(map[string]map[string][]string)
	pkgOutputs := make(map[string][]string)
	failedPackages := make(map[string]struct{})
	passedPackages := make(map[string]float64)
	emitOutput := make(map[string]map[string]bool)

	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		line := strings.TrimSuffix(sc.Text(), "\n")
		var ev testEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			if !quiet {
				fmt.Println(line)
			}
			continue
		}

		switch ev.Action {
		case "fail":
			if ev.Test != "" {
				if failedTests[ev.Package] == nil {
					failedTests[ev.Package] = make(map[string]struct{})
				}
				failedTests[ev.Package][ev.Test] = struct{}{}
				if emitOutput[ev.Package] == nil {
					emitOutput[ev.Package] = make(map[string]bool)
				}
				emitOutput[ev.Package][ev.Test] = true
				if quiet {
					fmt.Printf("\n=== FAIL %s  %s ===\n", ev.Package, ev.Test)
				}
			} else {
				failedPackages[ev.Package] = struct{}{}
				if quiet {
					fmt.Printf("\n=== FAIL %s ===\n", ev.Package)
				}
			}
			if ev.Output != "" {
				fmt.Print(ev.Output)
			}
		case "output":
			if ev.Output == "" {
				continue
			}
			shouldPrint := !quiet
			if quiet && ev.Test != "" {
				shouldPrint = emitOutput[ev.Package] != nil && emitOutput[ev.Package][ev.Test]
			}
			if quiet && ev.Test == "" {
				_, shouldPrint = failedPackages[ev.Package]
			}
			if shouldPrint {
				fmt.Print(ev.Output)
			}
			if ev.Test != "" {
				if testOutputs[ev.Package] == nil {
					testOutputs[ev.Package] = make(map[string][]string)
				}
				testOutputs[ev.Package][ev.Test] = append(testOutputs[ev.Package][ev.Test], ev.Output)
			} else {
				pkgOutputs[ev.Package] = append(pkgOutputs[ev.Package], ev.Output)
			}
		case "pass":
			if quiet && ev.Test == "" && ev.Package != "" {
				passedPackages[ev.Package] = ev.Elapsed
			} else if !quiet && ev.Output != "" {
				fmt.Print(ev.Output)
			}
		}
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "testsummary: reading test output: %v\n", err)
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return 1
	}

	waitErr := cmd.Wait()

	exit := 0
	if waitErr != nil {
		if ee, ok := waitErr.(*exec.ExitError); ok {
			exit = ee.ExitCode()
		} else {
			exit = 1
		}
	}

	if quiet {
		for _, pkg := range sortedKeysFloat(passedPackages) {
			if _, failed := failedPackages[pkg]; failed {
				continue
			}
			if tests, ok := failedTests[pkg]; ok && len(tests) > 0 {
				continue
			}
			fmt.Printf("ok  %s  %.3fs\n", pkg, passedPackages[pkg])
		}
	}

	for pkg := range failedPackages {
		if len(failedTests[pkg]) > 0 {
			delete(failedPackages, pkg)
		}
	}

	totalFailed := len(failedPackages)
	for _, tests := range failedTests {
		totalFailed += len(tests)
	}

	if totalFailed > 0 {
		if stderrBuf.Len() > 0 {
			fmt.Println("\n" + strings.Repeat("-", 60))
			fmt.Println("GO TEST STDERR")
			fmt.Println(strings.Repeat("-", 60))
			_, _ = os.Stdout.Write(stderrBuf.Bytes())
		}
		printSummary(failedPackages, failedTests, pkgOutputs, testOutputs, totalFailed, quiet)
	} else if stderrBuf.Len() > 0 {
		_, _ = os.Stderr.Write(stderrBuf.Bytes())
	}

	return exit
}

func printSummary(
	failedPackages map[string]struct{},
	failedTests map[string]map[string]struct{},
	pkgOutputs map[string][]string,
	testOutputs map[string]map[string][]string,
	totalFailed int,
	quiet bool,
) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("TEST FAILURE SUMMARY")
	fmt.Println(strings.Repeat("=", 60))

	if len(failedPackages) > 0 {
		fmt.Println("\nFailed packages:")
		for _, pkg := range sortedKeysSet(failedPackages) {
			fmt.Printf("  - %s\n", pkg)
		}
	}

	if len(failedTests) > 0 {
		fmt.Println("\nFailed tests:")
		for _, pkg := range sortedKeysMap(failedTests) {
			names := make([]string, 0, len(failedTests[pkg]))
			for t := range failedTests[pkg] {
				names = append(names, t)
			}
			slices.Sort(names)
			for _, test := range names {
				fmt.Printf("  - %s  %s\n", pkg, test)
			}
		}
	}

	if len(pkgOutputs) > 0 || len(testOutputs) > 0 {
		fmt.Println("\n" + strings.Repeat("-", 60))
		fmt.Println("FAILURE DETAILS")
		fmt.Println(strings.Repeat("-", 60))

		for _, pkg := range sortedKeysSet(failedPackages) {
			fmt.Printf("\n=== %s (package failure) ===\n", pkg)
			fmt.Print(trimFailureOutput(pkgOutputs[pkg], quiet))
		}

		for _, pkg := range sortedKeysMap(failedTests) {
			names := make([]string, 0, len(failedTests[pkg]))
			for t := range failedTests[pkg] {
				names = append(names, t)
			}
			slices.Sort(names)
			for _, test := range names {
				fmt.Printf("\n=== %s  %s ===\n", pkg, test)
				fmt.Print(trimFailureOutput(testOutputs[pkg][test], quiet))
			}
		}
	}

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Printf("Total failures: %d\n", totalFailed)
	fmt.Println(strings.Repeat("=", 60))
}

// trimFailureOutput keeps CI logs readable under TESTSUMMARY_QUIET by retaining
// assertion/fatal lines and a short tail of context instead of full slog dumps.
func trimFailureOutput(lines []string, quiet bool) string {
	if len(lines) == 0 {
		return ""
	}
	joined := strings.Join(lines, "")
	if !quiet {
		return joined
	}
	const maxTailLines = 40
	var keep []string
	for _, line := range lines {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "--- fail") ||
			strings.Contains(lower, "fatal") ||
			strings.Contains(lower, "error:") ||
			strings.Contains(line, "\t") && (strings.Contains(lower, "fail") || strings.Contains(lower, "timeout") || strings.Contains(lower, "want ")) {
			keep = append(keep, line)
		}
	}
	all := strings.Split(strings.TrimRight(joined, "\n"), "\n")
	if len(all) > maxTailLines {
		all = all[len(all)-maxTailLines:]
	}
	tail := strings.Join(all, "\n") + "\n"
	if len(keep) == 0 {
		return tail
	}
	return strings.Join(keep, "") + "\n--- last lines ---\n" + tail
}

func sortedKeysSet(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func sortedKeysMap[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func sortedKeysFloat(m map[string]float64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
