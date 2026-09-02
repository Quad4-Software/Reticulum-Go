// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/link"
	"quad4/reticulum-go/pkg/node"
	"quad4/reticulum-go/pkg/rnsutil"
	"quad4/reticulum-go/pkg/term"
	"quad4/reticulum-go/pkg/transport"
)

// RunX implements reticulum-go x / rnx / rgox remote execution.
func RunX(args []string, opt ...Options) int {
	stdout, stderr := cliIO(opt)
	fs := flag.NewFlagSet("rgox", flag.ContinueOnError)
	fs.SetOutput(stderr)

	configDir := fs.String("config", "", "path to config directory")
	identityPath := fs.String("i", "", "path to identity file")
	listenMode := fs.Bool("l", false, "listen for incoming commands")
	printID := fs.Bool("p", false, "print identity and destination hash then exit")
	interactive := fs.Bool("x", false, "interactive mode")
	noAnnounce := fs.Bool("b", false, "don't announce at start (listen)")
	noAuth := fs.Bool("n", false, "accept commands from anyone (listen)")
	noID := fs.Bool("N", false, "don't identify to listener")
	detailed := fs.Bool("d", false, "show detailed result summary")
	mirror := fs.Bool("m", false, "mirror remote exit code")
	timeoutSec := fs.Float64("w", 0, "path/link/command timeout seconds (0 = adaptive path and link)")
	resultTimeoutSec := fs.Float64("W", 0, "max seconds to receive result (0 = unlimited)")
	stdinStr := fs.String("stdin", "", "data passed to remote stdin")
	stdoutLimit := fs.Int("stdout", -1, "max stdout bytes returned (-1 = unlimited)")
	stderrLimit := fs.Int("stderr", -1, "max stderr bytes returned (-1 = unlimited)")
	jsonOut := fs.Bool("json", false, "emit JSON result")
	var allowed flagStringList
	fs.Var(&allowed, "a", "allowed identity hash (repeatable, listen)")
	bindFlagUsage(fs, "rgox - remote command execution (rnx)",
		"Execute commands on remote nodes or listen for incoming requests.",
		[]helpLine{
			{Cmd: "rgox [flags] <destination_hash> <command>"},
			{Cmd: "rgox -x [flags] <destination_hash>"},
			{Cmd: "rgox -l [flags]"},
			{Cmd: "reticulum-go x [flags] ..."},
		},
		"rgox <dest_hash> uname -a",
		"rgox -x <dest_hash>",
		"rgox -l",
	)

	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := rnsutil.LoadConfigDir(*configDir)
	if err != nil {
		diagErr(stderr, "config", err)
		return 1
	}

	idPath := *identityPath
	if idPath == "" {
		idPath = rnsutil.RNXIdentityPath(rnsutil.StorageDir(cfg))
	}
	id, err := rnsutil.PrepareRNXIdentity(idPath)
	if err != nil {
		diagErr(stderr, "identity", err)
		return 2
	}

	timeout := max(time.Duration(*timeoutSec*float64(time.Second)), 0)

	if *printID {
		destHash := destination.Hash(id, rnsutil.RNXAppName, rnsutil.RNXAspect)
		fmt.Fprintf(stdout, "%s : %s\n", infoMsg(stdout, "Identity"), rnsutil.PrettyHex(id.Hash()))
		fmt.Fprintf(stdout, "%s : %s\n", infoMsg(stdout, "Listening on"), rnsutil.PrettyHex(destHash))
		return 0
	}

	n, err := node.New(cfg)
	if err != nil {
		diagErr(stderr, "node", err)
		return 1
	}
	if err := n.Start(); err != nil {
		diagErr(stderr, "start", err)
		return 1
	}
	defer n.Stop()
	tr := n.Transport()

	if *listenMode {
		return runXListen(tr, id, xListenOpts{
			allowAll:   *noAuth,
			noAnnounce: *noAnnounce,
			allowedCLI: allowed,
			stderr:     stderr,
		})
	}

	if fs.NArg() < 1 {
		usageErr(stderr, "rgox [flags] <destination_hash> [command]")
		usageErr(stderr, "rgox -l [flags]")
		usageErr(stderr, "rgox -x [flags] <destination_hash>")
		return 2
	}
	destHash, err := rnsutil.ParseDestHash(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(stderr, errMsg(stderr, err.Error()))
		return rnsutil.ExitRNXInvalidDest
	}

	var stdoutL, stderrL *int
	if *stdoutLimit >= 0 {
		stdoutL = stdoutLimit
	}
	if *stderrLimit >= 0 {
		stderrL = stderrLimit
	}
	var stdin []byte
	if *stdinStr != "" {
		stdin = []byte(*stdinStr)
	}
	var timeoutPtr *float64
	if timeout > 0 {
		s := timeout.Seconds()
		timeoutPtr = &s
	}

	execOpts := xExecOpts{
		timeout:       timeout,
		resultTimeout: time.Duration(*resultTimeoutSec * float64(time.Second)),
		noid:          *noID,
		detailed:      *detailed,
		mirror:        *mirror,
		jsonOut:       *jsonOut,
		stdout:        stdout,
		stderr:        stderr,
		req: rnsutil.RNXRequest{
			TimeoutSec:  timeoutPtr,
			StdoutLimit: stdoutL,
			StderrLimit: stderrL,
			Stdin:       stdin,
		},
	}

	if *interactive {
		return runXInteractive(tr, id, destHash, &execOpts)
	}
	if fs.NArg() < 2 {
		usageErr(stderr, "rgox [flags] <destination_hash> <command>")
		return 2
	}
	execOpts.req.Command = strings.Join(fs.Args()[1:], " ")
	return runXExecute(tr, id, destHash, &execOpts)
}

type xListenOpts struct {
	allowAll   bool
	noAnnounce bool
	allowedCLI []string
	stderr     io.Writer
}

func runXListen(tr *transport.Transport, id *identity.Identity, opts xListenOpts) int {
	stderr := opts.stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	allowed, err := rnsutil.LoadRNXAllowedIdentities(opts.allowedCLI)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	if !opts.allowAll && len(allowed) == 0 {
		fmt.Fprintln(stderr, warnMsg(stderr, "Warning: No allowed identities configured, rgox will not accept any commands!"))
	}

	dest, err := destination.New(id, destination.In, destination.Single, rnsutil.RNXAppName, tr, rnsutil.RNXAspect)
	if err != nil {
		diagErr(stderr, "destination", err)
		return 1
	}
	dest.AcceptsLinks(true)

	dest.SetLinkEstablishedCallback(func(lnk any) {
		l, ok := lnk.(*link.Link)
		if !ok || l == nil {
			return
		}
		fmt.Fprintln(stderr, infoMsg(stderr, "Command link established"))
		l.SetRemoteIdentifiedCallback(func(lnk *link.Link, remote *identity.Identity) {
			if opts.allowAll {
				return
			}
			if remote == nil || !rnsutil.AllowedContains(allowed, remote.Hash()) {
				fmt.Fprintln(stderr, warnMsg(stderr, "Identity not allowed, tearing down link"))
				lnk.Teardown()
			}
		})
	})

	allowMode := byte(destination.AllowList)
	allowList := allowed
	if opts.allowAll {
		allowMode = destination.AllowAll
		allowList = nil
	}
	_ = dest.RegisterRequestHandlerAny(rnsutil.RNXCommandPath, func(_ string, data []byte, _ []byte, _ []byte, remote *identity.Identity, _ int64) any {
		req, err := rnsutil.ParseRNXRequestPayload(data)
		if err != nil {
			return rnsutil.PackRNXResult(rnsutil.RNXResult{StartedAt: float64(time.Now().Unix())})
		}
		who := "unknown"
		if remote != nil {
			who = rnsutil.PrettyHex(remote.Hash())
		}
		fmt.Fprintf(stderr, "Executing command [%s] for %s\n", req.Command, who)
		res := rnsutil.ExecuteRNXCommandLocally(req)
		return rnsutil.PackRNXResult(res)
	}, allowMode, allowList)

	fmt.Fprintln(stderr, okMsg(stderr, fmt.Sprintf("rgox listening for commands on %s", rnsutil.PrettyHex(dest.GetHash()))))
	if !opts.noAnnounce {
		_ = dest.Announce(false, nil, nil)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	return 0
}

type xExecOpts struct {
	timeout       time.Duration
	resultTimeout time.Duration
	noid          bool
	detailed      bool
	mirror        bool
	jsonOut       bool
	stdout        io.Writer
	stderr        io.Writer
	req           rnsutil.RNXRequest
	keepLink      *link.Link
	didIdentify   bool
}

func runXInteractive(tr *transport.Transport, id *identity.Identity, destHash []byte, opts *xExecOpts) int {
	stdout, stderr := opts.stdout, opts.stderr
	ctx, cancel := rnsutil.CLIWaitContext(opts.timeout)
	l, err := rnsutil.EstablishRNXLink(ctx, tr, destHash)
	cancel()
	if err != nil {
		if strings.Contains(err.Error(), "path:") {
			fmt.Fprintln(stdout, errMsg(stdout, "Path not found"))
			return rnsutil.ExitRNXPathNotFound
		}
		fmt.Fprintln(stdout, errMsg(stdout, "Could not establish link with "+rnsutil.PrettyHex(destHash)))
		return rnsutil.ExitRNXLinkFailed
	}
	defer l.Teardown()
	opts.keepLink = l

	sc := bufio.NewScanner(os.Stdin)
	for {
		fmt.Fprint(stderr, "> ")
		if !sc.Scan() {
			break
		}
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if lower == "exit" || lower == "quit" {
			return 0
		}
		if lower == "clear" {
			fmt.Fprint(stdout, term.ClearScreenW(stdout))
			continue
		}
		opts.req.Command = line
		_ = runXExecute(tr, id, destHash, opts)
	}
	return 0
}

func runXExecute(tr *transport.Transport, id *identity.Identity, destHash []byte, opts *xExecOpts) int {
	stdout, stderr := opts.stdout, opts.stderr
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}

	var l *link.Link
	var err error
	reuse := opts.keepLink != nil && opts.keepLink.IsActive()
	if reuse {
		l = opts.keepLink
	} else {
		fmt.Fprintln(stdout, infoMsg(stdout, fmt.Sprintf("Path to %s requested", rnsutil.PrettyHex(destHash))))
		ctx, cancel := rnsutil.CLIWaitContext(opts.timeout)
		l, err = rnsutil.EstablishRNXLink(ctx, tr, destHash)
		cancel()
		if err != nil {
			if strings.Contains(err.Error(), "path:") {
				fmt.Fprintln(stdout, errMsg(stdout, "Path not found"))
				return rnsutil.ExitRNXPathNotFound
			}
			fmt.Fprintln(stdout, errMsg(stdout, "Could not establish link with "+rnsutil.PrettyHex(destHash)))
			return rnsutil.ExitRNXLinkFailed
		}
		if opts.keepLink != nil {
			opts.keepLink = l
		} else {
			defer l.Teardown()
		}
	}

	if !opts.noid && !opts.didIdentify {
		if err := l.Identify(id); err != nil {
			diagErr(stderr, "identify", err)
			return rnsutil.ExitRNXRequestFailed
		}
		opts.didIdentify = true
	}

	rexec := rnsutil.RNXRequestTimeout(opts.timeout, l.RTT())
	receipt, err := l.Request(rnsutil.RNXCommandPath, rnsutil.PackRNXRequest(opts.req), rexec)
	if err != nil {
		fmt.Fprintln(stdout, errMsg(stdout, "Could not request remote execution"))
		return rnsutil.ExitRNXRequestFailed
	}

	waitCtx, cancel := context.WithTimeout(context.Background(), rexec+opts.timeout)
	if opts.resultTimeout > 0 {
		waitCtx, cancel = context.WithTimeout(context.Background(), opts.resultTimeout+rexec)
	}
	defer cancel()

	if err := rnsutil.WaitRequest(waitCtx, receipt); err != nil {
		if receipt.GetStatus() == link.StatusFailed {
			fmt.Fprintln(stdout, errMsg(stdout, "No result was received"))
			return rnsutil.ExitRNXNoResult
		}
		fmt.Fprintln(stdout, errMsg(stdout, "Receiving result failed"))
		return rnsutil.ExitRNXReceiveFailed
	}
	if receipt.GetStatus() == link.StatusFailed {
		fmt.Fprintln(stdout, errMsg(stdout, "No result was received"))
		return rnsutil.ExitRNXNoResult
	}

	raw := receipt.GetResponseValue()
	if raw == nil {
		fmt.Fprintln(stdout, errMsg(stdout, "No response"))
		return rnsutil.ExitRNXNoResponse
	}
	res, err := rnsutil.ParseRNXResult(raw)
	if err != nil {
		fmt.Fprintln(stdout, errMsg(stdout, "Received invalid result"))
		return rnsutil.ExitRNXInvalidResult
	}

	if opts.jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		payload := map[string]any{
			"executed":     res.Executed,
			"returncode":   res.ReturnCode,
			"stdout":       string(res.Stdout),
			"stderr":       string(res.Stderr),
			"stdout_total": res.StdoutTotal,
			"stderr_total": res.StderrTotal,
			"started_at":   res.StartedAt,
			"concluded_at": res.ConcludedAt,
			"destination":  fmt.Sprintf("%x", destHash),
			"command":      opts.req.Command,
		}
		_ = enc.Encode(payload)
	} else if res.Executed {
		if len(res.Stdout) > 0 {
			fmt.Fprint(stdout, string(res.Stdout))
		}
		if len(res.Stderr) > 0 {
			fmt.Fprint(stderr, string(res.Stderr))
		}
		if opts.detailed {
			fmt.Fprintln(stdout, "\n--- End of remote output, rgox done ---")
			if res.ConcludedAt != nil {
				fmt.Fprintf(stdout, "Remote command execution took %.3f seconds\n", *res.ConcludedAt-res.StartedAt)
			}
			if res.Stdout != nil {
				tstr := ""
				if len(res.Stdout) < res.StdoutTotal {
					tstr = fmt.Sprintf(", %d bytes displayed", len(res.Stdout))
				}
				fmt.Fprintf(stdout, "Remote wrote %d bytes to stdout%s\n", res.StdoutTotal, tstr)
			}
			if res.Stderr != nil {
				tstr := ""
				if len(res.Stderr) < res.StderrTotal {
					tstr = fmt.Sprintf(", %d bytes displayed", len(res.Stderr))
				}
				fmt.Fprintf(stdout, "Remote wrote %d bytes to stderr%s\n", res.StderrTotal, tstr)
			}
		} else if (opts.req.StdoutLimit != nil && *opts.req.StdoutLimit != 0 && len(res.Stdout) < res.StdoutTotal) ||
			(opts.req.StderrLimit != nil && *opts.req.StderrLimit != 0 && len(res.Stderr) < res.StderrTotal) {
			fmt.Fprintln(stdout, "\nOutput truncated before being returned:")
			if len(res.Stdout) > 0 && len(res.Stdout) < res.StdoutTotal {
				fmt.Fprintf(stdout, "  stdout truncated to %d bytes\n", len(res.Stdout))
			}
			if len(res.Stderr) > 0 && len(res.Stderr) < res.StderrTotal {
				fmt.Fprintf(stdout, "  stderr truncated to %d bytes\n", len(res.Stderr))
			}
		}
	} else {
		fmt.Fprintln(stdout, errMsg(stdout, "Remote could not execute command"))
		return rnsutil.ExitRNXRemoteExecFail
	}

	if opts.mirror {
		if res.ReturnCode == nil {
			return rnsutil.ExitRNXMirrorNilCode
		}
		return *res.ReturnCode
	}
	return 0
}
