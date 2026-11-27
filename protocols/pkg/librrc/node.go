// SPDX-License-Identifier: LicenseRef-Reticulum
package librrc

import (
	"fmt"
	"path/filepath"
	"sync"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/interfaces"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/reticulumconfig"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
)

type nodeRecord struct {
	handle    uint64
	cfg       *common.ReticulumConfig
	transport *transport.Transport
	identity  *identity.Identity
	ifaces    []interfaces.Interface
	started   bool
}

var (
	runtimeMu sync.RWMutex
	handles   = newHandleTable()
)

func nodeByHandle(id uint64) (*nodeRecord, error) {
	ref, err := handles.get(id, kindNode)
	if err != nil {
		return nil, err
	}
	return ref.(*nodeRecord), nil
}

// NodeCreate builds a node handle. An empty configPath uses in-memory
// defaults (no interfaces). A non-empty path loads a Reticulum-Go config
// file and attaches every enabled [interfaces] entry. Individual interface
// failures are logged and skipped. Shared-instance attach is always off:
// librrc runs its own in-process transport.
func NodeCreate(configPath string) (uint64, int) {
	var cfg *common.ReticulumConfig
	var configDir string
	if configPath == "" {
		cfg = common.DefaultConfig()
	} else {
		if err := validatePath(configPath); err != nil {
			return 0, setLastError(err)
		}
		loaded, err := reticulumconfig.LoadConfig(configPath)
		if err != nil {
			return 0, setLastError(fmt.Errorf("%w: %v", errIO, err))
		}
		cfg = loaded
		configDir = filepath.Dir(configPath)
	}
	cfg.ShareInstance = false

	rec := &nodeRecord{
		cfg:       cfg,
		transport: transport.NewTransport(cfg),
	}
	runtimeMu.Lock()
	id := handles.insert(kindNode, rec)
	rec.handle = id
	runtimeMu.Unlock()

	if configDir != "" {
		attachConfiguredInterfaces(rec, configDir)
	}
	return id, OK
}

func NodeStart(nodeHandle uint64) int {
	rec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return setLastError(err)
	}
	if rec.started {
		return OK
	}
	if err := rec.transport.Start(); err != nil {
		return setLastError(fmt.Errorf("%w: %v", errInternal, err))
	}
	rec.started = true
	return OK
}

func NodeStop(nodeHandle uint64) int {
	rec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return setLastError(err)
	}
	if !rec.started {
		return OK
	}
	_ = rec.transport.Close()
	rec.started = false
	return OK
}

func NodeDestroy(nodeHandle uint64) int {
	rec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return setLastError(err)
	}
	for _, iface := range rec.ifaces {
		_ = iface.Stop()
	}
	rec.ifaces = nil
	if rec.started {
		_ = rec.transport.Close()
		rec.started = false
	}
	runtimeMu.Lock()
	handles.delete(nodeHandle)
	runtimeMu.Unlock()
	return OK
}

func NodeSetIdentity(nodeHandle, identityHandle uint64) int {
	rec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return setLastError(err)
	}
	idRec, err := identityByHandle(identityHandle)
	if err != nil {
		return setLastError(err)
	}
	rec.identity = idRec.identity
	return OK
}

func NodeTransport(nodeHandle uint64) (*transport.Transport, int) {
	rec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return nil, setLastError(err)
	}
	if rec.transport == nil {
		return nil, setLastError(errState)
	}
	return rec.transport, OK
}

func nodeIdentity(nodeHandle uint64) (*identity.Identity, int) {
	rec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return nil, setLastError(err)
	}
	if rec.identity == nil {
		return nil, setLastError(errState)
	}
	return rec.identity, OK
}
