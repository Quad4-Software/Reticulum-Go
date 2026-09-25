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
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/Quad4-Software/Reticulum-Go/pkg/term"
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

// rerunRounds returns the TESTSUMMARY_RERUN_FAILS retry budget: the number of
// extra passes each failed test gets before it counts as a real failure.
func rerunRounds() int {
	v := os.Getenv("TESTSUMMARY_RERUN_FAILS")
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// rerunArgs rebuilds go test argv for a retry pass: keeps user flags like -C,
// -tags, -race, -short, -timeout and -vet, but drops -run/-count/-coverprofile
// so the retry selects only the failed tests without cache reuse.
func rerunArgs(user []string, pkg string, tests []string) []string {
	var out []string
	for i := 0; i < len(user); i++ {
		a := user[i]
		switch {
		case a == "-run" || a == "-count" || a == "-coverprofile" || a == "-cpu":
			i++
		case strings.HasPrefix(a, "-run=") || strings.HasPrefix(a, "-count=") ||
			strings.HasPrefix(a, "-coverprofile=") || strings.HasPrefix(a, "-cpu="):
		case a == "-json":
		default:
			out = append(out, a)
		}
	}
	out = append(out, "-count=1")
	if len(tests) > 0 {
		quoted := make([]string, 0, len(tests))
		for _, t := range tests {
			quoted = append(quoted, regexp.QuoteMeta(t))
		}
		slices.Sort(quoted)
		out = append(out, "-run", "^("+strings.Join(quoted, "|")+")$")
	}
	out = append(out, pkg)
	return out
}

// topLevelTestName strips subtests so -run can address the parent.
func topLevelTestName(name string) string {
	if i := strings.IndexByte(name, '/'); i >= 0 {
		return name[:i]
	}
	return name
}

// rerunFailed re-runs each failed test up to TESTSUMMARY_RERUN_FAILS times.
// Tests that pass on any retry are reported as FLAKE and removed from the
// failure sets. Package level failures without a named test are retried as a
// whole package. Retried tests run under -count=1 so the cache cannot hide a
// real failure.
func rerunFailed(
	user []string,
	failedTests map[string]map[string]struct{},
	failedPackages map[string]struct{},
) []string {
	var flakes []string
	for round := 1; round <= rerunRounds(); round++ {
		if len(failedTests) == 0 && len(failedPackages) == 0 {
			break
		}
		for pkg := range failedPackages {
			if len(failedTests[pkg]) > 0 {
				// Named test failures cover this package already.
				continue
			}
			fmt.Fprintf(os.Stderr, "testsummary: retry round %d for failed package %s\n", round, pkg)
			if rerunPackage(user, pkg) == 0 {
				delete(failedPackages, pkg)
				flakes = append(flakes, pkg)
				fmt.Fprintf(os.Stderr, "%s %s (package passed on retry %d)\n",
					term.Yellow(os.Stderr, "testsummary: FLAKE"), pkg, round)
			}
		}
		for pkg, tests := range failedTests {
			uniq := make(map[string]struct{})
			for t := range tests {
				uniq[topLevelTestName(t)] = struct{}{}
			}
			names := make([]string, 0, len(uniq))
			for n := range uniq {
				names = append(names, n)
			}
			fmt.Fprintf(os.Stderr, "testsummary: retry round %d for %s (%d tests)\n", round, pkg, len(names))
			passed := rerunPackageTests(user, pkg, names)
			for orig := range tests {
				if !passed[topLevelTestName(orig)] {
					continue
				}
				delete(tests, orig)
				flakes = append(flakes, pkg+" "+orig)
				fmt.Fprintf(os.Stderr, "%s %s %s (passed on retry %d)\n",
					term.Yellow(os.Stderr, "testsummary: FLAKE"), pkg, orig, round)
			}
			if len(tests) == 0 {
				delete(failedTests, pkg)
				delete(failedPackages, pkg)
			}
		}
	}
	return flakes
}

// rerunPackage retries a package that failed without named tests (build
// failure, panic, TestMain exit) and reports whether the retry passed.
func rerunPackage(user []string, pkg string) int {
	args := rerunArgs(user, pkg, nil)
	cmd := exec.Command("go", goTestArgs(args)...) // #nosec G204 -- same flag policy as the main run
	if env := childEnv(); env != nil {
		cmd.Env = env
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		if tail := tailLines(buf.String(), 20); tail != "" {
			fmt.Fprintf(os.Stderr, "testsummary: retry output for %s:\n%s\n", pkg, tail)
		}
		return 1
	}
	return 0
}

// rerunPackageTests runs a -run filtered retry and returns which named tests
// now pass. A test absent from the JSON output counts as not passed.
func rerunPackageTests(user []string, pkg string, tests []string) map[string]bool {
	args := rerunArgs(user, pkg, tests)
	cmd := exec.Command("go", goTestArgs(args)...) // #nosec G204 -- same flag policy as the main run
	if env := childEnv(); env != nil {
		cmd.Env = env
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	_ = cmd.Run()
	out := buf.String()
	passed := make(map[string]bool)
	failed := make(map[string]bool)
	for _, line := range strings.Split(out, "\n") {
		var ev testEvent
		if json.Unmarshal([]byte(line), &ev) != nil || ev.Test == "" {
			continue
		}
		top := topLevelTestName(ev.Test)
		switch ev.Action {
		case "pass":
			passed[top] = true
		case "fail":
			failed[top] = true
		}
	}
	for _, t := range tests {
		passed[t] = passed[t] && !failed[t]
	}
	if len(failed) > 0 {
		if tail := tailLines(out, 20); tail != "" {
			fmt.Fprintf(os.Stderr, "testsummary: retry output for %s:\n%s\n", pkg, tail)
		}
	}
	return passed
}

func tailLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
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

func validGoTestArg(arg string) bool {
	if arg == "" || strings.ContainsAny(arg, "\x00\n\r;`&") {
		return false
	}
	return true
}

func validateGoTestArgs(user []string) error {
	for _, arg := range user {
		if !validGoTestArg(arg) {
			return fmt.Errorf("invalid go test argument")
		}
	}
	return nil
}

func run() int {
	user := os.Args[1:]
	if err := validateGoTestArgs(user); err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", term.Red(os.Stderr, "testsummary:"), err)
		return 2
	}
	cmd := exec.Command("go", goTestArgs(user)...) // #nosec G204,G702 -- go binary is fixed, argv is go test flags checked by validateGoTestArgs
	if env := childEnv(); env != nil {
		cmd.Env = env
	}

	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", term.Red(os.Stderr, "testsummary:"), err)
		return 1
	}
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", term.Red(os.Stderr, "testsummary:"), err)
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
					fmt.Printf("\n%s %s  %s ===\n",
						term.Red(os.Stdout, "=== FAIL"), ev.Package, ev.Test)
				}
			} else {
				failedPackages[ev.Package] = struct{}{}
				if quiet {
					fmt.Printf("\n%s %s ===\n", term.Red(os.Stdout, "=== FAIL"), ev.Package)
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
		fmt.Fprintf(os.Stderr, "%s reading test output: %v\n", term.Red(os.Stderr, "testsummary:"), err)
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

	spuriousDeadline := exit != 0 && isSpuriousFuzzDeadline(failedTests, failedPackages, testOutputs, pkgOutputs)
	if spuriousDeadline {
		exit = 0
		for pkg := range failedTests {
			if quiet {
				fmt.Fprintf(os.Stderr, "%s %s spurious fuzz deadline (go#75804), treating as pass\n",
					term.Yellow(os.Stderr, "testsummary:"), pkg)
			}
			passedPackages[pkg] = 0
			delete(failedPackages, pkg)
		}
		failedTests = make(map[string]map[string]struct{})
	}

	if rerunRounds() > 0 && (len(failedTests) > 0 || len(failedPackages) > 0) {
		flakes := rerunFailed(user, failedTests, failedPackages)
		if len(flakes) > 0 {
			slices.Sort(flakes)
			fmt.Printf("\n%s\n", term.Yellow(os.Stdout, "testsummary: FLAKY TESTS (passed on retry):"))
			for _, f := range flakes {
				fmt.Printf("  - %s\n", f)
			}
		}
		if len(failedTests) == 0 && len(failedPackages) == 0 {
			exit = 0
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
			fmt.Printf("%s  %s  %.3fs\n", term.Green(os.Stdout, "ok"), pkg, passedPackages[pkg])
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
			fmt.Println("\n" + term.Dim(os.Stdout, strings.Repeat("-", 60)))
			fmt.Println(term.Bold(os.Stdout, "GO TEST STDERR"))
			fmt.Println(term.Dim(os.Stdout, strings.Repeat("-", 60)))
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
	fmt.Println("\n" + term.Red(os.Stdout, strings.Repeat("=", 60)))
	fmt.Println(term.Bold(os.Stdout, "TEST FAILURE SUMMARY"))
	fmt.Println(term.Red(os.Stdout, strings.Repeat("=", 60)))

	if len(failedPackages) > 0 {
		fmt.Println("\n" + term.Yellow(os.Stdout, "Failed packages:"))
		for _, pkg := range sortedKeysSet(failedPackages) {
			fmt.Printf("  - %s\n", pkg)
		}
	}

	if len(failedTests) > 0 {
		fmt.Println("\n" + term.Yellow(os.Stdout, "Failed tests:"))
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
		fmt.Println("\n" + term.Dim(os.Stdout, strings.Repeat("-", 60)))
		fmt.Println(term.Bold(os.Stdout, "FAILURE DETAILS"))
		fmt.Println(term.Dim(os.Stdout, strings.Repeat("-", 60)))

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

	fmt.Println("\n" + term.Red(os.Stdout, strings.Repeat("=", 60)))
	fmt.Printf("%s %d\n", term.Bold(os.Stdout, "Total failures:"), totalFailed)
	fmt.Println(term.Red(os.Stdout, strings.Repeat("=", 60)))
}

// trimFailureOutput keeps CI logs readable under TESTSUMMARY_QUIET by retaining
// assertion/fatal lines and a short tail of context instead of full slog dumps.
// isSpuriousFuzzDeadline reports the Go stdlib race (go.dev/issue/75804) where
// -fuzztime expiry can leak "context deadline exceeded" with no file:line as a
// test failure. Real assertion failures always cite _test.go:line.
func isSpuriousFuzzDeadline(
	failedTests map[string]map[string]struct{},
	failedPackages map[string]struct{},
	testOutputs map[string]map[string][]string,
	pkgOutputs map[string][]string,
) bool {
	if len(failedTests) != 1 {
		return false
	}
	for pkg, tests := range failedTests {
		if len(tests) != 1 {
			return false
		}
		for p := range failedPackages {
			if p != pkg {
				return false
			}
		}
		if !sawFuzzProgress(pkgOutputs, testOutputs, pkg) {
			return false
		}
		for test := range tests {
			if !strings.HasPrefix(test, "Fuzz") {
				return false
			}
			if !isOnlyFuzzDeadlineFailure(testOutputs[pkg][test]) {
				return false
			}
		}
	}
	return true
}

func sawFuzzProgress(
	pkgOutputs map[string][]string,
	testOutputs map[string]map[string][]string,
	pkg string,
) bool {
	for _, line := range pkgOutputs[pkg] {
		if strings.Contains(line, "fuzz: elapsed:") {
			return true
		}
	}
	for _, lines := range testOutputs[pkg] {
		for _, line := range lines {
			if strings.Contains(line, "fuzz: elapsed:") {
				return true
			}
		}
	}
	return false
}

func isOnlyFuzzDeadlineFailure(lines []string) bool {
	sawDeadline := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.Contains(trimmed, "_test.go:") {
			return false
		}
		switch {
		case trimmed == "context deadline exceeded":
			sawDeadline = true
		case strings.HasPrefix(trimmed, "=== RUN"),
			strings.HasPrefix(trimmed, "--- FAIL:"),
			strings.HasPrefix(trimmed, "--- PASS:"),
			strings.HasPrefix(trimmed, "fuzz:"):
		default:
			return false
		}
	}
	return sawDeadline
}

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
