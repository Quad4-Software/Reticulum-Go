// Command gen_rns_wire_vectors writes pkg/packet/testdata/rns_wire_vectors.json.
//
// Usage:
//
//	go run ./scripts/gen_rns_wire_vectors
package main

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"quad4/reticulum-go/pkg/announce"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/packet"
	"quad4/reticulum-go/pkg/resource"
)

const destName = "oracleapp.node"

type wirePacket struct {
	Name            string `json:"name"`
	RawHex          string `json:"raw_hex"`
	HashHex         string `json:"hash_hex"`
	Hops            byte   `json:"hops"`
	Context         byte   `json:"context"`
	Flags           byte   `json:"flags"`
	HeaderType      byte   `json:"header_type"`
	PacketType      byte   `json:"packet_type"`
	DestinationType byte   `json:"destination_type"`
	TransportType   byte   `json:"transport_type"`
}

type wireFile struct {
	IdentityPrvHex  string       `json:"identity_prv_hex"`
	IdentityHash    string       `json:"identity_hash"`
	IdentityPub     string       `json:"identity_pub"`
	SingleDestHash  string       `json:"single_dest_hash"`
	NameHash        string       `json:"name_hash"`
	RandomHash      string       `json:"random_hash"`
	AnnounceSig     string       `json:"announce_sig"`
	AnnouncePayload string       `json:"announce_payload"`
	PathRequestDest string       `json:"path_request_dest"`
	ResourceAdvHex  string       `json:"resource_adv_hex"`
	Packets         []wirePacket `json:"packets"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "gen_rns_wire_vectors: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	seed := sha512.Sum512([]byte("reticulum-rns-wire-oracle-seed"))
	id, err := identity.FromBytes(seed[:64])
	if err != nil {
		return fmt.Errorf("identity: %w", err)
	}
	prv, err := id.GetPrivateKey()
	if err != nil {
		return fmt.Errorf("private key: %w", err)
	}
	pub := id.GetPublicKey()
	destHash := announce.DestinationHash(id, destName)
	nameHashFull := sha256.Sum256([]byte(destName))
	nameHash := nameHashFull[:announce.NameHashSize]
	randomHashFull := sha256.Sum256([]byte("oracle-random-hash"))
	randomHash := randomHashFull[:announce.RandomHashSize]
	appData := []byte{0x01, 0x02}

	signed := append([]byte{}, destHash...)
	signed = append(signed, pub...)
	signed = append(signed, nameHash...)
	signed = append(signed, randomHash...)
	signed = append(signed, appData...)
	sig, err := id.Sign(signed)
	if err != nil {
		return fmt.Errorf("sign: %w", err)
	}
	payload := append([]byte{}, pub...)
	payload = append(payload, nameHash...)
	payload = append(payload, randomHash...)
	payload = append(payload, sig...)
	payload = append(payload, appData...)

	prName := sha256.Sum256([]byte("rnstransport.path.request"))
	prFull := sha256.Sum256(prName[:announce.NameHashSize])
	prDest := prFull[:announce.AddrHashSize]

	dataPkt, err := packVector(&packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeData,
		DestinationType: packet.DestinationSingle,
		DestinationHash: destHash,
		Context:         packet.ContextNone,
		Data:            []byte("hello hops"),
		Hops:            3,
	}, "data_hops3")
	if err != nil {
		return err
	}
	tagFull := sha256.Sum256([]byte("oracle-path-request-tag"))
	pathPkt, err := packVector(&packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeData,
		DestinationType: packet.DestinationPlain,
		DestinationHash: prDest,
		Context:         packet.ContextNone,
		Data:            append(append([]byte{}, destHash...), tagFull[:16]...),
	}, "path_request")
	if err != nil {
		return err
	}
	announcePkt, err := packVector(&packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeAnnounce,
		DestinationType: packet.DestinationSingle,
		DestinationHash: destHash,
		Context:         packet.ContextNone,
		Data:            payload,
	}, "announce_ht1")
	if err != nil {
		return err
	}

	hash := bytesSeq(32)
	ra := &resource.ResourceAdvertisement{
		TransferSize: 1024,
		DataSize:     2048,
		Parts:        3,
		Hash:         hash,
		Hashmap:      []byte{0x11, 0x22, 0x33, 0x44},
		Flags:        0x27,
	}
	adv, err := ra.Pack(0, resource.DefaultLinkMDU)
	if err != nil {
		return fmt.Errorf("resource adv: %w", err)
	}

	out := wireFile{
		IdentityPrvHex:  hex.EncodeToString(prv),
		IdentityHash:    hex.EncodeToString(id.Hash()),
		IdentityPub:     hex.EncodeToString(pub),
		SingleDestHash:  hex.EncodeToString(destHash),
		NameHash:        hex.EncodeToString(nameHash),
		RandomHash:      hex.EncodeToString(randomHash),
		AnnounceSig:     hex.EncodeToString(sig),
		AnnouncePayload: hex.EncodeToString(payload),
		PathRequestDest: hex.EncodeToString(prDest),
		ResourceAdvHex:  hex.EncodeToString(adv),
		Packets:         []wirePacket{dataPkt, pathPkt, announcePkt},
	}
	body, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	root, err := repoRoot()
	if err != nil {
		return err
	}
	path := filepath.Join(root, "pkg", "packet", "testdata", "rns_wire_vectors.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Println(path)
	return nil
}

func packVector(p *packet.Packet, name string) (wirePacket, error) {
	if err := p.Pack(); err != nil {
		return wirePacket{}, fmt.Errorf("%s pack: %w", name, err)
	}
	return wirePacket{
		Name:            name,
		RawHex:          hex.EncodeToString(p.Raw),
		HashHex:         hex.EncodeToString(p.GetHash()),
		Hops:            p.Hops,
		Context:         p.Context,
		Flags:           p.Raw[0],
		HeaderType:      p.HeaderType,
		PacketType:      p.PacketType,
		DestinationType: p.DestinationType,
		TransportType:   p.TransportType,
	}, nil
}

func bytesSeq(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i)
	}
	return b
}

func repoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found from %s", wd)
		}
		dir = parent
	}
}
