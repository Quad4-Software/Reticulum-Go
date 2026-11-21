// SPDX-License-Identifier: LicenseRef-Reticulum
package codec2_test

import (
	"encoding/hex"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/internal/lxsttest"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/audio/codec2"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
)

func TestCodec2WirePythonDecode(t *testing.T) {
	if !codec2.Native() {
		t.Skip("codec2 wire reference requires lxst_native build tag")
	}
	py := lxsttest.Python(t)
	params := proto.ProfileParams(proto.ProfileBandwidthLow)
	enc, err := codec2.NewEncoder(codec2.Config{
		Bitrate:      params.Bitrate,
		Header:       proto.Codec2HeaderForBitrate(params.Bitrate),
		FrameSamples: params.FrameSamples(),
		PlaySamples:  params.PlaybackFrameSamples(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = enc.Close() }()

	pcm := make([]int16, enc.FrameSamples())
	for i := range pcm {
		pcm[i] = int16((i % 40) * 200)
	}
	body, err := enc.Encode(pcm)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := proto.PackFrame(proto.CodecCodec2, body)
	if err != nil {
		t.Fatal(err)
	}

	script := filepath.Join(lxsttest.RepoRoot(t), "testdata", "lxst", "codec2_decode.py")
	out, err := exec.Command(py, script, hex.EncodeToString(wire)).Output()
	if err != nil {
		t.Fatalf("python decode: %v", err)
	}
	var res struct {
		OK      bool   `json:"ok"`
		Samples int    `json:"samples"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if !res.OK {
		t.Fatalf("python codec2 decode failed: %s", res.Error)
	}
	if res.Samples != params.FrameSamples() {
		t.Fatalf("samples %d want %d", res.Samples, params.FrameSamples())
	}
}

func TestCodec2HeaderSwitchDecode(t *testing.T) {
	params := proto.ProfileParams(proto.ProfileBandwidthVeryLow)
	enc, err := codec2.NewEncoder(codec2.Config{
		Bitrate:      params.Bitrate,
		Header:       proto.Codec2HeaderForBitrate(params.Bitrate),
		FrameSamples: params.FrameSamples(),
		PlaySamples:  params.PlaybackFrameSamples(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = enc.Close() }()
	dec, err := codec2.NewDecoder(codec2.Config{
		Bitrate:      params.Bitrate,
		Header:       proto.Codec2HeaderForBitrate(params.Bitrate),
		FrameSamples: params.FrameSamples(),
		PlaySamples:  params.PlaybackFrameSamples(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dec.Close() }()

	pcm := make([]int16, enc.FrameSamples())
	pkt, err := enc.Encode(pcm)
	if err != nil {
		t.Fatal(err)
	}
	if pkt[0] != proto.Codec2Header1600 {
		t.Fatalf("header %x", pkt[0])
	}
	out, err := dec.Decode(pkt)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != params.PlaybackFrameSamples() {
		t.Fatalf("samples %d", len(out))
	}

	low := proto.ProfileParams(proto.ProfileBandwidthLow)
	enc2, err := codec2.NewEncoder(codec2.Config{
		Bitrate:      low.Bitrate,
		Header:       proto.Codec2HeaderForBitrate(low.Bitrate),
		FrameSamples: low.FrameSamples(),
		PlaySamples:  low.PlaybackFrameSamples(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = enc2.Close() }()
	pcm2 := make([]int16, enc2.FrameSamples())
	pkt2, err := enc2.Encode(pcm2)
	if err != nil {
		t.Fatal(err)
	}
	out2, err := dec.Decode(pkt2)
	if err != nil {
		t.Fatal(err)
	}
	if len(out2) != low.PlaybackFrameSamples() {
		t.Fatalf("switched samples %d", len(out2))
	}
}
