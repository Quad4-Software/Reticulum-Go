// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package cli

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
)

type completionCommand struct {
	Name    string
	Aliases []string
	Run     func([]string, ...Options) int
	NoFlags bool
}

var completionCommands = []completionCommand{
	{Name: CmdStatus, Aliases: []string{"rgostatus"}, Run: RunStatus},
	{Name: CmdID, Aliases: []string{"rgoid"}, Run: RunID},
	{Name: CmdProbe, Aliases: []string{"rgoprobe"}, Run: RunProbe},
	{Name: CmdPath, Aliases: []string{"rgopath"}, Run: RunPath},
	{Name: CmdCP, Aliases: []string{"rgocp"}, Run: RunCP},
	{Name: CmdX, Aliases: []string{"rgox", "rnx"}, Run: RunX},
	{Name: CmdSH, Aliases: []string{"rgosh"}, Run: RunSH},
	{Name: CmdPageserver, Aliases: []string{"rgopageserver"}, Run: RunPageserver},
	{Name: CmdDebug, Run: RunDebug},
	{Name: CmdSlow, Aliases: []string{"rgoslow"}, Run: RunSlow},
	{Name: CmdSelfCheck, Aliases: []string{"rgoselfcheck"}, Run: RunSelfCheck},
	{Name: CmdSpeedtest, Aliases: []string{"rgospeed"}, Run: RunSpeedtest},
	{Name: CmdDump, Aliases: []string{"rgodump"}, Run: RunDump},
	{Name: CmdSnapshot, Aliases: []string{"rgosnap"}, Run: RunSnapshot},
	{Name: CmdZen, Aliases: []string{"rgozen"}, Run: RunZen},
	{Name: CmdGit, Aliases: []string{"rgogit"}, Run: RunGit},
	{Name: CmdDaemon},
	{Name: CmdCompletion, NoFlags: true},
}

type completionFlag struct {
	Name  string
	Usage string
	Bool  bool
	Path  bool
}

type completionSpec struct {
	Command completionCommand
	Flags   []completionFlag
}

var completionCaptureState struct {
	mu       sync.Mutex
	specMu   sync.Mutex
	active   bool
	captured *flag.FlagSet
}

func noteCompletionFlagSet(fs *flag.FlagSet) {
	completionCaptureState.mu.Lock()
	defer completionCaptureState.mu.Unlock()
	if completionCaptureState.active {
		completionCaptureState.captured = fs
	}
}

func captureCompletionFlags(cmd completionCommand) (*flag.FlagSet, error) {
	if cmd.NoFlags || cmd.Run == nil {
		return nil, nil
	}
	completionCaptureState.mu.Lock()
	completionCaptureState.active = true
	completionCaptureState.captured = nil
	completionCaptureState.mu.Unlock()

	var out bytes.Buffer
	_ = cmd.Run([]string{"--completion-capture-unknown-flag"}, Options{
		Stdout:      &out,
		Stderr:      &out,
		Argv0:       "reticulum-go",
		VersionLine: "completion-capture",
	})

	completionCaptureState.mu.Lock()
	defer completionCaptureState.mu.Unlock()
	completionCaptureState.active = false
	fs := completionCaptureState.captured
	completionCaptureState.captured = nil
	if fs == nil {
		return nil, fmt.Errorf("no flag set registered for %s", cmd.Name)
	}
	return fs, nil
}

func completionSpecs() ([]completionSpec, error) {
	completionCaptureState.specMu.Lock()
	defer completionCaptureState.specMu.Unlock()
	specs := make([]completionSpec, 0, len(completionCommands))
	for _, cmd := range completionCommands {
		spec := completionSpec{Command: cmd}
		var fs *flag.FlagSet
		var err error
		if cmd.Name == CmdDaemon {
			fs, _ = NewDaemonFlagSet()
		} else {
			fs, err = captureCompletionFlags(cmd)
			if err != nil {
				return nil, err
			}
		}
		if fs != nil {
			fs.VisitAll(func(f *flag.Flag) {
				_, isBool := f.Value.(interface{ IsBoolFlag() bool })
				spec.Flags = append(spec.Flags, completionFlag{
					Name:  f.Name,
					Usage: f.Usage,
					Bool:  isBool,
					Path:  completionTakesPath(f),
				})
			})
			resolveCompletionPathAliases(spec.Flags)
		}
		specs = append(specs, spec)
	}
	return specs, nil
}

func completionTakesPath(f *flag.Flag) bool {
	usage := strings.ToLower(f.Usage)
	if strings.Contains(usage, "service name") {
		return false
	}
	if strings.Contains(usage, "path to") || strings.Contains(usage, " file") || strings.Contains(usage, "directory") || strings.HasSuffix(usage, " path") {
		return true
	}
	return f.Name == "w" && (strings.Contains(usage, "write") || strings.Contains(usage, "output"))
}

func resolveCompletionPathAliases(flags []completionFlag) {
	for i := range flags {
		usage := strings.ToLower(strings.TrimSpace(flags[i].Usage))
		if !strings.HasPrefix(usage, "same as -") {
			continue
		}
		fields := strings.Fields(strings.TrimPrefix(usage, "same as -"))
		if len(fields) == 0 {
			continue
		}
		target := strings.Trim(fields[0], ",")
		for _, other := range flags {
			if other.Name == target {
				flags[i].Path = other.Path
				break
			}
		}
	}
}

// RunCompletion implements reticulum-go completion.
func RunCompletion(args []string, opt ...Options) int {
	stdout, stderr := cliIO(opt)
	if len(args) != 1 {
		usageErr(stderr, "reticulum-go completion bash|zsh|fish")
		return 2
	}
	specs, err := completionSpecs()
	if err != nil {
		diagErr(stderr, "completion", err)
		return 1
	}
	var out string
	switch args[0] {
	case "bash":
		out = generateBashCompletion(specs)
	case "zsh":
		out = generateZshCompletion(specs)
	case "fish":
		out = generateFishCompletion(specs)
	default:
		usageErr(stderr, "reticulum-go completion bash|zsh|fish")
		return 2
	}
	_, _ = io.WriteString(stdout, out)
	return 0
}

func completionNames() []string {
	names := make([]string, 0, len(completionCommands))
	for _, cmd := range completionCommands {
		names = append(names, cmd.Name)
	}
	sort.Strings(names)
	return names
}

func completionBinaries(specs []completionSpec) []string {
	seen := map[string]bool{"reticulum-go": true}
	out := []string{"reticulum-go"}
	for _, spec := range specs {
		for _, alias := range spec.Command.Aliases {
			if !seen[alias] {
				seen[alias] = true
				out = append(out, alias)
			}
		}
	}
	sort.Strings(out[1:])
	return out
}

func specByName(specs []completionSpec, name string) *completionSpec {
	for i := range specs {
		if specs[i].Command.Name == name {
			return &specs[i]
		}
	}
	return nil
}

func bashQuoteWords(words []string) string {
	return strings.Join(words, " ")
}

func generateBashCompletion(specs []completionSpec) string {
	var b strings.Builder
	b.WriteString("# bash completion for reticulum-go. Generated by reticulum-go completion bash.\n")
	b.WriteString("_reticulum_go_flags() {\n    case \"$1\" in\n")
	for _, spec := range specs {
		if len(spec.Flags) == 0 {
			continue
		}
		flags := make([]string, 0, len(spec.Flags))
		for _, f := range spec.Flags {
			flags = append(flags, "-"+f.Name)
		}
		fmt.Fprintf(&b, "        %s) printf '%%s\\n' %s ;;\n", spec.Command.Name, bashQuoteWords(flags))
	}
	b.WriteString("    esac\n}\n\n")
	b.WriteString("_reticulum_go_is_path_flag() {\n    case \"$1:$2\" in\n")
	for _, spec := range specs {
		for _, f := range spec.Flags {
			if f.Path {
				fmt.Fprintf(&b, "        %s:-%s) return 0 ;;\n", spec.Command.Name, f.Name)
			}
		}
	}
	b.WriteString("    esac\n    return 1\n}\n\n")
	b.WriteString(`_reticulum_go_complete() {
    local cur prev cmd
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev=""
    if (( COMP_CWORD > 0 )); then
        prev="${COMP_WORDS[COMP_CWORD-1]}"
    fi

    local base="${COMP_WORDS[0]##*/}"
    cmd=""
    case "$base" in
`)
	for _, spec := range specs {
		if len(spec.Command.Aliases) == 0 {
			continue
		}
		fmt.Fprintf(&b, "        %s) cmd=%q ;;\n", strings.Join(spec.Command.Aliases, "|"), spec.Command.Name)
	}
	fmt.Fprintf(&b, `    esac

    if [[ -z "$cmd" ]]; then
        if (( COMP_CWORD == 1 )); then
            COMPREPLY=( $(compgen -W %q -- "$cur") )
            return 0
        fi
        cmd="${COMP_WORDS[1]}"
    fi

    if [[ "$cmd" == "completion" ]]; then
        COMPREPLY=( $(compgen -W "bash zsh fish" -- "$cur") )
        return 0
    fi

    if _reticulum_go_is_path_flag "$cmd" "$prev"; then
        COMPREPLY=( $(compgen -f -- "$cur") )
        return 0
    fi
    if [[ "$cur" == -* ]]; then
        local flags
        flags="$(_reticulum_go_flags "$cmd")"
        COMPREPLY=( $(compgen -W "$flags" -- "$cur") )
        return 0
    fi
    COMPREPLY=( $(compgen -f -- "$cur") )
    return 0
}
`, strings.Join(completionNames(), " "))
	fmt.Fprintf(&b, "complete -F _reticulum_go_complete %s\n", strings.Join(completionBinaries(specs), " "))
	return b.String()
}

func zshQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `'\''`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, `[`, `\[`)
	s = strings.ReplaceAll(s, `]`, `\]`)
	s = strings.ReplaceAll(s, `$`, `\$`)
	return s
}

func generateZshCompletion(specs []completionSpec) string {
	var b strings.Builder
	b.WriteString("#compdef ")
	b.WriteString(strings.Join(completionBinaries(specs), " "))
	b.WriteString("\n# zsh completion for reticulum-go. Generated by reticulum-go completion zsh.\n")
	b.WriteString("_reticulum_go() {\n  local cmd\n  case $service in\n")
	for _, spec := range specs {
		if len(spec.Command.Aliases) == 0 {
			continue
		}
		fmt.Fprintf(&b, "    %s) cmd=%q ;;\n", strings.Join(spec.Command.Aliases, "|"), spec.Command.Name)
	}
	b.WriteString("  esac\n  if [[ -z $cmd ]]; then\n    if (( CURRENT == 2 )); then\n      local -a commands\n      commands=(\n")
	for _, name := range completionNames() {
		fmt.Fprintf(&b, "        %q\n", name)
	}
	b.WriteString("      )\n      _describe 'command' commands\n      return\n    fi\n    cmd=$words[2]\n  fi\n  if [[ $cmd == completion ]]; then\n    local -a shells\n    shells=(bash zsh fish)\n    _describe 'shell' shells\n    return\n  fi\n  case $cmd in\n")
	for _, spec := range specs {
		if len(spec.Flags) == 0 {
			continue
		}
		fmt.Fprintf(&b, "    %s)\n      _arguments \\\n", spec.Command.Name)
		for i, f := range spec.Flags {
			lineEnd := " \\\n"
			if i == len(spec.Flags)-1 {
				lineEnd = "\n"
			}
			action := ""
			if !f.Bool {
				action = ":value:"
				if f.Path {
					action = ":path:_files"
				}
			}
			fmt.Fprintf(&b, "        '-%s[%s]%s'%s", f.Name, zshQuote(f.Usage), action, lineEnd)
		}
		b.WriteString("      ;;\n")
	}
	b.WriteString("  esac\n}\n_reticulum_go \"$@\"\n")
	return b.String()
}

func fishQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	return s
}

func generateFishCompletion(specs []completionSpec) string {
	var b strings.Builder
	b.WriteString("# fish completion for reticulum-go. Generated by reticulum-go completion fish.\n")
	fmt.Fprintf(&b, "complete -c reticulum-go -n '__fish_use_subcommand' -a '%s'\n", strings.Join(completionNames(), " "))
	b.WriteString("complete -c reticulum-go -n '__fish_seen_subcommand_from completion' -a 'bash zsh fish'\n")
	for _, spec := range specs {
		commands := append([]string(nil), spec.Command.Aliases...)
		for _, f := range spec.Flags {
			short := ""
			long := ""
			if len(f.Name) == 1 {
				short = " -s " + f.Name
			} else {
				long = " -l " + f.Name
			}
			req := ""
			if !f.Bool {
				req = " -r"
			}
			path := ""
			if f.Path {
				path = " -F"
			}
			desc := ""
			if f.Usage != "" {
				desc = " -d '" + fishQuote(f.Usage) + "'"
			}
			fmt.Fprintf(&b, "complete -c reticulum-go -n '__fish_seen_subcommand_from %s'%s%s%s%s%s\n", spec.Command.Name, short, long, req, path, desc)
			for _, alias := range commands {
				fmt.Fprintf(&b, "complete -c %s%s%s%s%s%s\n", alias, short, long, req, path, desc)
			}
		}
	}
	return b.String()
}
