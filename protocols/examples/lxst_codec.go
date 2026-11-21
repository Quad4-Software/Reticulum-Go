//go:build ignore

// SPDX-License-Identifier: LicenseRef-Reticulum

// LXST wire codec example: pack signalling and audio frames.
package main

import (
	"fmt"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
)

func main() {
	signals := []int{
		proto.StatusAvailable,
		proto.SignalPreferredProfile(proto.ProfileQualityMedium),
		proto.SignalPreferredMode(proto.ModeFullDuplex),
	}
	sigWire, err := proto.PackSignalling(signals)
	if err != nil {
		fmt.Println("pack signalling:", err)
		return
	}
	pkt, err := proto.Unpack(sigWire)
	if err != nil {
		fmt.Println("unpack:", err)
		return
	}
	fmt.Printf("signalling: %v\n", pkt.Signals)

	payload := []byte{0xde, 0xad, 0xbe, 0xef}
	frameWire, err := proto.PackFrame(proto.CodecOpus, payload)
	if err != nil {
		fmt.Println("pack frame:", err)
		return
	}
	framePkt, err := proto.Unpack(frameWire)
	if err != nil {
		fmt.Println("unpack frame:", err)
		return
	}
	codec, got, err := proto.SplitFrame(framePkt.Frames[0])
	if err != nil {
		fmt.Println("split:", err)
		return
	}
	fmt.Printf("frame codec=%d payload=%x\n", codec, got)

	id := make([]byte, proto.IdentityHashLen)
	for i := range id {
		id[i] = byte(i + 1)
	}
	fmt.Printf("telephony hash: %x\n", proto.TelephonyHash(id))
}
