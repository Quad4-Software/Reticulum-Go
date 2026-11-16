// SPDX-License-Identifier: LicenseRef-Reticulum
package session

import (
	"context"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/resource"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/rnv"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/rnv/proto"
)

const largeClipProgressThreshold = 1 << 20

// ClipProgress reports outbound clip transfer progress.
type ClipProgress func(sent, total uint64)

// SendClip offers a clip, waits for ACCEPT, then transfers via Resource.
func (c *Conn) SendClip(ctx context.Context, data []byte, meta proto.ClipMeta, progress ClipProgress) error {
	if err := c.requireHandshake(); err != nil {
		return err
	}
	if !c.rateClipOK() {
		return rnv.ErrRateLimited
	}
	if meta.Codec == 0 {
		meta.Codec = proto.CodecOpaque
	}
	meta.Size = uint64(len(data))
	max := c.effectiveClipMax()
	if max == 0 {
		return rnv.ErrCapacity
	}
	if err := rnv.ValidateClipMeta(meta, max); err != nil {
		return err
	}
	if uint64(len(data)) > max {
		return rnv.ErrClipTooLarge
	}
	if len(data) > largeClipProgressThreshold && progress == nil {
		return rnv.ErrProgressRequired
	}
	if len(meta.ID) == 0 {
		meta.ID = contentHash(data)
	}
	if len(meta.SHA256) == 0 {
		meta.SHA256 = contentHash(data)
	}

	ch := make(chan clipResult, 1)
	c.mu.Lock()
	c.pendingClipID = cloneHash(meta.ID)
	c.pendingClipCh = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.pendingClipCh = nil
		c.pendingClipID = nil
		c.mu.Unlock()
	}()

	if err := c.sendEnvelope(proto.NewTyped(proto.TypeClipOffer, meta.ToBody())); err != nil {
		return err
	}

	timeout := c.ep.cfg.ClipTimeout
	if ctx == nil {
		ctx = context.Background()
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return rnv.ErrResourceTimeout
	case res := <-ch:
		if res.err != nil {
			return res.err
		}
	}

	if progress != nil {
		progress(0, meta.Size)
	}
	res, err := resource.New(data, true)
	if err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() {
		done <- c.lnk.SendResource(res)
	}()
	timer2 := time.NewTimer(timeout)
	defer timer2.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer2.C:
		return rnv.ErrResourceTimeout
	case err := <-done:
		if err != nil {
			return err
		}
	}
	if progress != nil {
		progress(meta.Size, meta.Size)
	}
	return c.sendEnvelope(proto.NewTyped(proto.TypeClipDone, meta.ToBody()))
}

func (c *Conn) handleClipOffer(meta proto.ClipMeta) {
	max := c.effectiveClipMax()
	if err := rnv.ValidateClipMeta(meta, max); err != nil {
		_ = c.sendReject(proto.RejectSize, err.Error())
		return
	}
	if max == 0 {
		_ = c.sendReject(proto.RejectCapacity, "clips disabled")
		return
	}
	c.mu.Lock()
	c.pendingClipID = cloneHash(meta.ID)
	ch := make(chan clipResult, 1)
	c.pendingClipCh = ch
	c.mu.Unlock()
	_ = c.sendEnvelope(proto.NewTyped(proto.TypeClipAccept, meta.ToBody()))
	go func() {
		timer := time.NewTimer(c.ep.cfg.ClipTimeout)
		defer timer.Stop()
		select {
		case res := <-ch:
			if res.err != nil || len(res.data) == 0 {
				return
			}
			if h := c.ep.cfg.Handlers.OnClip; h != nil {
				h(c, meta, res.data)
			}
		case <-timer.C:
		}
	}()
}

func (c *Conn) signalClipAccept(meta proto.ClipMeta) {
	c.mu.Lock()
	ch := c.pendingClipCh
	c.mu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- clipResult{meta: meta}:
	default:
	}
}

func (c *Conn) signalClipDone(meta proto.ClipMeta) {
	_ = meta
}
