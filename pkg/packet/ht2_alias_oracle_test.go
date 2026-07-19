package packet

import (
	"bytes"
	"testing"
)

// TestOracle_UnpackUpgradeHT2PackEqualsFreshPack catches Raw aliasing when
// Unpack views DestinationHash as a subslice of Raw and Pack reuses Raw.
func TestOracle_UnpackUpgradeHT2PackEqualsFreshPack(t *testing.T) {
	dest := bytes.Repeat([]byte{0x11}, TruncatedHashLength)
	tid := bytes.Repeat([]byte{0x22}, TruncatedHashLength)
	data := []byte("oracle-payload")

	ht1 := &Packet{
		HeaderType:      HeaderType1,
		PacketType:      PacketTypeData,
		DestinationType: DestinationSingle,
		DestinationHash: append([]byte(nil), dest...),
		Data:            append([]byte(nil), data...),
	}
	if err := ht1.Pack(); err != nil {
		t.Fatal(err)
	}

	// Oversized backing buffer so Pack reuses Raw capacity after Unpack.
	rawBuf := make([]byte, len(ht1.Raw), 256)
	copy(rawBuf, ht1.Raw)
	up := &Packet{Raw: rawBuf}
	if err := up.Unpack(); err != nil {
		t.Fatal(err)
	}
	up.HeaderType = HeaderType2
	up.TransportType = PropagationTransport
	up.TransportID = append([]byte(nil), tid...)
	if err := up.Pack(); err != nil {
		t.Fatal(err)
	}

	fresh := &Packet{
		HeaderType:      HeaderType2,
		PacketType:      PacketTypeData,
		TransportType:   PropagationTransport,
		DestinationType: DestinationSingle,
		DestinationHash: append([]byte(nil), dest...),
		TransportID:     append([]byte(nil), tid...),
		Data:            append([]byte(nil), data...),
	}
	if err := fresh.Pack(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(up.Raw, fresh.Raw) {
		t.Fatalf("HT1 unpack to HT2 pack aliased Raw\n got=%x\nwant=%x", up.Raw, fresh.Raw)
	}
}
