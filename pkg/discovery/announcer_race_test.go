// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package discovery

import (
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/transport"
)

func TestInterfaceAnnouncerStartStopRace(t *testing.T) {
	tr := transport.NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	tr.SetIdentity(id)
	ann, err := NewInterfaceAnnouncer(tr, &common.ReticulumConfig{EnableTransport: true}, id)
	if err != nil {
		t.Fatal(err)
	}
	ann.jobInterval = 5 * time.Millisecond

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 20 {
				ann.Start()
				ann.Stop()
			}
		})
	}
	wg.Wait()
}
