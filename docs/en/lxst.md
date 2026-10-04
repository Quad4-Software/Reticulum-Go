# pkg/lxst

LXST 0.5.4 telephony over Reticulum links. Wire format uses msgpack maps for signalling (`{0: [...]}`) and audio frames (`{1: [codec, payload]}`). Destination is lxst.telephony. Opus is the default codec, with Codec2 for low bandwidth.

```go
caller := call.NewCall(tr, call.Config{
	Identity: id,
	UseAudio: false,
})
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
if err := caller.Dial(ctx, remoteIdentity); err != nil {
	log.Fatal(err)
}
```

| Package | Role |
| --- | --- |
| [pkg/lxst/proto](../../pkg/lxst/proto) | Wire codec |
| [pkg/lxst/call](../../pkg/lxst/call) | State machine and signalling |
| [pkg/lxst/media](../../pkg/lxst/media) | Jitter buffer and adaptive bitrate |
| [pkg/lxst/audio](../../pkg/lxst/audio) | Opus, Codec2, device I/O |
| [pkg/lxst/rnsnode](../../pkg/lxst/rnsnode) | Shared Reticulum instance, UDP hop, or config directory |

| Binary | Purpose |
| --- | --- |
| [cmd/rnphone](../../cmd/rnphone) | Interactive phone (phonebook, ringtone, announce, history) |
| [cmd/rgesp-dial](../../cmd/rgesp-dial) | Scriptable dialer for tests and automation |

Build with the lxst_native tag for real audio (requires CGO plus libopus and libcodec2, either system packages or third_party builds from scripts/lxst/vendor-sync.sh):

```bash
task rnphone
./bin/rnphone-$(go env GOOS)-$(go env GOARCH)
```

Without lxst_native, pkg/lxst compiles with codec stubs so unit tests and the wire codec run without native libraries. Python lxst interop and the phone binaries always build with the tag.

```bash
make liblxst
task build-liblxst
task test-codec
```

Python round-trips need `pip install lxst==0.5.4`.

```bash
task test:lxst
task test:lxst:interop
go test -v -count=1 ./pkg/lxst/...
task example:lxst
task example:lxst:call
```

Live mesh call tests (`TestGoGo*`, `TestE2E_*`, `TestAcceptance_*`) are skipped under `-short`.
