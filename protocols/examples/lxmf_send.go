//go:build ignore

package main

import (
	"bufio"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	rnsdebug "git.quad4.io/reticulum/Reticulum-Go/pkg/debug"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/interfaces"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/reticulumconfig"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxmf"
)

const (
	initialWait     = 2 * time.Second
	pathWait        = 90 * time.Second
	pathPoll        = 500 * time.Millisecond
	defaultStamp    = 8
	defaultDestHash = "f489752fbef161c64d65e385a4e9fc74"
)

type chatSession struct {
	mu           sync.Mutex
	messenger    *lxmf.Messenger
	transport    *transport.Transport
	defaultDest  []byte
	replyDest    []byte
	stampCost    int
	title        string
	propNode     *lxmf.PropagationNode
	propRegistry *lxmf.PropagationRegistry
	propRetries  int
}

func main() {
	destHex := flag.String("dest", defaultDestHash, "default remote lxmf.delivery destination hash (hex)")
	title := flag.String("title", "go-lxmf", "initial message title")
	content := flag.String("content", "Hello from reticulum-go-protocols", "initial message body")
	displayName := flag.String("name", "Go LXMF", "display name in lxmf announce")
	configPath := flag.String("config", "", "Reticulum-Go config file (default ~/.reticulum-go/config)")
	stampCost := flag.Int("stamp", defaultStamp, "PoW stamp cost for outbound messages (0 to skip)")
	identityPath := flag.String("identity", "", "64-byte identity file (default ~/.reticulum-go/storage/identity)")
	announceEvery := flag.Duration("announce-every", 15*time.Minute, "re-announce interval (0 = once at startup)")
	noInitialSend := flag.Bool("no-initial-send", false, "skip the startup message")
	logLevel := flag.Int("loglevel", 0, "reticulum log level 0-7 (0=silent, 1=critical, 3=info)")
	lxmfLog := flag.Int("lxmf-log", 5, "lxmf log level 1-7 (5+= inbound decode trace)")
	propMode := flag.String("prop", "", "propagation: list reachable nodes, auto pick one, or node hash hex")
	propWait := flag.Duration("prop-wait", 90*time.Second, "wait for propagation node announces/paths")
	propRetries := flag.Int("prop-retries", 5, "propagation nodes to try when link or transfer fails")
	flag.Parse()
	setupLogging(*logLevel, *lxmfLog)

	home, err := os.UserHomeDir()
	if err != nil {
		fatal("home dir: %v", err)
	}
	if *configPath == "" {
		p, err := reticulumconfig.GetConfigPath()
		if err != nil {
			fatal("config path: %v", err)
		}
		*configPath = p
	}
	if *identityPath == "" {
		*identityPath = filepath.Join(home, ".reticulum-go", "storage", "identity")
	}

	storage := filepath.Join(home, ".reticulum-go", "storage")
	if err := os.Setenv("RETICULUM_STORAGE_PATH", storage); err != nil {
		fatal("set storage env: %v", err)
	}

	dest, err := hex.DecodeString(strings.TrimSpace(*destHex))
	if err != nil || len(dest) != lxmf.DestinationLength {
		fatal("invalid -dest hash: need %d bytes hex", lxmf.DestinationLength)
	}

	cfg, err := reticulumconfig.LoadConfig(*configPath)
	if err != nil {
		fatal("load config %s: %v", *configPath, err)
	}

	tr := transport.NewTransport(cfg)
	tid, err := identity.LoadOrCreateTransportIdentity("")
	if err != nil {
		fatal("transport identity: %v", err)
	}
	tr.SetIdentity(tid)
	if err := tr.Start(); err != nil {
		fatal("transport start: %v", err)
	}
	defer tr.Close()
	if err := tr.InitializePathRequestHandler(); err != nil {
		fatal("path request handler: %v", err)
	}
	if err := startInterfaces(cfg, tr); err != nil {
		fatal("interfaces: %v", err)
	}

	propRegistry := lxmf.NewPropagationRegistry()
	tr.RegisterAnnounceHandler(propRegistry)

	id, err := loadIdentity(*identityPath)
	if err != nil {
		fatal("identity: %v", err)
	}

	messenger, err := lxmf.NewDeliveryMessenger(id, tr)
	if err != nil {
		fatal("messenger: %v", err)
	}

	appData, err := lxmf.EncodeAnnounceAppDataV5(*displayName, int64(*stampCost))
	if err != nil {
		fatal("announce app data: %v", err)
	}
	messenger.Destination().SetDefaultAppData(appData)

	messenger.SetMessageHandler(nil)

	localHash := messenger.DestinationHash()
	fmt.Printf("local lxmf.delivery: %s\n", hex.EncodeToString(localHash))
	fmt.Printf("default peer:        %s\n", hex.EncodeToString(dest))
	fmt.Printf("waiting %s for interfaces to settle...\n", initialWait)
	time.Sleep(initialWait)

	propFlag := strings.TrimSpace(strings.ToLower(*propMode))
	if propFlag == "list" {
		printPropagationNodes(tr, propRegistry, *propWait, false)
		return
	}

	var propNode *lxmf.PropagationNode
	if propFlag != "" {
		propNode, err = resolvePropagationNode(tr, propRegistry, propFlag, *propWait)
		if err != nil {
			fatal("propagation: %v", err)
		}
		fmt.Printf("propagation node:    %s", hex.EncodeToString(propNode.Hash))
		if propNode.Name != "" {
			fmt.Printf(" (%s)", propNode.Name)
		}
		fmt.Printf(" stamp=%d hops=%d\n", propNode.StampCost, propNode.Hops)
		reachable := propRegistry.AttemptOrder(tr, propNode.Hash, nil, 0)
		fmt.Printf("propagation retries: %d reachable node(s), up to %d attempt(s)\n", len(reachable), *propRetries)
	}

	sess := &chatSession{
		messenger:    messenger,
		transport:    tr,
		defaultDest:  dest,
		replyDest:    append([]byte(nil), dest...),
		stampCost:    *stampCost,
		title:        *title,
		propNode:     propNode,
		propRegistry: propRegistry,
		propRetries:  *propRetries,
	}
	messenger.SetMessageHandler(sess.onInbound)

	if err := announce(messenger); err != nil {
		fatal("announce: %v", err)
	}
	fmt.Printf("announced as %q (stamp=%d)\n", *displayName, *stampCost)

	if *announceEvery > 0 {
		go announceLoop(messenger, *announceEvery)
	}

	if !*noInitialSend {
		if propNode != nil {
			if err := sess.waitForDestinationIdentity(dest, pathWait); err != nil {
				fatal("%v", err)
			}
		} else if err := sess.waitForPeer(dest, pathWait); err != nil {
			fatal("%v", err)
		}
		if err := sess.sendTo(dest, *title, *content); err != nil {
			fatal("initial send: %v", err)
		}
	}

	runChat(sess)
}

func (s *chatSession) onInbound(msg *lxmf.LXMessage, _ common.NetworkInterface) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.replyDest = append([]byte(nil), msg.SourceHash...)

	lxmf.Info("inbound lxmf",
		"from", hex.EncodeToString(msg.SourceHash),
		"title", msg.TitleString(),
		"signature_valid", msg.SignatureValidated,
		"stamp_bytes", len(msg.Stamp),
	)

	fmt.Printf("\n--- inbound from %s ---\n", hex.EncodeToString(msg.SourceHash))
	fmt.Printf("title: %s\n", msg.TitleString())
	fmt.Printf("%s\n", msg.ContentString())
	if !msg.SignatureValidated {
		fmt.Printf("(signature not verified: reason %d)\n", msg.UnverifiedReason)
	}
	if len(msg.Stamp) > 0 {
		valid, err := msg.ValidateStamp(s.stampCost, nil)
		if err != nil {
			fmt.Printf("(stamp check: %v)\n", err)
		} else if !valid {
			fmt.Printf("(stamp invalid for cost %d, value %d)\n", s.stampCost, msg.StampValue)
		} else {
			fmt.Printf("(stamp ok, value %d)\n", msg.StampValue)
		}
	}
	fmt.Print("> ")
}

func (s *chatSession) waitForPeer(dest []byte, timeout time.Duration) error {
	if err := s.transport.RequestPath(dest, "", nil, true); err != nil {
		fmt.Printf("path request: %v\n", err)
	}

	deadline := time.Now().Add(timeout)
	destHex := hex.EncodeToString(dest)
	for {
		idKnown := false
		if peer, _ := identity.Recall(dest); peer != nil {
			idKnown = true
		}
		hasPath := s.transport.HasPath(dest)
		if idKnown && hasPath {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout: recall=%v path=%v (announce/path to %s required)", idKnown, hasPath, destHex)
		}
		fmt.Printf("\rwaiting for peer: identity=%v path=%v   ", idKnown, hasPath)
		time.Sleep(pathPoll)
	}
}

func (s *chatSession) waitForDestinationIdentity(dest []byte, timeout time.Duration) error {
	destHex := hex.EncodeToString(dest)
	deadline := time.Now().Add(timeout)
	for {
		_, err := identity.Recall(dest)
		idKnown := err == nil
		if idKnown {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout: destination identity unknown for %s (announce required)", destHex)
		}
		fmt.Printf("\rwaiting for destination identity...   ")
		time.Sleep(pathPoll)
	}
}

func (s *chatSession) sendTo(dest []byte, title, content string) error {
	s.mu.Lock()
	propNode := s.propNode
	propRegistry := s.propRegistry
	propRetries := s.propRetries
	stampCost := s.stampCost
	s.mu.Unlock()

	if propNode != nil {
		if s.stampCost > 0 {
			fmt.Printf("generating stamp (cost=%d)...\n", stampCost)
		}
		fmt.Printf("propagation send (preferred=%s, up to %d nodes)...\n",
			hex.EncodeToString(propNode.Hash), propRetries)

		msg, err := s.messenger.Compose(dest, title, content, nil)
		if err != nil {
			return err
		}
		var used *lxmf.PropagationNode
		if stampCost > 0 {
			used, err = s.messenger.SendStampedPropagatedWithRetry(msg, propRegistry, propNode.Hash, stampCost, propRetries)
		} else {
			used, err = s.messenger.SendPropagatedWithRetry(msg, propRegistry, propNode.Hash, propRetries)
		}
		if err != nil {
			return err
		}
		s.mu.Lock()
		s.propNode = used
		s.mu.Unlock()
		fmt.Printf("sent via propagation node %s", hex.EncodeToString(used.Hash))
		if used.Name != "" {
			fmt.Printf(" (%s)", used.Name)
		}
		fmt.Printf(" hash=%s\n", hex.EncodeToString(msg.Hash))
		return nil
	}

	if s.stampCost > 0 {
		fmt.Printf("generating stamp (cost=%d)...\n", s.stampCost)
	}

	msg, err := s.messenger.Compose(dest, title, content, nil)
	if err != nil {
		return err
	}

	if s.stampCost > 0 {
		if err := s.messenger.SendStamped(msg, s.stampCost); err != nil {
			return err
		}
	} else if _, err := s.messenger.SendText(dest, title, content); err != nil {
		return err
	}

	fmt.Printf("sent to %s hash=%s stamp_cost=%d\n", hex.EncodeToString(dest), hex.EncodeToString(msg.Hash), s.stampCost)
	return nil
}

func (s *chatSession) sendReply(content string) error {
	s.mu.Lock()
	dest := append([]byte(nil), s.replyDest...)
	s.mu.Unlock()

	if len(dest) != lxmf.DestinationLength {
		return fmt.Errorf("no reply destination")
	}

	if err := s.waitForPeer(dest, 30*time.Second); err != nil {
		return err
	}
	return s.sendTo(dest, s.title, content)
}

func runChat(sess *chatSession) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("listening for lxmf messages (Ctrl+C or /quit to exit)")
	fmt.Print("> ")

	input := make(chan string)
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			input <- scanner.Text()
		}
		close(input)
	}()

	for {
		select {
		case <-sig:
			fmt.Println("\nexiting")
			return
		case line, ok := <-input:
			if !ok {
				fmt.Println("\nexiting")
				return
			}
			line = strings.TrimSpace(line)
			if line == "" {
				fmt.Print("> ")
				continue
			}
			if line == "/quit" || line == "/exit" {
				fmt.Println("exiting")
				return
			}
			if err := sess.sendReply(line); err != nil {
				fmt.Fprintf(os.Stderr, "send failed: %v\n", err)
			}
			fmt.Print("> ")
		}
	}
}

func announce(messenger *lxmf.Messenger) error {
	return messenger.Destination().Announce(false, nil, nil)
}

func announceLoop(messenger *lxmf.Messenger, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		if err := announce(messenger); err != nil {
			fmt.Fprintf(os.Stderr, "re-announce failed: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "re-announced lxmf.delivery\n")
		}
	}
}

func loadIdentity(path string) (*identity.Identity, error) {
	if st, err := os.Stat(path); err == nil && !st.IsDir() {
		return identity.FromFile(path)
	}
	id, err := identity.NewIdentity()
	if err != nil {
		return nil, err
	}
	if err := id.ToFile(path); err != nil {
		return nil, fmt.Errorf("save new identity to %s: %w", path, err)
	}
	fmt.Printf("created identity at %s\n", path)
	return id, nil
}

func startInterfaces(cfg *common.ReticulumConfig, tr *transport.Transport) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	ctx := &interfaces.FromConfigContext{
		I2PStoragePath: filepath.Join(home, ".reticulum-go", "storage"),
		TransportID:    tr.TransportIdentityHash(),
		RegisterPeer: func(name string, peer common.NetworkInterface) error {
			return tr.RegisterInterface(name, peer)
		},
	}

	started := 0
	for name, ifaceCfg := range cfg.Interfaces {
		if ifaceCfg == nil || !ifaceCfg.Enabled {
			continue
		}
		iface, err := interfaces.NewFromConfigWithContext(name, ifaceCfg, ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "skip interface %q: %v\n", name, err)
			continue
		}
		iface.SetPacketCallback(func(data []byte, ni common.NetworkInterface) {
			tr.HandlePacket(data, ni)
		})
		if err := iface.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "start interface %q: %v\n", name, err)
			continue
		}
		ni, ok := iface.(common.NetworkInterface)
		if !ok {
			continue
		}
		if err := tr.RegisterInterface(name, ni); err != nil {
			return err
		}
		fmt.Printf("interface up: %s (%s)\n", name, ifaceCfg.Type)
		started++
	}
	if started == 0 {
		return fmt.Errorf("no interfaces started from %s", cfg.ConfigPath)
	}
	return nil
}

func resolvePropagationNode(tr *transport.Transport, reg *lxmf.PropagationRegistry, mode string, wait time.Duration) (*lxmf.PropagationNode, error) {
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode == "auto" {
		fmt.Printf("waiting up to %s for a reachable propagation node...\n", wait)
		nodes, err := reg.WaitFor(tr, 1, wait)
		if err != nil {
			return nil, err
		}
		node, ok := reg.PickRandom(func(n *lxmf.PropagationNode) bool {
			for _, reachable := range nodes {
				if hex.EncodeToString(reachable.Hash) == hex.EncodeToString(n.Hash) {
					return true
				}
			}
			return false
		})
		if !ok {
			return nil, lxmf.ErrNoPropagationNode
		}
		return node, nil
	}

	hash, err := hex.DecodeString(strings.TrimSpace(mode))
	if err != nil || len(hash) != lxmf.DestinationLength {
		return nil, fmt.Errorf("invalid -prop hash: need %d bytes hex or 'auto' or 'list'", lxmf.DestinationLength)
	}

	fmt.Printf("waiting up to %s for propagation node %s...\n", wait, hex.EncodeToString(hash))
	deadline := time.Now().Add(wait)
	for {
		for _, n := range reg.List() {
			if hex.EncodeToString(n.Hash) == hex.EncodeToString(hash) {
				if tr.HasPath(n.Hash) {
					copy := *n
					copy.Hash = append([]byte(nil), n.Hash...)
					return &copy, nil
				}
			}
		}
		if err := tr.RequestPath(hash, "", nil, true); err != nil {
			fmt.Printf("path request: %v\n", err)
		}
		if tr.HasPath(hash) {
			return &lxmf.PropagationNode{
				Hash:      append([]byte(nil), hash...),
				StampCost: lxmf.PropagationStampCostMin,
				LastSeen:  time.Now(),
			}, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("%w: %s", lxmf.ErrNoPropagationNode, hex.EncodeToString(hash))
		}
		time.Sleep(pathPoll)
	}
}

func printPropagationNodes(tr *transport.Transport, reg *lxmf.PropagationRegistry, wait time.Duration, requireReachable bool) {
	fmt.Printf("listening up to %s for propagation node announces...\n", wait)
	deadline := time.Now().Add(wait)
	for {
		nodes := reg.List()
		reachable := 0
		for _, n := range nodes {
			if tr.HasPath(n.Hash) {
				reachable++
			}
		}
		fmt.Printf("\rpropagation nodes: heard=%d reachable=%d   ", len(nodes), reachable)
		if len(nodes) > 0 && (!requireReachable || reachable > 0) {
			break
		}
		if time.Now().After(deadline) {
			fmt.Println()
			if len(nodes) == 0 {
				fmt.Println("no propagation nodes heard")
				return
			}
			break
		}
		time.Sleep(pathPoll)
	}
	fmt.Println()
	for _, n := range reg.List() {
		path := tr.HasPath(n.Hash)
		name := n.Name
		if name == "" {
			name = "-"
		}
		fmt.Printf("  %s  name=%q  stamp=%d  hops=%d  path=%v\n",
			hex.EncodeToString(n.Hash), name, n.StampCost, n.Hops, path)
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "lxmf-send: "+format+"\n", args...)
	os.Exit(1)
}

func setupLogging(rnsLevel, lxmfLevel int) {
	rnsdebug.Init()
	if rnsLevel < 0 {
		rnsLevel = 0
	}
	if rnsLevel > 7 {
		rnsLevel = 7
	}
	if lxmfLevel < 1 {
		lxmfLevel = 1
	}
	if lxmfLevel > 7 {
		lxmfLevel = 7
	}
	rnsdebug.SetDebugLevel(rnsLevel)
	lxmf.MirrorRNSDebug(false)
	lxmf.SetLogLevel(lxmfLevel)
	if rnsLevel == 0 {
		fmt.Fprintln(os.Stderr, "logging: reticulum=silent lxmf=", lxmfLevel)
	} else {
		fmt.Fprintf(os.Stderr, "logging: reticulum=%d lxmf=%d\n", rnsLevel, lxmfLevel)
	}
}
