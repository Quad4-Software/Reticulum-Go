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
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"quad4/reticulum-go/pkg/common"
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
	listenMode := fs.Bool("l", false, "listen for incoming transfers")
	fetchMode := fs.Bool("f", false, "fetch file from remote (requires -f path and destination)")
	fetchPath := fs.String("F", "", "remote file path to fetch (with -f)")
	timeoutSec := fs.Float64("w", 15, "path and link timeout in seconds")
	silent := fs.Bool("s", false, "silent (minimal progress)")
	noCompress := fs.Bool("no-compress", false, "disable auto compression")
	allowAll := fs.Bool("a", false, "allow unauthenticated senders (listen)")
	allowFetch := fs.Bool("allow-fetch", false, "allow fetch_file requests (listen)")
	jail := fs.String("jail", "", "restrict fetch paths under this directory")
	saveDir := fs.String("save", "", "directory to save received files")
	overwrite := fs.Bool("overwrite", false, "overwrite existing files on receive")
	announceSec := fs.Float64("announce", 0, "announce interval seconds (0 = once at start, <0 = never)")
	printID := fs.Bool("p", false, "print identity and destination hash then exit")
	var allowed flagStringList
	fs.Var(&allowed, "allowed", "allowed identity hash (repeatable)")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := rnsutil.LoadConfigDir(*configDir)
	if err != nil {
		fmt.Fprintf(stderr, "config: %v\n", err)
		return 1
	}
	cfg.ShareInstance = true
	if cfg.SharedInstanceType == "" {
		cfg.SharedInstanceType = common.SharedInstanceTCP
	}

	idPath := *identityPath
	if idPath == "" {
		idPath = rnsutil.RNCPIdentityPath(rnsutil.StorageDir(cfg))
	}
	id, err := rnsutil.PrepareRNCPIdentity(idPath)
	if err != nil {
		fmt.Fprintf(stderr, "identity: %v\n", err)
		return 2
	}

	timeout := time.Duration(*timeoutSec * float64(time.Second))
	if timeout <= 0 {
		timeout = rnsutil.DefaultRNCPTimeout
	}

	if *printID {
		destHash := destination.Hash(id, rnsutil.RNCPAppName, rnsutil.RNCPAspect)
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
		if fs.NArg() != 1 || *fetchPath == "" {
			fmt.Fprintln(stderr, "usage: rgocp -f -F <remote_path> [flags] <destination_hash>")
			return 2
		}
		destHash, err := rnsutil.ParseDestHash(fs.Arg(0))
		if err != nil {
			fmt.Fprintf(stderr, "%v\n", err)
			return 1
		}
		return runFetch(tr, id, destHash, *fetchPath, timeout, *silent, *saveDir, *overwrite, stdout, stderr)
	default:
		if fs.NArg() != 2 {
			fmt.Fprintln(stderr, "usage: rgocp [flags] <file> <destination_hash>")
			fmt.Fprintln(stderr, "       rgocp -l [flags]")
			fmt.Fprintln(stderr, "       rgocp -f -F <remote_path> [flags] <destination_hash>")
			return 2
		}
		filePath := fs.Arg(0)
		destHash, err := rnsutil.ParseDestHash(fs.Arg(1))
		if err != nil {
			fmt.Fprintf(stderr, "%v\n", err)
			return 1
		}
		return runSend(tr, id, filePath, destHash, timeout, *silent, !*noCompress, stdout, stderr)
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
			fmt.Fprintf(stderr, "jail: %v\n", err)
			return 1
		}
		fmt.Fprintln(stderr, infoMsg(stderr, fmt.Sprintf("Restricting fetch requests to paths under %q", jailAbs)))
	}

	dest, err := destination.New(id, destination.In, destination.Single, rnsutil.RNCPAppName, tr, rnsutil.RNCPAspect)
	if err != nil {
		fmt.Fprintf(stderr, "destination: %v\n", err)
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
				fmt.Fprintf(stderr, "save: %v\n", err)
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
	if jail != "" {
		cleaned := strings.TrimPrefix(req, jail+"/")
		full := filepath.Join(jail, cleaned)
		abs, err := filepath.Abs(full)
		if err != nil {
			return "", false
		}
		if !strings.HasPrefix(abs, jail+string(os.PathSeparator)) && abs != jail {
			return "", false
		}
		return abs, true
	}
	abs, err := filepath.Abs(req)
	if err != nil {
		return "", false
	}
	return abs, true
}

func runSend(tr *transport.Transport, id *identity.Identity, filePath string, destHash []byte, timeout time.Duration, silent, compress bool, stdout, stderr io.Writer) int {
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
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	fmt.Fprintln(stdout, infoMsg(stdout, fmt.Sprintf("Path to %s requested", rnsutil.PrettyHex(destHash))))
	l, err := rnsutil.EstablishRNCPLink(ctx, tr, destHash)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	defer l.Teardown()

	if err := l.Identify(id); err != nil {
		fmt.Fprintf(stderr, "identify: %v\n", err)
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
		prog.Update("Transferring", pct, got, total, bps)
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

func runFetch(tr *transport.Transport, id *identity.Identity, destHash []byte, remotePath string, timeout time.Duration, silent bool, saveDir string, overwrite bool, stdout, stderr io.Writer) int {
	_ = silent
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
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	fmt.Fprintln(stdout, infoMsg(stdout, fmt.Sprintf("Path to %s requested", rnsutil.PrettyHex(destHash))))
	l, err := rnsutil.EstablishRNCPLink(ctx, tr, destHash)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	defer l.Teardown()

	if err := l.Identify(id); err != nil {
		fmt.Fprintf(stderr, "identify: %v\n", err)
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
		fmt.Fprintf(stderr, "request: %v\n", err)
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
		fmt.Fprintf(stderr, "save: %v\n", err)
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
