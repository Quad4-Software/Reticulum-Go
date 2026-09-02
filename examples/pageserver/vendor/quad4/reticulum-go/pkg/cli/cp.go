// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cli

import (
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/link"
	"quad4/reticulum-go/pkg/node"
	"quad4/reticulum-go/pkg/resource"
	"quad4/reticulum-go/pkg/rnsutil"
	"quad4/reticulum-go/pkg/term"
	"quad4/reticulum-go/pkg/transport"
)

func RunCP(args []string, opt ...Options) int {
	stdout, stderr := cliIO(opt)
	fs := flag.NewFlagSet("rgocp", flag.ContinueOnError)
	fs.SetOutput(stderr)

	configDir := fs.String("config", "", "path to config directory")
	identityPath := fs.String("identity", "", "path to identity file (default: storage/identities/rncp)")
	fs.StringVar(identityPath, "i", "", "path to identity file (Python rncp alias)")
	listenMode := fs.Bool("l", false, "listen for incoming transfers")
	fetchMode := fs.Bool("f", false, "fetch file from remote listener instead of sending")
	timeoutSec := fs.Float64("w", 0, "path and link timeout in seconds (0 = adaptive from interface bitrate)")
	silent := fs.Bool("S", false, "silent (minimal progress)")
	fs.BoolVar(silent, "silent", false, "silent (minimal progress)")
	noCompress := fs.Bool("C", false, "disable auto compression")
	fs.BoolVar(noCompress, "no-compress", false, "disable auto compression")
	allowAll := fs.Bool("n", false, "accept requests from anyone (listen)")
	fs.BoolVar(allowAll, "no-auth", false, "accept requests from anyone (listen)")
	allowFetch := fs.Bool("F", false, "allow authenticated clients to fetch files (listen)")
	fs.BoolVar(allowFetch, "allow-fetch", false, "allow authenticated clients to fetch files (listen)")
	jail := fs.String("j", "", "restrict fetch requests to specified path")
	fs.StringVar(jail, "jail", "", "restrict fetch requests to specified path")
	saveDir := fs.String("s", "", "directory to save received files")
	fs.StringVar(saveDir, "save", "", "directory to save received files")
	overwrite := fs.Bool("O", false, "overwrite existing files on receive")
	fs.BoolVar(overwrite, "overwrite", false, "overwrite existing files on receive")
	announceSec := fs.Float64("b", 0, "announce interval seconds (0 = once at start, <0 = never)")
	fs.Float64Var(announceSec, "announce", 0, "announce interval seconds (0 = once at start, <0 = never)")
	printID := fs.Bool("p", false, "print identity and destination hash then exit")
	phyRates := fs.Bool("P", false, "display physical layer transfer rates")
	fs.BoolVar(phyRates, "phy-rates", false, "display physical layer transfer rates")
	var allowed flagStringList
	fs.Var(&allowed, "a", "allowed identity hash (repeatable, listen)")
	fs.Var(&allowed, "allowed", "allowed identity hash (repeatable, listen)")
	bindFlagUsage(fs, "rgocp - file transfer over RNS links",
		"Send, receive, fetch, or listen for file transfers. Flags match Python rncp.",
		[]helpLine{
			{Cmd: "rgocp [flags] <file> <destination_hash>"},
			{Cmd: "rgocp -l [flags]"},
			{Cmd: "rgocp -f [flags] <remote_path> <destination_hash>"},
			{Cmd: "reticulum-go cp [flags] ..."},
		},
		"rgocp document.pdf <dest_hash>",
		"rgocp -l -s ./incoming -a <identity_hash>",
		"rgocp -l -n -F",
		"rgocp -f /path/on/remote <dest_hash>",
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
		idPath = rnsutil.RNCPIdentityPath(rnsutil.StorageDir(cfg))
	}
	id, err := rnsutil.PrepareRNCPIdentity(idPath)
	if err != nil {
		diagErr(stderr, "identity", err)
		return 2
	}

	timeout := max(time.Duration(*timeoutSec*float64(time.Second)), 0)

	if *printID {
		destHash := destination.Hash(id, rnsutil.RNCPAppName, rnsutil.RNCPAspect)
		fmt.Fprintf(stdout, "%s : %s\n", infoMsg(stdout, "Identity"), hex.EncodeToString(id.Hash()))
		fmt.Fprintf(stdout, "%s : %s\n", infoMsg(stdout, "Listening on"), hex.EncodeToString(destHash))
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

	switch {
	case *listenMode:
		return runListen(tr, id, listenOpts{
			allowAll:    *allowAll,
			allowFetch:  *allowFetch,
			jail:        *jail,
			saveDir:     *saveDir,
			overwrite:   *overwrite,
			noCompress:  *noCompress,
			announceSec: *announceSec,
			allowedCLI:  allowed,
			stderr:      stderr,
		})
	case *fetchMode:
		if fs.NArg() != 2 {
			usageErr(stderr, "rgocp -f [flags] <remote_path> <destination_hash>")
			return 2
		}
		remotePath := fs.Arg(0)
		destHash, err := rnsutil.ParseDestHash(fs.Arg(1))
		if err != nil {
			fmt.Fprintf(stderr, "%v\n", err)
			return 1
		}
		return runFetch(tr, id, destHash, remotePath, timeout, *silent, *saveDir, *overwrite, *phyRates, stdout, stderr)
	default:
		if fs.NArg() != 2 {
			usageErr(stderr, "rgocp [flags] <file> <destination_hash>")
			usageErr(stderr, "rgocp -l [flags]")
			usageErr(stderr, "rgocp -f [flags] <remote_path> <destination_hash>")
			return 2
		}
		filePath := fs.Arg(0)
		destHash, err := rnsutil.ParseDestHash(fs.Arg(1))
		if err != nil {
			fmt.Fprintf(stderr, "%v\n", err)
			return 1
		}
		return runSend(tr, id, filePath, destHash, timeout, *silent, !*noCompress, *phyRates, stdout, stderr)
	}
}

type listenOpts struct {
	allowAll    bool
	allowFetch  bool
	jail        string
	saveDir     string
	overwrite   bool
	noCompress  bool
	announceSec float64
	allowedCLI  []string
	stderr      io.Writer
}

func runListen(tr *transport.Transport, id *identity.Identity, opts listenOpts) int {
	stderr := opts.stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	allowed, err := rnsutil.LoadAllowedIdentities(opts.allowedCLI)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	if !opts.allowAll && len(allowed) == 0 {
		fmt.Fprintln(stderr, warnMsg(stderr, "No allowed identities configured, rgocp will not accept any files!"))
	}

	saveDir := opts.saveDir
	if saveDir != "" {
		abs, err := filepath.Abs(saveDir)
		if err != nil || !dirWritable(abs) {
			fmt.Fprintln(stderr, errMsg(stderr, "Output directory not found or not writable"))
			return 3
		}
		saveDir = abs
	}

	var jailAbs string
	if opts.jail != "" {
		jailAbs, err = filepath.Abs(opts.jail)
		if err != nil {
			diagErr(stderr, "jail", err)
			return 1
		}
		fmt.Fprintln(stderr, infoMsg(stderr, fmt.Sprintf("Restricting fetch requests to paths under %q", jailAbs)))
	}

	dest, err := destination.New(id, destination.In, destination.Single, rnsutil.RNCPAppName, tr, rnsutil.RNCPAspect)
	if err != nil {
		diagErr(stderr, "destination", err)
		return 1
	}
	dest.AcceptsLinks(true)

	var mu sync.Mutex
	dest.SetLinkEstablishedCallback(func(lnk any) {
		l, ok := lnk.(*link.Link)
		if !ok || l == nil {
			return
		}
		fmt.Fprintln(stderr, infoMsg(stderr, "Incoming link established"))
		_ = l.SetResourceStrategy(link.AcceptApp)
		l.SetRemoteIdentifiedCallback(func(lnk *link.Link, remote *identity.Identity) {
			if opts.allowAll {
				return
			}
			if remote == nil || !rnsutil.AllowedContains(allowed, remote.Hash()) {
				fmt.Fprintln(stderr, warnMsg(stderr, "Sender not allowed, tearing down link"))
				lnk.Teardown()
			}
		})
		l.SetResourceCallback(func(any) bool {
			remote := l.GetRemoteIdentity()
			if remote != nil && rnsutil.AllowedContains(allowed, remote.Hash()) {
				return true
			}
			return opts.allowAll
		})
		l.SetResourceStartedCallback(func(any) {
			fmt.Fprintln(stderr, infoMsg(stderr, "Starting resource transfer"))
		})
		l.SetResourceConcludedCallback(func(p any) {
			var data []byte
			var meta map[string]any
			switch v := p.(type) {
			case link.IncomingResource:
				data = v.Data
				meta = v.Metadata
			case []byte:
				data = v
			default:
				fmt.Fprintln(stderr, errMsg(stderr, "Invalid data received, ignoring resource"))
				return
			}
			if meta == nil {
				fmt.Fprintln(stderr, errMsg(stderr, "Invalid data received, ignoring resource"))
				return
			}
			name := rnsutil.FilenameFromMetadata(meta)
			mu.Lock()
			path, err := rnsutil.WriteReceivedFile(saveDir, name, data, opts.overwrite)
			mu.Unlock()
			if err != nil {
				diagErr(stderr, "save", err)
				return
			}
			fmt.Fprintln(stderr, okMsg(stderr, fmt.Sprintf("Saved received file to %s", path)))
		})
	})

	if opts.allowFetch {
		allowMode := byte(destination.AllowList)
		allowList := allowed
		if opts.allowAll {
			allowMode = destination.AllowAll
			allowList = nil
			fmt.Fprintln(stderr, warnMsg(stderr, "Allowing unauthenticated fetch requests"))
		}
		_ = dest.RegisterRequestHandlerAny(rnsutil.RNCPFetchPath, func(_ string, data []byte, _ []byte, linkID []byte, _ *identity.Identity, _ int64) any {
			filePath, ok := resolveFetchPath(string(data), jailAbs)
			if !ok {
				return rnsutil.RNCPFetchNotAllowed
			}
			st, err := os.Stat(filePath)
			if err != nil || st.IsDir() {
				return false
			}
			li := tr.FindLink(linkID)
			target, ok := li.(*link.Link)
			if !ok || target == nil {
				return nil
			}
			body, err := os.ReadFile(filePath) // #nosec G304 -- jail-validated path
			if err != nil {
				return nil
			}
			res, err := resource.New(body, !opts.noCompress)
			if err != nil {
				return nil
			}
			_ = res.SetMetadata(map[string]any{"name": []byte(filepath.Base(filePath))})
			go func() { _ = target.SendResource(res) }()
			return true
		}, allowMode, allowList)
	}

	fmt.Fprintln(stderr, okMsg(stderr, fmt.Sprintf("rgocp listening on %s", rnsutil.PrettyHex(dest.GetHash()))))

	if opts.announceSec >= 0 {
		_ = dest.Announce(false, nil, nil)
		if opts.announceSec > 0 {
			go func() {
				t := time.NewTicker(time.Duration(opts.announceSec * float64(time.Second)))
				defer t.Stop()
				for range t.C {
					_ = dest.Announce(false, nil, nil)
				}
			}()
		}
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	return 0
}

func resolveFetchPath(req, jail string) (string, bool) {
	req = strings.TrimSpace(req)
	if jail == "" {
		abs, err := filepath.Abs(req)
		if err != nil {
			return "", false
		}
		return abs, true
	}

	cleaned := strings.TrimPrefix(req, jail+"/")
	full := filepath.Join(jail, cleaned)
	abs, err := filepath.Abs(full)
	if err != nil {
		return "", false
	}
	if !strings.HasPrefix(abs, jail+string(os.PathSeparator)) && abs != jail {
		return "", false
	}

	// A Clean+HasPrefix check on the unresolved string is not enough: a
	// symlink inside jail can point outside it while the string still looks
	// contained. Resolve symlinks on both sides before trusting abs.
	resolvedJail, err := filepath.EvalSymlinks(jail)
	if err != nil {
		resolvedJail = jail
	}
	resolved, ok := evalExistingAncestorPath(abs)
	if !ok {
		return "", false
	}
	if resolved != resolvedJail && !strings.HasPrefix(resolved, resolvedJail+string(os.PathSeparator)) {
		return "", false
	}
	return resolved, true
}

// evalExistingAncestorPath resolves symlinks in path, walking up to the
// nearest existing ancestor when the leaf (or more) does not exist yet, then
// reattaches the missing suffix unresolved. A symlinked ancestor still
// cannot smuggle a request outside the jail just because the final
// component happens not to exist yet.
func evalExistingAncestorPath(path string) (string, bool) {
	suffix := ""
	cur := path
	for {
		resolved, err := filepath.EvalSymlinks(cur)
		if err == nil {
			if suffix == "" {
				return resolved, true
			}
			return filepath.Join(resolved, suffix), true
		}
		if !os.IsNotExist(err) {
			return "", false
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", false
		}
		base := filepath.Base(cur)
		if suffix == "" {
			suffix = base
		} else {
			suffix = filepath.Join(base, suffix)
		}
		cur = parent
	}
}

func runSend(tr *transport.Transport, id *identity.Identity, filePath string, destHash []byte, timeout time.Duration, silent, compress, phyRates bool, stdout, stderr io.Writer) int {
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}
	if st, err := os.Stat(filePath); err != nil || st.IsDir() {
		fmt.Fprintln(stdout, errMsg(stdout, "File not found"))
		return 1
	}
	ctx, cancel := rnsutil.CLIWaitContext(timeout)
	defer cancel()

	fmt.Fprintln(stdout, infoMsg(stdout, fmt.Sprintf("Path to %s requested", rnsutil.PrettyHex(destHash))))
	l, err := rnsutil.EstablishRNCPLink(ctx, tr, destHash)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	defer l.Teardown()

	if err := l.Identify(id); err != nil {
		diagErr(stderr, "identify", err)
		return 1
	}

	progOut := term.FileOf(stderr)
	if progOut == nil {
		progOut = os.Stderr
	}
	prog := rnsutil.NewProgressPrinterTo(silent, progOut)
	start := time.Now()
	var lastGot int64
	var lastAt time.Time
	err = rnsutil.SendFileOverLink(l, filePath, compress, func(pct float64, got, total int64) {
		now := time.Now()
		bps := 0.0
		if !lastAt.IsZero() && now.After(lastAt) {
			bps = float64(got-lastGot) * 8 / now.Sub(lastAt).Seconds()
		}
		lastGot, lastAt = got, now
		suffix := ""
		if phyRates && bps > 0 {
			suffix = fmt.Sprintf(" (%s/s at physical layer)", rnsutil.SizeString(bps, "b"))
		}
		prog.Update("Transferring"+suffix, pct, got, total, bps)
	})
	if err != nil {
		prog.Done(errMsg(stderr, "Transfer failed: "+err.Error()))
		return 1
	}
	elapsed := time.Since(start)
	st, _ := os.Stat(filePath)
	size := int64(0)
	if st != nil {
		size = st.Size()
	}
	bps := float64(size) * 8 / elapsed.Seconds()
	prog.Done(okMsg(stderr, fmt.Sprintf("Transfer complete - %s in %s - %s/s",
		rnsutil.SizeString(float64(size), "B"), elapsed.Truncate(time.Millisecond), rnsutil.SizeString(bps, "b"))))
	fmt.Fprintln(stdout, okMsg(stdout, fmt.Sprintf("%s sent to %s", filepath.Base(filePath), rnsutil.PrettyHex(destHash))))
	return 0
}

func runFetch(tr *transport.Transport, id *identity.Identity, destHash []byte, remotePath string, timeout time.Duration, silent bool, saveDir string, overwrite, phyRates bool, stdout, stderr io.Writer) int {
	_ = silent
	_ = phyRates
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}
	if saveDir != "" {
		abs, err := filepath.Abs(saveDir)
		if err != nil || !dirWritable(abs) {
			fmt.Fprintln(stderr, errMsg(stderr, "Output directory not found or not writable"))
			return 3
		}
		saveDir = abs
	}
	ctx, cancel := rnsutil.CLIWaitContext(timeout)
	defer cancel()

	fmt.Fprintln(stdout, infoMsg(stdout, fmt.Sprintf("Path to %s requested", rnsutil.PrettyHex(destHash))))
	l, err := rnsutil.EstablishRNCPLink(ctx, tr, destHash)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	defer l.Teardown()

	if err := l.Identify(id); err != nil {
		diagErr(stderr, "identify", err)
		return 1
	}

	done := make(chan struct{})
	var received link.IncomingResource
	var once sync.Once
	_ = l.SetResourceStrategy(link.AcceptAll)
	l.SetResourceConcludedCallback(func(p any) {
		once.Do(func() {
			switch v := p.(type) {
			case link.IncomingResource:
				received = v
			case []byte:
				received = link.IncomingResource{Data: v}
			}
			close(done)
		})
	})

	receipt, err := l.Request(rnsutil.RNCPFetchPath, remotePath, timeout)
	if err != nil {
		diagErr(stderr, "request", err)
		return 1
	}
	if err := rnsutil.WaitRequest(ctx, receipt); err != nil {
		fmt.Fprintf(stderr, "request timeout: %v\n", err)
		return 1
	}
	switch rnsutil.ClassifyFetchResponse(receipt.GetResponseValue()) {
	case rnsutil.FetchNotAllowed:
		fmt.Fprintln(stdout, errMsg(stdout, fmt.Sprintf("Fetch request failed, fetching the file %s was not allowed by the remote", remotePath)))
		return 0
	case rnsutil.FetchNotFound:
		fmt.Fprintln(stdout, errMsg(stdout, fmt.Sprintf("Fetch request failed, the file %s was not found on the remote", remotePath)))
		return 0
	case rnsutil.FetchRemoteError:
		fmt.Fprintln(stdout, errMsg(stdout, "Fetch request failed due to an error on the remote system"))
		return 0
	case rnsutil.FetchUnknown:
		fmt.Fprintln(stdout, errMsg(stdout, "Fetch request failed due to an unknown error (probably not authorised)"))
		return 0
	case rnsutil.FetchFound:
	}

	select {
	case <-done:
	case <-ctx.Done():
		fmt.Fprintln(stdout, errMsg(stdout, "The transfer failed"))
		return 1
	}
	if len(received.Data) == 0 && received.Metadata == nil {
		fmt.Fprintln(stdout, errMsg(stdout, "The transfer failed"))
		return 1
	}
	name := rnsutil.FilenameFromMetadata(received.Metadata)
	path, err := rnsutil.WriteReceivedFile(saveDir, name, received.Data, overwrite)
	if err != nil {
		diagErr(stderr, "save", err)
		return 1
	}
	fmt.Fprintln(stdout, okMsg(stdout, fmt.Sprintf("%s fetched from %s -> %s", remotePath, rnsutil.PrettyHex(destHash), path)))
	return 0
}

func dirWritable(path string) bool {
	st, err := os.Stat(path)
	if err != nil || !st.IsDir() {
		return false
	}
	f, err := os.CreateTemp(path, ".rgocp-write-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

type flagStringList []string

func (f *flagStringList) String() string { return strings.Join(*f, ",") }
func (f *flagStringList) Set(v string) error {
	*f = append(*f, v)
	return nil
}
