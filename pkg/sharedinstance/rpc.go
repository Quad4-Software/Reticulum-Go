// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package sharedinstance

import (
	"encoding/hex"
	"net"
	"strconv"
	"sync"

	"quad4/msgpack/v5/pkg/msgpack"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/transport"
)

// RPCHandler serves shared-instance control requests.
type RPCHandler struct {
	Transport *transport.Transport
}

func (h *RPCHandler) Handle(call map[string]any) any {
	if h == nil || h.Transport == nil {
		return nil
	}
	if get, ok := call["get"].(string); ok {
		switch get {
		case "path_table":
			var maxHops *int
			if mh, ok := call["max_hops"].(int64); ok {
				v := int(mh)
				maxHops = &v
			} else if mh, ok := call["max_hops"].(int); ok {
				maxHops = &mh
			}
			return h.Transport.GetPathTable(maxHops)
		case "interface_stats":
			return h.Transport.GetInterfaceStatsRPC()
		case "rate_table":
			return h.Transport.GetRateTableRPC()
		case "next_hop_if_name":
			return h.Transport.GetNextHopIfNameRPC(decodeHash(call["destination_hash"]))
		case "next_hop":
			return h.Transport.GetNextHopRPC(decodeHash(call["destination_hash"]))
		case "first_hop_timeout":
			return h.Transport.GetFirstHopTimeoutRPC(decodeHash(call["destination_hash"]))
		case "link_count":
			return h.Transport.GetLinkCountRPC()
		case "blackholed_identities":
			return h.Transport.GetBlackholedIdentitiesRPC()
		case "is_blackholed":
			return h.Transport.IsBlackholedRPC(decodeHash(call["identity_hash"]))
		}
	}
	if drop, ok := call["drop"].(string); ok {
		switch drop {
		case "path":
			return h.Transport.DropPathRPC(decodeHash(call["destination_hash"]))
		case "all_via":
			return h.Transport.DropAllViaRPC(decodeHash(call["destination_hash"]))
		case "announce_queues":
			return h.Transport.DropAnnounceQueuesRPC()
		}
	}
	if hash := decodeHash(call["blackhole_identity"]); hash != nil {
		until, _ := call["until"].(float64)
		reason, _ := call["reason"].(string)
		tab := h.Transport.BlackholeTable()
		if tab != nil {
			ok, _ := tab.Add(hash, until, reason)
			return ok
		}
		return false
	}
	if hash := decodeHash(call["unblackhole_identity"]); hash != nil {
		tab := h.Transport.BlackholeTable()
		if tab != nil {
			ok, _ := tab.Remove(hash)
			return ok
		}
		return false
	}
	return nil
}

func decodeHash(v any) []byte {
	switch h := v.(type) {
	case []byte:
		return h
	case string:
		b, err := hex.DecodeString(h)
		if err != nil {
			return nil
		}
		return b
	default:
		return nil
	}
}

// RPCServer listens for authenticated msgpack RPC calls from local clients.
type RPCServer struct {
	listener net.Listener
	authkey  []byte
	handler  *RPCHandler
	wg       sync.WaitGroup
	done     chan struct{}
}

// StartRPCServer binds the instance control port and serves requests.
func StartRPCServer(cfg *common.ReticulumConfig, tr *transport.Transport) (*RPCServer, error) {
	if cfg == nil || tr == nil {
		return nil, nil
	}
	authkey := cfg.RPCKey
	if len(authkey) == 0 {
		authkey = tr.RPCAuthKey()
	}
	if len(authkey) == 0 {
		return nil, nil
	}
	var (
		ln  net.Listener
		err error
	)
	useUnix := cfg.SharedInstanceType == common.SharedInstanceUnix
	if useUnix {
		name := cfg.InstanceName
		if name == "" {
			name = "default"
		}
		ln, err = net.Listen("unix", "@"+"rns/"+name+"/rpc")
	} else {
		ln, err = net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(cfg.InstanceControlPort)))
	}
	if err != nil {
		return nil, err
	}
	s := &RPCServer{
		listener: ln,
		authkey:  authkey,
		handler:  &RPCHandler{Transport: tr},
		done:     make(chan struct{}),
	}
	s.wg.Add(1)
	go s.serve()
	debug.Log(debug.DebugInfo, "Shared instance RPC listening", "addr", ln.Addr().String())
	return s, nil
}

func (s *RPCServer) serve() {
	defer s.wg.Done()
	for {
		select {
		case <-s.done:
			return
		default:
		}
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.done:
				return
			default:
			}
			continue
		}
		s.wg.Add(1)
		go func(c net.Conn) {
			defer s.wg.Done()
			defer c.Close()
			if err := AuthenticateServer(c, s.authkey); err != nil {
				debug.Log(debug.DebugError, "Shared instance RPC auth failed", "error", err)
				return
			}
			payload, err := recvBytes(c, 1<<20)
			if err != nil {
				return
			}
			var call map[string]any
			if err := msgpack.Unmarshal(payload, &call); err != nil {
				return
			}
			resp := s.handler.Handle(call)
			out, err := msgpack.Marshal(resp)
			if err != nil {
				return
			}
			_ = sendBytes(c, out)
		}(conn)
	}
}

func (s *RPCServer) Close() error {
	close(s.done)
	if s.listener != nil {
		_ = s.listener.Close()
	}
	s.wg.Wait()
	return nil
}
