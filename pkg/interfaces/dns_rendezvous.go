// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !js

package interfaces

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
)

const (
	dnsRendezvousDefaultInterval = 60 * time.Second
	dnsRendezvousBitrate         = 10_000_000
)

// DNSLookupTXT looks up TXT records for a name. Tests inject fakes.
type DNSLookupTXT func(name string) ([]string, error)

// DNSRendezvousEndpoint is one peer discovered from DNS.
type DNSRendezvousEndpoint struct {
	Proto string // udp (default)
	Host  string
	Port  int
}

// ParseRNSTXT parses TXT payloads such as:
//
//	rns=udp://1.2.3.4:4242
//	rns proto=udp host=1.2.3.4 port=4242
func ParseRNSTXT(txt string) (DNSRendezvousEndpoint, bool) {
	txt = strings.TrimSpace(txt)
	if txt == "" {
		return DNSRendezvousEndpoint{}, false
	}
	lower := strings.ToLower(txt)
	if strings.HasPrefix(lower, "rns=") {
		u := strings.TrimSpace(txt[4:])
		return parseRNSURL(u)
	}
	if strings.HasPrefix(lower, "rns ") || lower == "rns" {
		ep := DNSRendezvousEndpoint{Proto: "udp"}
		for part := range strings.FieldsSeq(txt) {
			kv := strings.SplitN(part, "=", 2)
			if len(kv) != 2 {
				continue
			}
			k := strings.ToLower(kv[0])
			v := strings.TrimSpace(kv[1])
			switch k {
			case "proto", "protocol":
				ep.Proto = strings.ToLower(v)
			case "host", "addr", "address":
				ep.Host = v
			case "port":
				p, err := strconv.Atoi(v)
				if err == nil {
					ep.Port = p
				}
			}
		}
		if !validRNSEndpoint(ep) {
			return DNSRendezvousEndpoint{}, false
		}
		if ep.Proto == "" {
			ep.Proto = "udp"
		}
		return ep, true
	}
	if strings.Contains(lower, "://") {
		return parseRNSURL(txt)
	}
	return DNSRendezvousEndpoint{}, false
}

func parseRNSURL(u string) (DNSRendezvousEndpoint, bool) {
	u = strings.TrimSpace(u)
	scheme, rest, ok := strings.Cut(u, "://")
	if !ok {
		return DNSRendezvousEndpoint{}, false
	}
	host, portStr, err := net.SplitHostPort(rest)
	if err != nil {
		return DNSRendezvousEndpoint{}, false
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return DNSRendezvousEndpoint{}, false
	}
	ep := DNSRendezvousEndpoint{
		Proto: strings.ToLower(strings.TrimSpace(scheme)),
		Host:  strings.TrimSpace(host),
		Port:  port,
	}
	if !validRNSEndpoint(ep) {
		return DNSRendezvousEndpoint{}, false
	}
	if ep.Proto == "" {
		ep.Proto = "udp"
	}
	return ep, true
}

func validRNSEndpoint(ep DNSRendezvousEndpoint) bool {
	if ep.Host == "" || ep.Port <= 0 || ep.Port > 65535 {
		return false
	}
	if strings.ContainsAny(ep.Host, " \t\r\n") {
		return false
	}
	proto := ep.Proto
	if proto == "" {
		proto = "udp"
	}
	switch proto {
	case "udp", "tcp":
	default:
		return false
	}
	if ip := net.ParseIP(ep.Host); ip != nil {
		return true
	}
	// Allow DNS names without spaces. Reject bare numbers that are not IPs.
	if strings.Contains(ep.Host, ".") || strings.Contains(ep.Host, ":") {
		return true
	}
	for _, r := range ep.Host {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '-' {
			return true
		}
	}
	return false
}

// DNSRendezvousOptions configures DNS rendezvous.
type DNSRendezvousOptions struct {
	Domain          string
	ListenAddr      string // local UDP bind, default 0.0.0.0:0
	ResolveInterval time.Duration
	LookupTXT       DNSLookupTXT
}

// DNSRendezvousInterface discovers UDP peers from DNS TXT and carries packets
// to the active endpoint. It is a rendezvous underlay, not a DNS tunnel.
type DNSRendezvousInterface struct {
	BaseInterface
	domain     string
	listenAddr *net.UDPAddr
	interval   time.Duration
	lookup     DNSLookupTXT
	conn       *net.UDPConn
	target     *net.UDPAddr
	targetStr  string
	readBuf    []byte
	done       chan struct{}
	stopOnce   sync.Once
	Resolves   atomic.Uint64
	Lookups    atomic.Uint64
}

// NewDNSRendezvousInterface constructs a DNS rendezvous interface.
func NewDNSRendezvousInterface(name string, enabled bool, opts DNSRendezvousOptions) (*DNSRendezvousInterface, error) {
	domain := strings.TrimSpace(opts.Domain)
	if domain == "" {
		return nil, fmt.Errorf("dns rendezvous requires domain")
	}
	listen := opts.ListenAddr
	if listen == "" {
		listen = "0.0.0.0:0"
	}
	laddr, err := net.ResolveUDPAddr("udp", listen)
	if err != nil {
		return nil, err
	}
	interval := opts.ResolveInterval
	if interval <= 0 {
		interval = dnsRendezvousDefaultInterval
	}
	lookup := opts.LookupTXT
	if lookup == nil {
		lookup = net.LookupTXT
	}
	di := &DNSRendezvousInterface{
		BaseInterface: NewBaseInterface(name, common.IFTypeDNSRendezvous, enabled),
		domain:        domain,
		listenAddr:    laddr,
		interval:      interval,
		lookup:        lookup,
		readBuf:       make([]byte, DefaultMTU),
		done:          make(chan struct{}),
	}
	di.In = true
	di.Out = true
	di.MTU = DefaultMTU
	di.Bitrate = dnsRendezvousBitrate
	if enabled {
		if err := di.Start(); err != nil {
			return nil, err
		}
	}
	return di, nil
}

func (di *DNSRendezvousInterface) String() string {
	return fmt.Sprintf("DNSRendezvousInterface[%s/%s]", di.Name, di.domain)
}

func (di *DNSRendezvousInterface) Start() error {
	di.Mutex.Lock()
	if di.Online && di.conn != nil {
		di.Mutex.Unlock()
		return nil
	}
	enabled := di.Enabled
	di.Mutex.Unlock()
	if !enabled {
		return fmt.Errorf("interface not enabled")
	}
	conn, err := net.ListenUDP("udp", di.listenAddr)
	if err != nil {
		return err
	}
	di.Mutex.Lock()
	di.conn = conn
	di.Online = true
	di.Mutex.Unlock()
	if err := di.resolveAndApply(); err != nil {
		debug.Log(debug.DebugVerbose, "DNS rendezvous initial resolve", "name", di.Name, "error", err)
	}
	go di.readLoop()
	go di.resolveLoop()
	return nil
}

func (di *DNSRendezvousInterface) Stop() error {
	di.Mutex.Lock()
	di.Enabled = false
	di.Online = false
	conn := di.conn
	di.conn = nil
	di.Mutex.Unlock()
	di.stopOnce.Do(func() { close(di.done) })
	if conn != nil {
		_ = conn.Close()
	}
	return nil
}

func (di *DNSRendezvousInterface) resolveLoop() {
	ticker := time.NewTicker(di.interval)
	defer ticker.Stop()
	for {
		select {
		case <-di.done:
			return
		case <-ticker.C:
			if err := di.resolveAndApply(); err != nil {
				debug.Log(debug.DebugVerbose, "DNS rendezvous resolve", "name", di.Name, "error", err)
			}
		}
	}
}

func (di *DNSRendezvousInterface) resolveAndApply() error {
	di.Lookups.Add(1)
	txts, err := di.lookup(di.domain)
	if err != nil {
		return err
	}
	var endpoints []DNSRendezvousEndpoint
	for _, t := range txts {
		if ep, ok := ParseRNSTXT(t); ok && ep.Proto == "udp" {
			endpoints = append(endpoints, ep)
		}
	}
	if len(endpoints) == 0 {
		return fmt.Errorf("no rns udp endpoints in TXT for %s", di.domain)
	}
	ep := endpoints[0]
	addr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(ep.Host, strconv.Itoa(ep.Port)))
	if err != nil {
		return err
	}
	key := addr.String()
	di.Mutex.Lock()
	changed := di.targetStr != key
	di.target = addr
	di.targetStr = key
	di.Mutex.Unlock()
	if changed {
		di.Resolves.Add(1)
		debug.Log(debug.DebugInfo, "DNS rendezvous endpoint", "name", di.Name, "target", key)
	}
	return nil
}

func (di *DNSRendezvousInterface) readLoop() {
	for {
		di.Mutex.RLock()
		conn := di.conn
		done := di.done
		di.Mutex.RUnlock()
		if conn == nil {
			return
		}
		select {
		case <-done:
			return
		default:
		}
		_ = conn.SetReadDeadline(time.Now().Add(time.Second))
		n, _, err := conn.ReadFromUDP(di.readBuf)
		if n > 0 {
			payload := append([]byte(nil), di.readBuf[:n]...)
			di.ProcessIncoming(payload)
		}
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			select {
			case <-done:
				return
			default:
			}
			debug.Log(debug.DebugVerbose, "DNS rendezvous read ended", "name", di.Name, "error", err)
			return
		}
	}
}

func (di *DNSRendezvousInterface) ProcessOutgoing(data []byte) error {
	di.Mutex.RLock()
	conn := di.conn
	target := di.target
	online := di.Online
	di.Mutex.RUnlock()
	if !online || conn == nil {
		return fmt.Errorf("dns rendezvous offline")
	}
	if target == nil {
		return fmt.Errorf("dns rendezvous has no endpoint yet")
	}
	_, err := conn.WriteToUDP(data, target)
	return err
}

func (di *DNSRendezvousInterface) Send(data []byte, _ string) error {
	if err := common.RejectReceiveOnly(di); err != nil {
		return err
	}
	masked, err := common.ApplyIFACOutbound(di, data)
	if err != nil {
		return err
	}
	if err := di.ProcessOutgoing(masked); err != nil {
		return err
	}
	di.updateBandwidthStats(uint64(len(masked)))
	return nil
}

func (di *DNSRendezvousInterface) GetConn() net.Conn { return nil }

func (di *DNSRendezvousInterface) SendPathRequest([]byte) error { return nil }

func (di *DNSRendezvousInterface) SendLinkPacket([]byte, []byte, time.Time) error { return nil }

func (di *DNSRendezvousInterface) GetBandwidthAvailable() bool {
	di.Mutex.RLock()
	defer di.Mutex.RUnlock()
	return di.Online && di.conn != nil && di.target != nil
}

// ActiveTarget returns the current UDP endpoint string.
func (di *DNSRendezvousInterface) ActiveTarget() string {
	di.Mutex.RLock()
	defer di.Mutex.RUnlock()
	return di.targetStr
}

// ForceResolve runs one DNS lookup immediately (tests).
func (di *DNSRendezvousInterface) ForceResolve() error {
	return di.resolveAndApply()
}
