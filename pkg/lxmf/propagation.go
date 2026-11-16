// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"sync"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
)

// PropagationNode is one lxmf.propagation destination discovered from announces.
type PropagationNode struct {
	Hash      []byte
	Name      string
	StampCost int64
	Hops      uint8
	LastSeen  time.Time
}

// PropagationRegistry records propagation node announces on the transport.
type PropagationRegistry struct {
	mu    sync.RWMutex
	nodes map[destID]*PropagationNode
}

// NewPropagationRegistry returns an empty propagation node registry.
func NewPropagationRegistry() *PropagationRegistry {
	return &PropagationRegistry{
		nodes: make(map[destID]*PropagationNode),
	}
}

// AspectFilter implements announce.Handler.
func (r *PropagationRegistry) AspectFilter() []string { return nil }

// ReceivePathResponses implements announce.Handler.
func (r *PropagationRegistry) ReceivePathResponses() bool { return false }

// ReceivedAnnounce implements announce.Handler.
func (r *PropagationRegistry) ReceivedAnnounce(destHash []byte, identAny any, appData []byte, hops uint8) error {
	if len(destHash) != DestinationLength || !PNAnnounceDataIsValid(appData) {
		return nil
	}

	name, ok, err := PNNameFromAppData(appData)
	if err != nil {
		return err
	}
	if !ok {
		name = ""
	}
	cost, ok, err := PNStampCostFromAppData(appData)
	if err != nil {
		return err
	}
	if !ok {
		cost = int64(PropagationStampCostMin)
	}
	if cost < int64(PropagationStampCostMin) {
		cost = int64(PropagationStampCostMin)
	}

	id, err := identityFromAnnounce(identAny, destHash)
	if err != nil || id == nil {
		return nil
	}
	expected, err := PropagationDestinationHash(id)
	if err != nil {
		return err
	}
	if !bytesEqual(expected, destHash) {
		return nil
	}

	key, ok := destIDFrom(destHash)
	if !ok {
		return nil
	}
	node := &PropagationNode{
		Hash:      append([]byte(nil), destHash...),
		Name:      name,
		StampCost: cost,
		Hops:      hops,
		LastSeen:  time.Now(),
	}

	r.mu.Lock()
	r.nodes[key] = node
	r.mu.Unlock()
	return nil
}

func identityFromAnnounce(identAny any, destHash []byte) (*identity.Identity, error) {
	if identAny != nil {
		if id, ok := identAny.(*identity.Identity); ok && id != nil {
			return id, nil
		}
	}
	return identity.Recall(destHash)
}

// PropagationDestinationHash returns the lxmf.propagation hash for id.
func PropagationDestinationHash(id *identity.Identity) ([]byte, error) {
	if id == nil {
		return nil, errors.New("lxmf: nil identity")
	}
	dest, err := destination.New(id, destination.Out, destination.Single, AppName, nil, "propagation")
	if err != nil {
		return nil, err
	}
	return dest.GetHash(), nil
}

// List returns known propagation nodes sorted by name then hash.
func (r *PropagationRegistry) List() []*PropagationNode {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]*PropagationNode, 0, len(r.nodes))
	for _, n := range r.nodes {
		copy := *n
		copy.Hash = append([]byte(nil), n.Hash...)
		out = append(out, &copy)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return bytes.Compare(out[i].Hash, out[j].Hash) < 0
	})
	return out
}

// PickRandom chooses one node that passes pred. When pred is nil, any node matches.
func (r *PropagationRegistry) PickRandom(pred func(*PropagationNode) bool) (*PropagationNode, bool) {
	nodes := r.List()
	if len(nodes) == 0 {
		return nil, false
	}

	candidates := nodes
	if pred != nil {
		candidates = make([]*PropagationNode, 0, len(nodes))
		for _, n := range nodes {
			if pred(n) {
				candidates = append(candidates, n)
			}
		}
	}
	if len(candidates) == 0 {
		return nil, false
	}

	idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(candidates))))
	if err != nil {
		return candidates[0], true
	}
	chosen := candidates[idx.Int64()]
	copy := *chosen
	copy.Hash = append([]byte(nil), chosen.Hash...)
	return &copy, true
}

// WaitFor blocks until at least minReachable propagation nodes have paths or timeout elapses.
func (r *PropagationRegistry) WaitFor(tr *transport.Transport, minReachable int, timeout time.Duration) ([]*PropagationNode, error) {
	if tr == nil {
		return nil, errors.New("lxmf: nil transport")
	}
	if minReachable < 1 {
		minReachable = 1
	}

	deadline := time.Now().Add(timeout)
	for {
		reachable := r.reachable(tr)
		if len(reachable) >= minReachable {
			return reachable, nil
		}
		if time.Now().After(deadline) {
			all := r.List()
			if len(all) == 0 {
				return nil, fmt.Errorf("%w: no propagation nodes heard within %s", ErrNoPropagationNode, timeout)
			}
			return nil, fmt.Errorf("%w: heard %d node(s), %d reachable within %s", ErrNoPropagationNode, len(all), len(reachable), timeout)
		}
		time.Sleep(pathPollInterval)
	}
}

func (r *PropagationRegistry) reachable(tr *transport.Transport) []*PropagationNode {
	out := make([]*PropagationNode, 0)
	for _, n := range r.List() {
		if tr.HasPath(n.Hash) {
			out = append(out, n)
		}
	}
	return out
}

const pathPollInterval = 500 * time.Millisecond

func bytesEqual(a, b []byte) bool {
	return bytes.Equal(a, b)
}
