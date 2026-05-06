// Command testsummary runs `go test -json` and prints a concise failure summary at the end.
//
// Usage:
//
//	go run ./scripts/ci/testsummary [go test args...]
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
	Action  string `json:"Action"`
	Package string `json:"Package"`
	Test    string `json:"Test"`
	Output  string `json:"Output"`
}

func main() {
	os.Exit(run())
}

func run() int {
	args := append([]string{"test", "-json"}, os.Args[1:]...)
	cmd := exec.Command("go", args...)

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

	failedTests := make(map[string]map[string]struct{})
	testOutputs := make(map[string]map[string][]string)
	pkgOutputs := make(map[string][]string)
	failedPackages := make(map[string]struct{})

	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		line := strings.TrimSuffix(sc.Text(), "\n")
		var ev testEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			fmt.Println(line)
			continue
		}

		if ev.Output != "" {
			fmt.Print(ev.Output)
		}

		switch ev.Action {
		case "fail":
			if ev.Test != "" {
				if failedTests[ev.Package] == nil {
					failedTests[ev.Package] = make(map[string]struct{})
				}
				failedTests[ev.Package][ev.Test] = struct{}{}
			} else {
				failedPackages[ev.Package] = struct{}{}
			}
		case "output":
			if ev.Output == "" {
				continue
			}
			if ev.Test != "" {
				if testOutputs[ev.Package] == nil {
					testOutputs[ev.Package] = make(map[string][]string)
				}
				testOutputs[ev.Package][ev.Test] = append(testOutputs[ev.Package][ev.Test], ev.Output)
			} else {
				pkgOutputs[ev.Package] = append(pkgOutputs[ev.Package], ev.Output)
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

	if stderrBuf.Len() > 0 {
		_, _ = os.Stderr.Write(stderrBuf.Bytes())
	}

	exit := 0
	if waitErr != nil {
		if ee, ok := waitErr.(*exec.ExitError); ok {
			exit = ee.ExitCode()
		} else {
			exit = 1
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
		printSummary(failedPackages, failedTests, pkgOutputs, testOutputs, totalFailed)
	}

	return exit
}

func printSummary(
	failedPackages map[string]struct{},
	failedTests map[string]map[string]struct{},
	pkgOutputs map[string][]string,
	testOutputs map[string]map[string][]string,
	totalFailed int,
) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("TEST FAILURE SUMMARY")
	fmt.Println(strings.Repeat("=", 60))

	if len(failedPackages) > 0 {
		fmt.Println("\nFailed packages:")
		pkgs := sortedKeysSet(failedPackages)
		for _, pkg := range pkgs {
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

	fmt.Println("\n" + strings.Repeat("-", 60))
	fmt.Println("FAILURE DETAILS")
	fmt.Println(strings.Repeat("-", 60))

	for _, pkg := range sortedKeysSet(failedPackages) {
		fmt.Printf("\n=== %s (package failure) ===\n", pkg)
		for _, line := range pkgOutputs[pkg] {
			fmt.Print(line)
		}
	}

	for _, pkg := range sortedKeysMap(failedTests) {
		names := make([]string, 0, len(failedTests[pkg]))
		for t := range failedTests[pkg] {
			names = append(names, t)
		}
		slices.Sort(names)
		for _, test := range names {
			fmt.Printf("\n=== %s  %s ===\n", pkg, test)
			var lines []string
			if byTest := testOutputs[pkg]; byTest != nil {
				lines = byTest[test]
			}
			for _, line := range lines {
				fmt.Print(line)
			}
		}
	}

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Printf("Total failures: %d\n", totalFailed)
	fmt.Println(strings.Repeat("=", 60))
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
