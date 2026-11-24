// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"
)

type helpLine struct {
	Cmd  string
	Desc string
}

func helpTitle(w io.Writer, title string) {
	_, _ = fmt.Fprintf(w, "%s\n\n", boldMsg(w, title))
}

func helpText(w io.Writer, text string) {
	for line := range strings.SplitSeq(text, "\n") {
		if strings.TrimSpace(line) == "" {
			_, _ = fmt.Fprintln(w)
			continue
		}
		_, _ = fmt.Fprintln(w, line)
	}
}

func helpUsageHeader(w io.Writer) {
	_, _ = fmt.Fprintf(w, "%s\n", boldMsg(w, "Usage:"))
}

func helpUsageLine(w io.Writer, cmd, desc string) {
	if desc == "" {
		_, _ = fmt.Fprintf(w, "  %s\n", infoMsg(w, cmd))
		return
	}
	const cmdCol = 54
	pad := max(cmdCol-len(cmd), 2)
	_, _ = fmt.Fprintf(w, "  %s%s%s\n", infoMsg(w, cmd), strings.Repeat(" ", pad), desc)
}

func helpUsageLines(w io.Writer, lines ...helpLine) {
	for _, line := range lines {
		helpUsageLine(w, line.Cmd, line.Desc)
	}
}

func helpSection(w io.Writer, title string) {
	_, _ = fmt.Fprintf(w, "\n%s\n", boldMsg(w, title))
}

func helpBullet(w io.Writer, text string) {
	_, _ = fmt.Fprintf(w, "  %s %s\n", dimMsg(w, "·"), text)
}

func helpNote(w io.Writer, text string) {
	_, _ = fmt.Fprintln(w, dimMsg(w, text))
}

func helpFlagsHeader(w io.Writer) {
	_, _ = fmt.Fprintf(w, "\n%s\n", boldMsg(w, "Flags:"))
}

func helpExamples(w io.Writer, examples ...string) {
	if len(examples) == 0 {
		return
	}
	helpSection(w, "Examples:")
	for _, ex := range examples {
		_, _ = fmt.Fprintf(w, "  %s\n", infoMsg(w, ex))
	}
}

func usageErr(w io.Writer, text string) {
	text = strings.TrimPrefix(text, "usage: ")
	_, _ = fmt.Fprintf(w, "%s %s\n", errMsg(w, "usage:"), infoMsg(w, text))
}

func bindFlagUsage(fs *flag.FlagSet, title string, intro string, usage []helpLine, examples ...string) {
	w := fs.Output()
	fs.Usage = func() {
		helpTitle(w, title)
		if intro != "" {
			helpText(w, intro)
			_, _ = fmt.Fprintln(w)
		}
		helpUsageHeader(w)
		helpUsageLines(w, usage...)
		if len(examples) > 0 {
			helpExamples(w, examples...)
		}
		helpFlagsHeader(w)
		fs.PrintDefaults()
	}
}

func bindFlagUsageBullets(fs *flag.FlagSet, title string, intro string, bullets []string, usage []helpLine, examples ...string) {
	w := fs.Output()
	fs.Usage = func() {
		helpTitle(w, title)
		if intro != "" {
			helpText(w, intro)
			_, _ = fmt.Fprintln(w)
		}
		for _, b := range bullets {
			helpBullet(w, b)
		}
		if len(bullets) > 0 {
			_, _ = fmt.Fprintln(w)
		}
		helpUsageHeader(w)
		helpUsageLines(w, usage...)
		if len(examples) > 0 {
			helpExamples(w, examples...)
		}
		helpFlagsHeader(w)
		fs.PrintDefaults()
	}
}
