//go:build ignore
// +build ignore

package main

import (
	"encoding/hex"
	"log"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/mf"
)

const (
	exampleSenderHash = "0123456789abcdef0123456789abcdef"
	exampleMessage    = "Hello, Reticulum!"
)

func main() {
	senderHash, _ := hex.DecodeString(exampleSenderHash)
	msg := &mf.Message{
		SenderHash: senderHash,
		Text:       exampleMessage,
	}

	packed, err := msg.Pack()
	if err != nil {
		log.Fatal(err)
	}

	unpacked, err := mf.Unpack(packed)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Original text: %s", msg.Text)
	log.Printf("Unpacked text: %s", unpacked.Text)
	log.Printf("Packet size: %d bytes (sender hash: %d bytes, text: %d bytes)", len(packed), len(msg.SenderHash), len(msg.Text))
}
