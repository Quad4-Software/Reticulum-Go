// SPDX-License-Identifier: LicenseRef-Reticulum
package gorrcd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

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

func stderrOK(msg string) string {
	return term.GreenW(os.Stderr, msg)
}

func stderrLabel(msg string) string {
	return term.CyanW(os.Stderr, msg)
}

func printKV(label, value string) {
	opConsole.printf("  %s %s\n", statusLabel(label+":"), value)
}

func printSection(title string) {
	opConsole.printf("\n%s\n", statusBold(title))
}

func liveStatusEnabled() bool {
	return opConsole.isLive()
}

func writeLiveStatusLine(line string) {
	opConsole.updateStatus(line)
}

func clearLiveStatusLine() {
	opConsole.clearStatus()
}

// PrintFirstRunNotice prints a colored first-run hint on stderr.
func PrintFirstRunNotice(cfgPath, identPath, roomsPath, rnsDir string) {
	fmt.Fprintf(os.Stderr, "\n%s Created default gorrcd files.\n", stderrOK("ok"))
	fmt.Fprintf(os.Stderr, "  %s %s\n", stderrLabel("Config:"), cfgPath)
	fmt.Fprintf(os.Stderr, "  %s %s\n", stderrLabel("Identity:"), identPath)
	fmt.Fprintf(os.Stderr, "  %s %s\n", stderrLabel("Rooms:"), roomsPath)
	if rnsDir != "" {
		fmt.Fprintf(os.Stderr, "  %s %s\n", stderrLabel("RNS config:"), filepath.Join(rnsDir, "config"))
	}
	fmt.Fprintf(os.Stderr, "\nEdit the configuration, then re-run gorrcd.\n\n")
}
