// SPDX-License-Identifier: LicenseRef-Reticulum
package proto_test

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/internal/lxsttest"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/phonebook"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
)

type pythonReference struct {
	Version  string             `json:"version"`
	App      string             `json:"app"`
	Aspect   string             `json:"aspect"`
	Status   map[string]int     `json:"status"`
	PrefMode int                `json:"preferred_mode"`
	PrefProf int                `json:"preferred_profile"`
	Codecs   map[string]int     `json:"codecs"`
	Profiles map[string]int     `json:"profiles"`
	Avail    []int              `json:"available_profiles"`
	FrameMs  map[string]int     `json:"frame_times"`
	BufferN  map[string]int     `json:"buffer_frames"`
	Modes    map[string]int     `json:"modes"`
	Opus     map[string]opusRow `json:"opus"`
	Phone    map[string]int     `json:"telephone"`
	Allow    map[string]int     `json:"allow"`
	C2Hdr    map[string]int     `json:"codec2_headers"`
	DestHash string             `json:"dest_hash"`
	Wire     map[string]string  `json:"wire"`
}

type opusRow struct {
	ID       int  `json:"id"`
	Channels int  `json:"channels"`
	Rate     int  `json:"rate"`
	Bitrate  int  `json:"bitrate"`
	Voip     bool `json:"voip"`
}

func loadPythonReference(t *testing.T) pythonReference {
	t.Helper()
	py := lxsttest.Python(t)
	script := filepath.Join(lxsttest.RepoRoot(t), "testdata", "lxst", "lxst_reference.py")
	out, err := exec.Command(py, script).Output()
	if err != nil {
		t.Fatalf("lxst_reference.py: %v", err)
	}
	var o pythonReference
	if err := json.Unmarshal(out, &o); err != nil {
		t.Fatalf("reference json: %v\n%s", err, out)
	}
	return o
}

func TestLXSTConstantsMatchPython(t *testing.T) {
	o := loadPythonReference(t)
	if o.Version != "0.5.4" {
		t.Fatalf("lxst version %s", o.Version)
	}
	if o.App != proto.AppName || o.Aspect != proto.AspectName {
		t.Fatalf("app %s aspect %s", o.App, o.Aspect)
	}
	wantStatus := map[string]int{
		"busy":        proto.StatusBusy,
		"rejected":    proto.StatusRejected,
		"calling":     proto.StatusCalling,
		"available":   proto.StatusAvailable,
		"ringing":     proto.StatusRinging,
		"connecting":  proto.StatusConnecting,
		"established": proto.StatusEstablished,
	}
	for k, want := range wantStatus {
		if o.Status[k] != want {
			t.Fatalf("status %s python %d go %d", k, o.Status[k], want)
		}
	}
	if o.PrefMode != proto.PreferredMode || o.PrefProf != proto.PreferredProfile {
		t.Fatalf("preferred mode %d profile %d", o.PrefMode, o.PrefProf)
	}
	wantProf := map[string]int{
		"ulbw":    proto.ProfileBandwidthUltraLow,
		"vlbw":    proto.ProfileBandwidthVeryLow,
		"lbw":     proto.ProfileBandwidthLow,
		"mq":      proto.ProfileQualityMedium,
		"hq":      proto.ProfileQualityHigh,
		"shq":     proto.ProfileQualityMax,
		"ull":     proto.ProfileLatencyUltraLow,
		"ll":      proto.ProfileLatencyLow,
		"default": proto.DefaultProfile,
	}
	for k, want := range wantProf {
		if o.Profiles[k] != want {
			t.Fatalf("profile %s python %d go %d", k, o.Profiles[k], want)
		}
	}
	if o.Modes["full"] != proto.ModeFullDuplex || o.Modes["half"] != proto.ModeHalfDuplex {
		t.Fatalf("modes %+v", o.Modes)
	}
	if o.Codecs["raw"] != int(proto.CodecRaw) || o.Codecs["opus"] != int(proto.CodecOpus) ||
		o.Codecs["codec2"] != int(proto.CodecCodec2) || o.Codecs["null"] != int(proto.CodecNull) {
		t.Fatalf("codecs %+v", o.Codecs)
	}
	if o.Allow["all"] != int(phonebook.AllowAll) || o.Allow["none"] != int(phonebook.AllowNone) {
		t.Fatalf("allow %+v", o.Allow)
	}
	wantAvail := proto.AvailableProfiles()
	if len(o.Avail) != len(wantAvail) {
		t.Fatalf("available profiles %v want %v", o.Avail, wantAvail)
	}
	for i, p := range wantAvail {
		if o.Avail[i] != p {
			t.Fatalf("available[%d] python %d go %d", i, o.Avail[i], p)
		}
		params := proto.ProfileParams(p)
		if o.FrameMs[strconv.Itoa(p)] != params.FrameMs {
			t.Fatalf("frame ms profile %d python %d go %d", p, o.FrameMs[strconv.Itoa(p)], params.FrameMs)
		}
		if o.BufferN[strconv.Itoa(p)] != params.BufferN {
			t.Fatalf("buffer n profile %d python %d go %d", p, o.BufferN[strconv.Itoa(p)], params.BufferN)
		}
	}
	if proto.NextProfile(proto.ProfileQualityMax) != proto.ProfileLatencyLow {
		t.Fatal("next profile after shq must be ll")
	}
	if proto.NextProfile(proto.ProfileLatencyUltraLow) != proto.ProfileBandwidthUltraLow {
		t.Fatal("next profile after ull must wrap to ulbw")
	}
	assertOpusRow(t, o.Opus["voice_low"], proto.OpusVoiceLow)
	assertOpusRow(t, o.Opus["voice_medium"], proto.OpusVoiceMedium)
	assertOpusRow(t, o.Opus["voice_high"], proto.OpusVoiceHigh)
	assertOpusRow(t, o.Opus["voice_max"], proto.OpusVoiceMax)
	assertOpusRow(t, o.Opus["audio_min"], proto.OpusAudioMin)
	assertOpusRow(t, o.Opus["audio_low"], proto.OpusAudioLow)
	assertOpusRow(t, o.Opus["audio_medium"], proto.OpusAudioMedium)
	assertOpusRow(t, o.Opus["audio_high"], proto.OpusAudioHigh)
	assertOpusRow(t, o.Opus["audio_max"], proto.OpusAudioMax)
	if o.Phone["ring_time"] != 60 || o.Phone["wait_time"] != 70 || o.Phone["connect_time"] != 5 {
		t.Fatalf("telephone timings %+v", o.Phone)
	}
	if o.Phone["announce_interval"] != 60*60*3 || o.Phone["announce_interval_min"] != 60*5 {
		t.Fatalf("announce %+v", o.Phone)
	}
	if o.Phone["dial_hz"] != 382 {
		t.Fatalf("dial hz %d", o.Phone["dial_hz"])
	}
	for br, hdr := range map[int]byte{700: 0, 1200: 1, 1300: 2, 1400: 3, 1600: 4, 2400: 5, 3200: 6} {
		if o.C2Hdr[strconv.Itoa(br)] != int(hdr) {
			t.Fatalf("codec2 header %d python %d want %d", br, o.C2Hdr[strconv.Itoa(br)], hdr)
		}
		if proto.Codec2HeaderForBitrate(br) != hdr {
			t.Fatalf("go codec2 header %d", br)
		}
	}
	id := make([]byte, 16)
	for i := range id {
		id[i] = byte(i + 1)
	}
	wantHash, err := hex.DecodeString(o.DestHash)
	if err != nil {
		t.Fatal(err)
	}
	got := proto.TelephonyHash(id)
	if !bytes.Equal(got, wantHash) {
		t.Fatalf("dest hash %x want %x", got, wantHash)
	}
	t.Log("PYTHON_LXST_CONSTANTS_PROVED")
}

func TestUmsgpackWireMatchesPython(t *testing.T) {
	o := loadPythonReference(t)
	cases := []struct {
		name string
		pack func() ([]byte, error)
	}{
		{"busy", func() ([]byte, error) { return proto.PackSignalling([]int{proto.StatusBusy}) }},
		{"rejected", func() ([]byte, error) { return proto.PackSignalling([]int{proto.StatusRejected}) }},
		{"calling", func() ([]byte, error) { return proto.PackSignalling([]int{proto.StatusCalling}) }},
		{"available", func() ([]byte, error) { return proto.PackSignalling([]int{proto.StatusAvailable}) }},
		{"ringing", func() ([]byte, error) { return proto.PackSignalling([]int{proto.StatusRinging}) }},
		{"connecting", func() ([]byte, error) { return proto.PackSignalling([]int{proto.StatusConnecting}) }},
		{"established", func() ([]byte, error) { return proto.PackSignalling([]int{proto.StatusEstablished}) }},
		{"pref_mq_fd", func() ([]byte, error) {
			return proto.PackSignalling([]int{
				proto.StatusAvailable,
				proto.SignalPreferredProfile(proto.ProfileQualityMedium),
				proto.SignalPreferredMode(proto.ModeFullDuplex),
			})
		}},
		{"frame_opus", func() ([]byte, error) { return proto.PackFrame(proto.CodecOpus, []byte{9, 8, 7}) }},
		{"frame_codec2", func() ([]byte, error) { return proto.PackFrame(proto.CodecCodec2, []byte{6, 1, 2, 3}) }},
	}
	for _, tc := range cases {
		raw, err := tc.pack()
		if err != nil {
			t.Fatalf("%s pack: %v", tc.name, err)
		}
		want, err := hex.DecodeString(o.Wire[tc.name])
		if err != nil {
			t.Fatalf("%s hex: %v", tc.name, err)
		}
		if !bytes.Equal(raw, want) {
			t.Fatalf("%s go %x python %x", tc.name, raw, want)
		}
		pkt, err := proto.Unpack(want)
		if err != nil {
			t.Fatalf("%s unpack python: %v", tc.name, err)
		}
		if len(pkt.Signals) == 0 && len(pkt.Frames) == 0 {
			t.Fatalf("%s empty after unpack", tc.name)
		}
	}
	t.Log("PYTHON_UMSGPACK_WIRE_PROVED")
}

func assertOpusRow(t *testing.T, row opusRow, profile int) {
	t.Helper()
	p := proto.OpusProfileParams(profile)
	if row.ID != profile || row.Channels != p.Channels || row.Rate != p.SampleRate || row.Bitrate != p.Bitrate || row.Voip != p.Voip {
		t.Fatalf("opus profile %d python %+v go %+v", profile, row, p)
	}
}
