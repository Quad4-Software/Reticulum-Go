// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"bytes"
	"strings"
	"testing"
)

const fuzzMaxBytes = 256 << 10

func clip(b []byte, max int) []byte {
	if len(b) <= max {
		return b
	}
	return b[:max]
}

func FuzzLXMF_Unpack(f *testing.F) {
	f.Add([]byte{})
	f.Add(bytes.Repeat([]byte{0xFF}, 96))
	f.Fuzz(func(t *testing.T, data []byte) {
		data = clip(data, fuzzMaxBytes)
		_, _ = Unpack(data, nil)
	})
}

func FuzzLXMF_SplitUnpack(f *testing.F) {
	dst := bytes.Repeat([]byte{0xAB}, DestinationLength)
	f.Add(dst, []byte{})
	f.Add(dst, bytes.Repeat([]byte{0x01}, 128))
	f.Fuzz(func(t *testing.T, dstRaw, inner []byte) {
		if len(dstRaw) < DestinationLength {
			pad := make([]byte, DestinationLength)
			copy(pad, dstRaw)
			dstRaw = pad
		} else {
			dstRaw = dstRaw[:DestinationLength]
		}
		inner = clip(inner, fuzzMaxBytes)
		_, _ = UnpackFromBytes(dstRaw, inner, nil)
	})
}

func FuzzLXMF_DecodePaperURI(f *testing.F) {
	f.Add("")
	f.Add("https://wrong")
	f.Add("lxm://")
	f.Add("lxm://abcd")
	f.Fuzz(func(t *testing.T, uri string) {
		if len(uri) > 65536 {
			uri = uri[:65536]
		}
		_, _ = DecodePaperURI(uri)
	})
}

func FuzzLXMF_AnnounceAndPNAppData(f *testing.F) {
	f.Add([]byte("legacy-plain-name"))
	f.Add([]byte{0x92, 0xA1, 'x', 0x08})
	f.Fuzz(func(t *testing.T, appData []byte) {
		appData = clip(appData, maxAnnounceAppDataBytes)
		_, _ = DisplayNameFromAppData(appData)
		_, _, _ = StampCostFromAppData(appData)
		_, _, _ = CompressionSupportFromAppData(appData)
		_, _, _ = FeatureSupportFromAppData(appData)
		_ = PNAnnounceDataIsValid(appData)
		_, _, _ = PNStampCostFromAppData(appData)
		_, _, _ = PNNameFromAppData(appData)
	})
}

func FuzzLXMF_ContainerUnpack(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0x80})
	f.Fuzz(func(t *testing.T, data []byte) {
		data = clip(data, fuzzMaxBytes)
		_, _, _ = UnpackContainer(data, nil)
	})
}

func FuzzLXMF_StamperWorkblock(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte("material"))
	f.Fuzz(func(t *testing.T, material []byte) {
		material = clip(material, 4096)
		rounds := 1 + len(material)%48
		_, _ = StampWorkblock(material, rounds)
	})
}

func FuzzLXMF_StamperValidValue(f *testing.F) {
	f.Add([]byte{}, []byte{}, 0)
	f.Fuzz(func(t *testing.T, wb, stamp []byte, cost int) {
		wb = clip(wb, 256*64)
		stamp = clip(stamp, 64)
		if cost < 0 {
			cost = -cost
		}
		cost %= 300
		_ = StampValid(stamp, cost, wb)
		if len(stamp) == StampSize && len(wb) > 0 {
			_ = StampValue(wb, stamp)
		}
	})
}

func FuzzLXMF_PNStampCheck(f *testing.F) {
	f.Add([]byte{})
	f.Add(bytes.Repeat([]byte{0x01}, 200))
	f.Fuzz(func(t *testing.T, transient []byte) {
		transient = clip(transient, fuzzMaxBytes)
		cost := 0
		if len(transient) > 0 {
			cost = int(transient[0]) % 257
		}
		_, _, _, _ = ValidatePNStamp(transient, cost)
	})
}

func FuzzLXMF_PeeringKeyCheck(f *testing.F) {
	f.Add([]byte{}, []byte{})
	f.Fuzz(func(t *testing.T, pid, pkey []byte) {
		pid = clip(pid, 1024)
		pkey = clip(pkey, StampSize*2)
		cost := 0
		if len(pid) > 0 && len(pkey) > 0 {
			cost = (int(pid[0]) + int(pkey[0])) % 257
		}
		_ = ValidatePeeringKey(pid, pkey, cost)
	})
}

func FuzzLXMF_MessageStoreFilename(f *testing.F) {
	f.Add("")
	f.Add("a_b_c")
	f.Add("deadbeef0000000000000000000000000000000000000000000000000000_1700000000.5_42")
	f.Fuzz(func(t *testing.T, name string) {
		if len(name) > 4096 {
			name = name[:4096]
		}
		_, _, _, _ = ParseMessageStoreFilename(name)
	})
}

func FuzzLXMF_ConfigParse(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte("[lxmf]\ndisplay_name = x\n"))
	f.Fuzz(func(t *testing.T, raw []byte) {
		raw = clip(raw, fuzzMaxBytes)
		_, _ = ParseConfig(bytes.NewReader(raw))
	})
}

func FuzzLXMF_PaperURIRoundTrip(f *testing.F) {
	f.Add([]byte{0x01})
	f.Add(bytes.Repeat([]byte{0x42}, 32))
	f.Fuzz(func(t *testing.T, payload []byte) {
		payload = clip(payload, PaperMDU+1)
		uri, err := PaperURI(payload)
		if err != nil {
			return
		}
		if len(uri) > 65536 {
			return
		}
		got, err := DecodePaperURI(uri)
		if err != nil {
			t.Fatalf("DecodePaperURI after PaperURI: %v", err)
		}
		if !bytes.Equal(got, payload) {
			t.Fatal("paper URI round-trip mismatch")
		}
		_, _ = DecodePaperURI(strings.ToUpper(uri))
	})
}

func FuzzLXMF_ReadSectionedKV(f *testing.F) {
	f.Add([]byte("# c\n[k]\nv=1\n"))
	f.Fuzz(func(t *testing.T, raw []byte) {
		raw = clip(raw, fuzzMaxBytes)
		_, _ = readSectionedKV(bytes.NewReader(raw))
	})
}
