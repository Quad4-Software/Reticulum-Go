// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/term"

	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/link"
	"quad4/reticulum-go/pkg/node"
	"quad4/reticulum-go/pkg/rgosh"
	"quad4/reticulum-go/pkg/rnsutil"
	"quad4/reticulum-go/pkg/transport"
)

// RunSH implements reticulum-go sh / rgosh remote shell.
func RunSH(args []string, opt ...Options) int {
	stdout, stderr := cliIO(opt)
	fs := flag.NewFlagSet("rgosh", flag.ContinueOnError)
	fs.SetOutput(stderr)

	configDir := fs.String("config", "", "path to config directory")
	identityPath := fs.String("i", "", "path to identity file")
	service := fs.String("s", "", "service name (identity file suffix)")
	listenMode := fs.Bool("l", false, "listen for incoming shell sessions")
	printID := fs.Bool("p", false, "print identity and destination hash then exit")
	noAnnounce := fs.Bool("b", false, "don't announce at start (listen)")
	noAuth := fs.Bool("n", false, "accept sessions from anyone (listen)")
	noID := fs.Bool("N", false, "don't identify to listener")
	mirror := fs.Bool("m", false, "mirror remote exit code")
	forced := fs.Bool("C", false, "forbid remote cmdline (forced default command)")
	compat := fs.Bool("compat", false, "speak Python rnsh wire protocol")
	lineMode := fs.Bool("line", false, "force line-buffered stdin")
	rawMode := fs.Bool("raw", false, "disable stdin coalescing")
	verbose := fs.Bool("v", false, "enable reticulum debug logs on stderr")
	timeoutSec := fs.Float64("w", 15, "path/link timeout seconds")
	var allowed flagStringList
	fs.Var(&allowed, "a", "allowed identity hash (repeatable, listen)")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	// Interactive shells share stderr with the remote TTY. Keep transport
	// chatter off unless -v is set.
	if !*verbose {
		debug.SetDebugLevel(debug.DebugCritical)
	}

	cfg, err := rnsutil.LoadConfigDir(*configDir)
	if err != nil {
		fmt.Fprintf(stderr, "config: %v\n", err)
		return 1
	}

	appName := rnsutil.RgoshAppNameForMode(*compat)
	idPath := *identityPath
	if idPath == "" {
		base := rnsutil.StorageDir(cfg)
		if *compat {
			idPath = rnsutil.RnshIdentityPath(base)
		} else {
			idPath = rnsutil.RgoshIdentityPath(base)
		}
		if *service != "" {
			idPath = idPath + "." + *service
		}
	}
	id, err := rnsutil.PrepareRgoshIdentity(idPath)
	if err != nil {
		fmt.Fprintf(stderr, "identity: %v\n", err)
		return 2
	}

	timeout := time.Duration(*timeoutSec * float64(time.Second))
	if timeout <= 0 {
		timeout = rnsutil.DefaultRgoshTimeout
	}

	if *printID {
		destHash := destination.Hash(id, appName)
		fmt.Fprintf(stdout, "%s : %s\n", infoMsg(stdout, "Identity"), rnsutil.PrettyHex(id.Hash()))
		fmt.Fprintf(stdout, "%s : %s\n", infoMsg(stdout, "Listening on"), rnsutil.PrettyHex(destHash))
		return 0
	}

	n, err := node.New(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "node: %v\n", err)
		return 1
	}
	if err := n.Start(); err != nil {
		fmt.Fprintf(stderr, "start: %v\n", err)
		return 1
	}
	defer n.Stop()
	tr := n.Transport()

	defaultCmd := rgosh.DefaultShell()
	if fs.NArg() > 0 && *listenMode {
		defaultCmd = append([]string(nil), fs.Args()...)
	}

	if *listenMode {
		return runSHListen(tr, id, shListenOpts{
			allowAll:   *noAuth,
			noAnnounce: *noAnnounce,
			allowedCLI: allowed,
			compat:     *compat,
			forced:     *forced,
			defaultCmd: defaultCmd,
			appName:    appName,
			stderr:     stderr,
		})
	}

	if fs.NArg() < 1 {
		fmt.Fprintln(stderr, "usage: rgosh [flags] <destination_hash> [command...]")
		fmt.Fprintln(stderr, "       rgosh -l [flags] [command...]")
		return 2
	}
	destHash, err := rnsutil.ParseDestHash(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(stderr, errMsg(stderr, err.Error()))
		return 2
	}
	cmdline := fs.Args()[1:]
	return runSHClient(tr, id, destHash, shClientOpts{
		timeout:  timeout,
		noid:     *noID,
		mirror:   *mirror,
		compat:   *compat,
		lineMode: *lineMode,
		rawMode:  *rawMode,
		cmdline:  cmdline,
		appName:  appName,
		stdout:   stdout,
		stderr:   stderr,
	})
}

type shListenOpts struct {
	allowAll   bool
	noAnnounce bool
	allowedCLI []string
	compat     bool
	forced     bool
	defaultCmd []string
	appName    string
	stderr     io.Writer
}

func runSHListen(tr *transport.Transport, id *identity.Identity, opts shListenOpts) int {
	stderr := opts.stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	allowed, err := rnsutil.LoadRgoshAllowedIdentities(opts.allowedCLI)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	if opts.allowAll {
		fmt.Fprintln(stderr, warnMsg(stderr, "Warning: accepting shell sessions from anyone (-n)"))
	} else if len(allowed) == 0 {
		fmt.Fprintln(stderr, warnMsg(stderr, "Warning: No allowed identities configured, rgosh will not accept sessions"))
	}

	baseCfg := rgosh.Config{
		Compat:        opts.compat,
		AllowAll:      opts.allowAll,
		Allowed:       allowed,
		DefaultCmd:    opts.defaultCmd,
		ForcedCommand: opts.forced,
		Listener:      true,
	}

	dest, err := destination.New(id, destination.In, destination.Single, opts.appName, tr)
	if err != nil {
		fmt.Fprintf(stderr, "destination: %v\n", err)
		return 1
	}
	dest.AcceptsLinks(true)

	dest.SetLinkEstablishedCallback(func(lnk any) {
		l, ok := lnk.(*link.Link)
		if !ok || l == nil {
			return
		}
		fmt.Fprintln(stderr, infoMsg(stderr, "Shell link established"))
		go serveSHLink(l, baseCfg, stderr)
	})

	fmt.Fprintln(stderr, okMsg(stderr, fmt.Sprintf("rgosh listening on %s", rnsutil.PrettyHex(dest.GetHash()))))
	if !opts.noAnnounce {
		_ = dest.Announce(false, nil, nil)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	return 0
}

func serveSHLink(l *link.Link, baseCfg rgosh.Config, stderr io.Writer) {
	ch := l.GetChannel()
	if baseCfg.Compat {
		_ = rgosh.RegisterCompat(ch)
	} else {
		_ = rgosh.RegisterNative(ch)
	}
	sess := rgosh.NewSession(baseCfg, rgosh.ChannelSender{Ch: ch})
	sess.StartProcess = rgosh.StartLocalProcess
	sess.OnTeardown = func() { l.Teardown() }
	sess.OnAuthDenied = func(reason string) {
		fmt.Fprintln(stderr, warnMsg(stderr, "Identity not allowed: "+reason))
	}

	ch.AddMessageHandler(func(msg rgosh.Message) bool {
		_ = sess.HandleMessage(msg)
		return true
	})

	l.SetRemoteIdentifiedCallback(func(lnk *link.Link, remote *identity.Identity) {
		if remote == nil {
			return
		}
		if !sess.SetRemoteIdentity(remote.Hash()) {
			lnk.Teardown()
		}
	})
	if remote := l.GetRemoteIdentity(); remote != nil {
		if !sess.SetRemoteIdentity(remote.Hash()) {
			l.Teardown()
		}
	}
}

type shClientOpts struct {
	timeout  time.Duration
	noid     bool
	mirror   bool
	compat   bool
	lineMode bool
	rawMode  bool
	cmdline  []string
	appName  string
	stdout   io.Writer
	stderr   io.Writer
}

func runSHClient(tr *transport.Transport, id *identity.Identity, destHash []byte, opts shClientOpts) int {
	stdout, stderr := opts.stdout, opts.stderr
	ctx, cancel := context.WithTimeout(context.Background(), opts.timeout)
	defer cancel()

	l, err := rnsutil.EstablishRgoshLink(ctx, tr, destHash, opts.appName)
	if err != nil {
		fmt.Fprintln(stderr, errMsg(stderr, err.Error()))
		return 1
	}
	defer l.Teardown()

	ch := l.GetChannel()
	if opts.compat {
		_ = rgosh.RegisterCompat(ch)
	} else {
		_ = rgosh.RegisterNative(ch)
	}

	exitCh := make(chan int, 1)
	done := make(chan struct{})
	var doneOnce sync.Once
	closeDone := func() { doneOnce.Do(func() { close(done) }) }
	var endMu sync.Mutex
	var endMsg string
	setEndMsg := func(msg string) {
		endMu.Lock()
		if endMsg == "" && msg != "" {
			endMsg = msg
		}
		endMu.Unlock()
	}

	sess := rgosh.NewSession(rgosh.Config{
		Compat:   opts.compat,
		Listener: false,
	}, rgosh.ChannelSender{Ch: ch})
	var exited atomic.Bool
	var userInt atomic.Bool
	var stdinOff atomic.Bool
	sess.OnExit = func(code int) {
		exited.Store(true)
		select {
		case exitCh <- code:
		default:
		}
		closeDone()
	}
	sess.OnTeardown = closeDone
	sess.OnAuthDenied = func(reason string) {
		setEndMsg("auth denied: " + reason)
		closeDone()
	}
	sess.OnError = func(msg string, fatal bool) {
		setEndMsg(msg)
		if fatal {
			closeDone()
		}
	}
	sess.OnStdout = func(data []byte) { _, _ = stdout.Write(data) }
	sess.OnStderr = func(data []byte) { _, _ = stderr.Write(data) }

	ch.AddMessageHandler(func(msg rgosh.Message) bool {
		_ = sess.HandleMessage(msg)
		return true
	})

	// Register handlers before Identify so peer Error/Version are not dropped.
	l.SetLinkClosedCallback(func(*link.Link) {
		if !exited.Load() && !userInt.Load() {
			setEndMsg("link closed")
		}
		closeDone()
	})

	if !opts.noid {
		if err := l.Identify(id); err != nil {
			fmt.Fprintln(stderr, errMsg(stderr, "identify: "+err.Error()))
			return 1
		}
	}

	if !opts.compat && !opts.noid {
		authDeadline := time.After(2 * time.Second)
		for !sess.Authed() {
			select {
			case <-authDeadline:
				goto afterAuth
			case <-done:
				return 1
			case <-time.After(20 * time.Millisecond):
			}
		}
	}
afterAuth:

	if err := sess.SendVersion(); err != nil {
		fmt.Fprintln(stderr, errMsg(stderr, err.Error()))
		return 1
	}

	deadline := time.After(opts.timeout)
	lastVers := time.Now()
	for sess.State() == rgosh.StateWaitVers {
		select {
		case <-deadline:
			fmt.Fprintln(stderr, errMsg(stderr, "timeout waiting for version"))
			return 1
		case <-done:
			return 1
		case <-time.After(50 * time.Millisecond):
			// Rare resend only: peer moves to WAIT_CMD after the first Version,
			// so a fast retry is a protocol error (LSSTATE_WAIT_CMD).
			if sess.State() == rgosh.StateWaitVers && time.Since(lastVers) >= 2*time.Second {
				_ = sess.SendVersion()
				lastVers = time.Now()
			}
		}
	}
	if st := sess.State(); st != rgosh.StateWaitCmd && st != rgosh.StateRunning {
		fmt.Fprintln(stderr, errMsg(stderr, "session ended before exec ("+st.String()+")"))
		return 1
	}

	pipeStdin := !isTTY(os.Stdin)
	pipeStdout := !isTTY(os.Stdout)
	pipeStderr := !isTTY(os.Stderr)
	interactive := !pipeStdin && !pipeStdout
	termName := os.Getenv("TERM")
	rows, cols := ttySize()
	cmdline := opts.cmdline
	if len(cmdline) == 0 {
		cmdline = nil
	}

	var restoreTTY func()
	if interactive {
		fd := fileFD(os.Stdin)
		st, err := term.MakeRaw(fd)
		if err != nil {
			fmt.Fprintln(stderr, errMsg(stderr, "tty raw: "+err.Error()))
			return 1
		}
		restoreTTY = func() { _ = term.Restore(fd, st) }
		defer restoreTTY()
	}

	if err := sess.SendExec(rgosh.ExecRequest{
		Cmdline:    cmdline,
		PipeStdin:  pipeStdin,
		PipeStdout: pipeStdout,
		PipeStderr: pipeStderr,
		Term:       termName,
		Rows:       rows,
		Cols:       cols,
	}); err != nil {
		fmt.Fprintln(stderr, errMsg(stderr, err.Error()))
		return 1
	}

	line := opts.lineMode
	raw := opts.rawMode
	if interactive && !line {
		// Interactive TTY must send keystrokes immediately (arrow keys, menus).
		raw = true
	}
	if !raw && !line {
		if rtt := time.Duration(l.GetRTT() * float64(time.Second)); rgosh.PreferLineForRTT(rtt) {
			line = true
		}
	}
	var coalesceWindow time.Duration
	if raw {
		coalesceWindow = 0
		line = false
	} else if !line {
		coalesceWindow = rgosh.DefaultCoalesceWindow
	}

	coal := rgosh.NewCoalescer(line, coalesceWindow, func(b []byte) {
		_ = sess.SendStream(rgosh.StreamStdin, b, false)
	})

	var shutOnce sync.Once
	shutdown := func() {
		shutOnce.Do(func() {
			userInt.Store(true)
			stdinOff.Store(true)
			if restoreTTY != nil {
				restoreTTY()
				restoreTTY = nil
			}
			coal.Close()
			_ = sess.SendStream(rgosh.StreamStdin, nil, true)
			rgosh.ChannelSender{Ch: ch}.WaitTxIdle(2 * time.Second)
			closeDone()
		})
	}

	go func() {
		buf := make([]byte, 4096)
		for {
			if stdinOff.Load() {
				return
			}
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				if interactive && n == 1 && buf[0] == 0x03 {
					shutdown()
					return
				}
				_, _ = coal.Write(buf[:n])
			}
			if err != nil {
				coal.Close()
				_ = sess.SendStream(rgosh.StreamStdin, nil, true)
				return
			}
		}
	}()

	sig := make(chan os.Signal, 1)
	notifyShellSignals(sig)
	go func() {
		for {
			select {
			case <-done:
				return
			case s := <-sig:
				if shellSignalWinch(s, func() {
					r, c := ttySize()
					_ = sess.SendWinSize(r, c, 0, 0)
				}) {
					continue
				}
				shutdown()
				return
			}
		}
	}()

	finish := func(code int, timedOut bool) int {
		if restoreTTY != nil {
			restoreTTY()
			restoreTTY = nil
		}
		if userInt.Load() {
			if opts.mirror {
				return code
			}
			return 130
		}
		endMu.Lock()
		msg := endMsg
		endMu.Unlock()
		if timedOut {
			fmt.Fprintln(stderr, errMsg(stderr, "timeout waiting for remote exit"))
			return 1
		}
		if msg != "" {
			fmt.Fprintln(stderr, errMsg(stderr, msg))
		}
		if opts.mirror {
			return code
		}
		if msg != "" {
			return 1
		}
		return 0
	}

	if interactive {
		select {
		case code := <-exitCh:
			return finish(code, false)
		case <-done:
			code := 0
			select {
			case code = <-exitCh:
			case <-time.After(3 * time.Second):
			}
			return finish(code, false)
		}
	}

	select {
	case code := <-exitCh:
		return finish(code, false)
	case <-done:
		code := 0
		select {
		case code = <-exitCh:
		default:
		}
		return finish(code, false)
	case <-time.After(opts.timeout + 30*time.Second):
		return finish(1, true)
	}
}

func fileFD(f *os.File) int {
	// #nosec G115 -- kernel fds are small integers well below MaxInt on all supported OSes
	return int(f.Fd())
}

func isTTY(f *os.File) bool {
	return term.IsTerminal(fileFD(f))
}

func ttySize() (rows, cols int) {
	rows, cols = 24, 80
	w, h, err := term.GetSize(fileFD(os.Stdout))
	if err != nil || w <= 0 || h <= 0 {
		w, h, err = term.GetSize(fileFD(os.Stdin))
	}
	if err == nil && w > 0 && h > 0 {
		cols, rows = w, h
	}
	return rows, cols
}
