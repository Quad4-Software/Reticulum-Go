// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/link"
	"quad4/reticulum-go/pkg/node"
	"quad4/reticulum-go/pkg/rnsutil"
	"quad4/reticulum-go/pkg/transport"
)

// ClientOptions configures the git-remote-rns helper.
type ClientOptions struct {
	ConfigDir      string
	RNSConfigDir   string
	DestHex        string
	Group          string
	Repo           string
	Timeout        time.Duration
	JSONProgress   bool
	ProgressWriter io.Writer
}

// Client implements git-remote-rns over Reticulum links.
type Client struct {
	cfg            *ClientConfig
	destHex        string
	repoPath       string
	identity       *identity.Identity
	tr             *transport.Transport
	nodeStop       func()
	link           *link.Link
	refBatchSize   int
	remoteRefs     map[string]string
	progress       bool
	jsonProgress   bool
	progressWriter io.Writer
	tmpDir         string
}

// NewClient creates a git-remote-rns client.
func NewClient(opts ClientOptions) (*Client, error) {
	if err := EnsureClientConfig(opts.ConfigDir); err != nil {
		return nil, err
	}
	cfg, err := LoadClientConfig(opts.ConfigDir)
	if err != nil {
		return nil, err
	}
	if opts.RNSConfigDir != "" {
		cfg.RNSConfigDir = opts.RNSConfigDir
	}
	destHex := opts.DestHex
	if alias, ok := cfg.DestAliases[destHex]; ok {
		destHex = alias
	}
	id, err := PrepareGitIdentity(cfg.IdentityPath)
	if err != nil {
		return nil, fmt.Errorf("identity: %w", err)
	}
	tmp, err := os.MkdirTemp("", "rnsgit-client-")
	if err != nil {
		return nil, err
	}
	batch := cfg.RefBatchSize
	if batch <= 0 {
		batch = DefaultRefBatchSize
	}
	return &Client{
		cfg:            cfg,
		destHex:        destHex,
		repoPath:       RepoPath(opts.Group, opts.Repo),
		identity:       id,
		refBatchSize:   batch,
		remoteRefs:     map[string]string{},
		progressWriter: opts.ProgressWriter,
		jsonProgress:   opts.JSONProgress,
		tmpDir:         tmp,
	}, nil
}

// Close releases temporary resources.
func (c *Client) Close() {
	if c.link != nil {
		c.link.Teardown()
	}
	if c.nodeStop != nil {
		c.nodeStop()
	}
	_ = os.RemoveAll(c.tmpDir)
}

// UseTransport attaches an existing Reticulum transport for Connect.
func (c *Client) UseTransport(tr *transport.Transport, stop func()) {
	c.tr = tr
	c.nodeStop = stop
}

// Connect starts transport and opens a link to the git node.
func (c *Client) Connect(ctx context.Context) error {
	if c.tr == nil {
		rnsCfg, err := rnsutil.LoadConfigDir(c.cfg.RNSConfigDir)
		if err != nil {
			return err
		}
		n, err := node.New(rnsCfg)
		if err != nil {
			return err
		}
		if err := n.Start(); err != nil {
			return err
		}
		c.nodeStop = func() { _ = n.Stop() }
		c.tr = n.Transport()
	}

	if err := waitTransportOutgoing(ctx, c.tr); err != nil {
		c.stopAttachedNode()
		return err
	}

	destHash, err := rnsutil.ParseDestHash(c.destHex)
	if err != nil {
		c.stopAttachedNode()
		return err
	}
	if err := rnsutil.WaitPathWindow(ctx, c.tr, destHash); err != nil {
		c.stopAttachedNode()
		return fmt.Errorf("path: %w", err)
	}
	remote, err := identity.Recall(destHash)
	if err != nil {
		c.stopAttachedNode()
		return fmt.Errorf("recall: %w", err)
	}
	outDest, err := destination.New(remote, destination.Out, destination.Single, AppName, c.tr, Aspect)
	if err != nil {
		c.stopAttachedNode()
		return err
	}
	l := link.NewLink(outDest, c.tr, nil, nil, nil)
	if err := activateGitLink(ctx, l); err != nil {
		c.stopAttachedNode()
		return err
	}
	if err := l.Identify(c.identity); err != nil {
		l.Teardown()
		c.stopAttachedNode()
		return fmt.Errorf("identify: %w", err)
	}
	time.Sleep(100 * time.Millisecond)
	c.link = l
	c.refBatchSize = dynamicRefBatch(c.refBatchSize, l)
	return nil
}

func (c *Client) stopAttachedNode() {
	if c.nodeStop != nil {
		c.nodeStop()
		c.nodeStop = nil
	}
	c.tr = nil
}

func activateGitLink(ctx context.Context, l *link.Link) error {
	if err := l.Establish(); err != nil {
		return err
	}
	wait, cancel := rnsutil.BoundWait(ctx, rnsutil.LinkEstablishmentWindow(l))
	defer cancel()
	if err := rnsutil.WaitLinkActive(wait, l); err != nil {
		l.Teardown()
		return err
	}
	return nil
}

func dynamicRefBatch(base int, l *link.Link) int {
	if l == nil || base <= 0 {
		return DefaultRefBatchSize
	}
	rtt := l.GetRTT()
	if rtt <= 0 {
		return base
	}
	if rtt > 2.0 {
		n := base / 2
		if n < 4 {
			return 4
		}
		return n
	}
	if rtt < 0.2 {
		n := base * 2
		if n > 64 {
			return 64
		}
		return n
	}
	return base
}

// RunGitHelper executes the git remote-helper protocol on stdin/stdout.
func (c *Client) RunGitHelper(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer) error {
	if err := c.Connect(ctx); err != nil {
		return err
	}
	if stderr != nil {
		fmt.Fprint(stderr, "\rPath resolved     \n")
	}
	in := bufio.NewReader(stdin)
	fetchQueue := make([][2]string, 0)
	pushQueue := make([][2]string, 0)

	for {
		line, err := in.ReadString('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if err := c.flushQueues(ctx, fetchQueue, pushQueue, stdout, stderr); err != nil {
				return err
			}
			fetchQueue = fetchQueue[:0]
			pushQueue = pushQueue[:0]
			fmt.Fprintln(stdout)
			continue
		}
		switch {
		case line == "capabilities":
			fmt.Fprintln(stdout, "list")
			fmt.Fprintln(stdout, "fetch")
			fmt.Fprintln(stdout, "push")
			fmt.Fprintln(stdout, "option")
			fmt.Fprintln(stdout)
		case line == "list":
			if err := c.handleList(ctx, stdout, false); err != nil {
				return err
			}
		case strings.HasPrefix(line, "list "):
			if err := c.handleList(ctx, stdout, true); err != nil {
				return err
			}
		case strings.HasPrefix(line, "option"):
			parts := strings.Fields(line)
			if len(parts) >= 3 && parts[1] == "progress" {
				c.progress = parts[2] == "true" || parts[2] == "1" || parts[2] == "yes"
				fmt.Fprintln(stdout, "ok")
			} else {
				fmt.Fprintln(stdout, "unsupported")
			}
		case strings.HasPrefix(line, "fetch"):
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				fetchQueue = append(fetchQueue, [2]string{parts[1], parts[2]})
				pushQueue = pushQueue[:0]
			}
		case strings.HasPrefix(line, "push"):
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				spec := parts[1]
				local, remote, ok := strings.Cut(spec, ":")
				if !ok {
					local, remote = spec, ""
				}
				pushQueue = append(pushQueue, [2]string{local, remote})
				fetchQueue = fetchQueue[:0]
			}
		default:
			return fmt.Errorf("unknown git command: %s", line)
		}
	}
	return nil
}

func (c *Client) handleList(ctx context.Context, stdout io.Writer, forPush bool) error {
	req := map[any]any{IdxRepository: c.repoPath}
	if forPush {
		req["for_push"] = true
	}
	body, err := c.sendRequest(ctx, PathList, req, 120*time.Second)
	if err != nil {
		return err
	}
	if len(body) == 0 || body[0] != ResOK {
		msg := string(body[1:])
		return fmt.Errorf("list failed: %s", msg)
	}
	text := string(body[1:])
	c.remoteRefs = map[string]string{}
	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 || parts[1] == "HEAD" {
			continue
		}
		c.remoteRefs[parts[1]] = parts[0]
	}
	fmt.Fprint(stdout, text)
	if !strings.HasSuffix(text, "\n") {
		fmt.Fprintln(stdout)
	}
	return nil
}

func (c *Client) flushQueues(ctx context.Context, fetch [][2]string, push [][2]string, stdout, stderr io.Writer) error {
	if len(fetch) > 0 {
		if err := c.processFetch(ctx, fetch, stderr); err != nil {
			return err
		}
	}
	for _, item := range push {
		if err := c.processPush(ctx, item[0], item[1], stdout, stderr); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) processFetch(ctx context.Context, queue [][2]string, stderr io.Writer) error {
	have := make([]string, 0)
	for _, sha := range c.remoteRefs {
		if out, err := localGitCmd("cat-file", "-t", sha).Output(); err == nil && strings.TrimSpace(string(out)) != "" {
			have = append(have, sha)
		}
	}
	for len(queue) > 0 {
		n := min(c.refBatchSize, len(queue))
		batch := queue[:n]
		queue = queue[n:]
		refs := make([]map[string]string, 0, len(batch))
		for _, item := range batch {
			entry := map[string]string{"sha": item[0], "ref": item[1]}
			if out, err := localGitCmd("rev-parse", item[1]).Output(); err == nil {
				local := strings.TrimSpace(string(out))
				if local != "" && local != item[0] {
					entry["have"] = local
				}
			}
			refs = append(refs, entry)
		}
		reqMap := map[any]any{IdxRepository: c.repoPath, "refs": refs}
		if len(have) > 0 {
			reqMap["have"] = have
		}
		body, meta, err := c.sendRequestWithMeta(ctx, PathFetch, reqMap, 2*time.Hour)
		if err != nil {
			return err
		}
		if body == nil && meta == nil {
			return fmt.Errorf("empty fetch response")
		}
		if body != nil && len(body) > 0 && body[0] == ResOK && len(body) == 1 {
			continue
		}
		bundlePath := ""
		if len(body) > 0 && meta == nil && body[0] == ResOK {
			continue
		}
		if meta != nil {
			if code, ok := MetadataResultCode(meta); ok && code != ResOK {
				return fmt.Errorf("fetch failed code %d", code)
			}
			bundlePath = filepath.Join(c.tmpDir, "fetch.bundle")
			if err := os.WriteFile(bundlePath, body, 0o600); err != nil {
				return err
			}
		} else if body != nil {
			if body[0] != ResOK {
				if isGitBundle(body) {
					bundlePath = filepath.Join(c.tmpDir, "fetch.bundle")
					if err := os.WriteFile(bundlePath, body, 0o600); err != nil {
						return err
					}
				} else {
					return fmt.Errorf("fetch failed: %s", string(body[1:]))
				}
			} else {
				bundlePath = filepath.Join(c.tmpDir, "fetch.bundle")
				if err := os.WriteFile(bundlePath, body[1:], 0o600); err != nil {
					return err
				}
			}
		}
		if bundlePath == "" {
			continue
		}
		if c.progress && stderr != nil {
			if st, err := os.Stat(bundlePath); err == nil {
				fmt.Fprintf(stderr, "Transferring: 100%% (%d bytes).\n", st.Size())
			}
		}
		if err := localGitCmd("bundle", "verify", "-q", bundlePath).Run(); err != nil {
			return fmt.Errorf("bundle verify failed")
		}
		args := []string{"bundle", "unbundle", bundlePath}
		if c.progress {
			args = []string{"bundle", "unbundle", "--progress", bundlePath}
		}
		cmd := localGitCmd(args...)
		if c.progress {
			cmd.Stderr = stderr
		}
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("bundle unbundle: %w", err)
		}
		for _, item := range batch {
			sha, ref := item[0], item[1]
			if SanSHA(sha) == "" || SanRef(ref) == "" {
				continue
			}
			if err := localGitCmd("update-ref", ref, sha).Run(); err != nil {
				return fmt.Errorf("update-ref %s: %w", ref, err)
			}
		}
	}
	return nil
}

func (c *Client) processPush(ctx context.Context, localRef, remoteRef string, stdout, stderr io.Writer) error {
	if localRef == "" {
		body, err := c.sendRequest(ctx, PathDelete, map[any]any{IdxRepository: c.repoPath, "ref": remoteRef}, 120*time.Second)
		if err != nil || len(body) == 0 || body[0] != ResOK {
			fmt.Fprintf(stdout, "error %s %s\n", remoteRef, EscapeGitStdout("delete failed"))
			return nil
		}
		fmt.Fprintf(stdout, "ok %s\n", remoteRef)
		return nil
	}
	force := strings.HasPrefix(localRef, "+")
	if force {
		localRef = localRef[1:]
	}
	localSHA, err := localGitCmd("rev-parse", localRef).Output()
	if err != nil {
		fmt.Fprintf(stdout, "error %s %s\n", remoteRef, EscapeGitStdout("could not resolve local ref"))
		return nil
	}
	sha := strings.TrimSpace(string(localSHA))
	bundlePath := filepath.Join(c.tmpDir, "push.bundle")
	createArgs := []string{"bundle", "create", bundlePath, localRef}
	for _, rsha := range c.remoteRefs {
		if out, err := localGitCmd("cat-file", "-t", rsha).Output(); err == nil && len(out) > 0 {
			createArgs = append(createArgs, "^"+rsha)
		}
	}
	create := localGitCmd(createArgs...)
	if c.progress {
		create.Stderr = stderr
	}
	bundleEmpty := false
	if err := create.Run(); err != nil {
		if out, _ := create.CombinedOutput(); strings.Contains(strings.ToLower(string(out)), "empty bundle") {
			bundleEmpty = true
		} else {
			fmt.Fprintf(stdout, "error %s %s\n", remoteRef, EscapeGitStdout("bundle creation failed"))
			return nil
		}
	}
	if !bundleEmpty {
		data, err := os.ReadFile(bundlePath) // #nosec G304 -- temp bundle path
		if err != nil {
			return err
		}
		body, err := c.sendRequest(ctx, PathPush, map[any]any{
			IdxRepository: c.repoPath,
			"local_ref":   localRef,
			"remote_ref":  remoteRef,
			"force":       force,
			"bundle":      data,
		}, 2*time.Hour)
		if err != nil || len(body) == 0 || body[0] != ResOK {
			msg := "push failed"
			if len(body) > 1 {
				msg = string(body[1:])
			}
			fmt.Fprintf(stdout, "error %s %s\n", remoteRef, EscapeGitStdout(msg))
			return nil
		}
	} else {
		body, err := c.sendRequest(ctx, PathPush, map[any]any{
			IdxRepository: c.repoPath,
			"operations": []map[string]any{{
				"action": "update_ref",
				"ref":    remoteRef,
				"sha":    sha,
				"force":  force,
			}},
		}, 120*time.Second)
		if err != nil || len(body) == 0 || body[0] != ResOK {
			fmt.Fprintf(stdout, "error %s %s\n", remoteRef, EscapeGitStdout("ref update failed"))
			return nil
		}
	}
	fmt.Fprintf(stdout, "ok %s\n", remoteRef)
	return nil
}

func (c *Client) sendRequest(ctx context.Context, path string, payload any, timeout time.Duration) ([]byte, error) {
	body, _, err := c.sendRequestWithMeta(ctx, path, payload, timeout)
	return body, err
}

func (c *Client) sendRequestWithMeta(ctx context.Context, path string, payload any, timeout time.Duration) ([]byte, map[string]any, error) {
	if c.link == nil {
		return nil, nil, fmt.Errorf("link not ready")
	}
	receipt, err := c.link.Request(path, payload, timeout)
	if err != nil {
		return nil, nil, err
	}
	if err := rnsutil.WaitRequest(ctx, receipt); err != nil {
		return nil, nil, err
	}
	if receipt.GetStatus() != link.StatusActive {
		return nil, nil, fmt.Errorf("request failed")
	}
	meta := receipt.GetMetadata()
	if meta != nil {
		return receipt.GetResponse(), meta, nil
	}
	return receipt.GetResponse(), nil, nil
}

// RunGitRemoteRNS is the entry point for git-remote-rns.
func RunGitRemoteRNS(args []string, rnsConfig string) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: git-remote-rns <remote-name> <url>")
		return 1
	}
	url := args[1]
	if !strings.HasPrefix(strings.ToLower(url), ProtoRNS) {
		fmt.Fprintln(os.Stderr, "Invalid URL scheme. Must be rns://")
		return 1
	}
	destHex, group, repo, err := ParseRNSURL(url)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	cfgDir := os.Getenv("RNGIT_CONFIG")
	client, err := NewClient(ClientOptions{
		ConfigDir:    cfgDir,
		RNSConfigDir: rnsConfig,
		DestHex:      destHex,
		Group:        group,
		Repo:         repo,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "git-remote-rns failed: %v\n", err)
		return 255
	}
	defer client.Close()
	ctx, cancel := rnsutil.CLIWaitContext(0)
	defer cancel()
	fmt.Fprint(os.Stderr, "Requesting path...")
	if err := client.RunGitHelper(ctx, os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "git-remote-rns failed: %v\n", err)
		return 255
	}
	return 0
}

func waitTransportOutgoing(ctx context.Context, tr *transport.Transport) error {
	if tr == nil {
		return fmt.Errorf("nil transport")
	}
	deadline := time.Now().Add(5 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if tr.SlowestOnlineBitrate() > 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
