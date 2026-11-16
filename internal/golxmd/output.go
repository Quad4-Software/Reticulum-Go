// SPDX-License-Identifier: LicenseRef-Reticulum
package golxmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/term"
)

func colorOut() io.Writer {
	return opConsole.writer()
}

func statusOK(msg string) string {
	return term.GreenW(colorOut(), msg)
}

func statusFail(msg string) string {
	return term.RedW(colorOut(), msg)
}

func statusWarn(msg string) string {
	return term.YellowW(colorOut(), msg)
}

func statusLabel(msg string) string {
	return term.CyanW(colorOut(), msg)
}

func statusBold(msg string) string {
	return term.BoldW(colorOut(), msg)
}

// PrintStartupBanner prints a concise operator summary on stdout.
func PrintStartupBanner(version, delivery, propagation, home, logFile string) {
	printOperatorSummary(OperatorSummary{
		Version:       version,
		Home:          home,
		LogFile:       logFile,
		Delivery:      delivery,
		Propagation:   propagation,
		PropagationOn: propagation != "",
	})
}

// PrintSelfCheckResults prints colored self-check output.
func PrintSelfCheckResults(results []SelfCheckResult) {
	fmt.Fprintln(colorOut())
	fmt.Fprintf(colorOut(), "%s\n", statusBold("golxmd self-check"))
	passed := 0
	for _, r := range results {
		mark := statusFail("FAIL")
		if r.OK {
			mark = statusOK("ok")
			passed++
		}
		line := fmt.Sprintf("  [%s] %s", mark, r.Name)
		if r.Detail != "" {
			line += ": " + r.Detail
		}
		fmt.Fprintln(colorOut(), line)
	}
	fmt.Fprintln(colorOut())
	summary := fmt.Sprintf("%d/%d checks passed", passed, len(results))
	if passed == len(results) {
		fmt.Fprintf(colorOut(), "%s\n\n", statusOK(summary))
	} else {
		fmt.Fprintf(colorOut(), "%s\n\n", statusFail(summary))
	}
}

// PrintFirstRunNotice prints a colored first-run hint on stderr.
func PrintFirstRunNotice(cfgPath, identPath, storageDir, rnsDir string) {
	fmt.Fprintf(os.Stderr, "\n%s Created default golxmd files.\n", statusOK("ok"))
	fmt.Fprintf(os.Stderr, "  %s %s\n", statusLabel("Config:"), cfgPath)
	fmt.Fprintf(os.Stderr, "  %s %s\n", statusLabel("Identity:"), identPath)
	fmt.Fprintf(os.Stderr, "  %s %s\n", statusLabel("Storage:"), storageDir)
	if rnsDir != "" {
		fmt.Fprintf(os.Stderr, "  %s %s\n", statusLabel("RNS config:"), filepath.Join(rnsDir, "config"))
	}
	fmt.Fprintf(os.Stderr, "\nEdit the configuration, then re-run golxmd.\n\n")
}

func printStatusHeader(destHash string, uptime float64) {
	fmt.Fprintln(colorOut())
	fmt.Fprintf(colorOut(), "%s %s, uptime %s\n",
		statusBold("LXMF Propagation Node"),
		prettyHex(destHash),
		statusLabel(prettyTime(uptime)),
	)
}

func printKV(label, value string) {
	fmt.Fprintf(colorOut(), "  %s %s\n", statusLabel(label+":"), value)
}

func printSection(title string) {
	fmt.Fprintf(colorOut(), "\n%s\n", statusBold(title))
}

func indentPeerLine(s string) string {
	return "  " + s
}

func peerStatusLabel(available bool) string {
	if available {
		return statusOK("Available")
	}
	return statusFail("Unreachable")
}

func truncateName(name string, max int) string {
	name = sanitizeName(name)
	if len(name) <= max {
		return name
	}
	return name[:max] + "..."
}

func joinPathParts(parts ...string) string {
	return strings.Join(parts, string(os.PathSeparator))
}
