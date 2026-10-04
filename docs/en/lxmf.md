# pkg/lxmf

Matches LXMF 1.2.0 on the wire: pack and Ed25519 sign, opportunistic and direct delivery, proof-of-work stamps, lxm:// paper URIs, packed containers, and announce app-data. `NewDeliveryMessenger` builds an inbound lxmf.delivery destination so the hash matches what Python LXMF derives for the same identity.

```go
messenger, err := lxmf.NewDeliveryMessenger(id, tr)
if err != nil {
	log.Fatal(err)
}
messenger.SetMessageHandler(func(m *lxmf.LXMessage, _ common.NetworkInterface) {
	log.Printf("%s: %q", m.FormatHash(), m.ContentString())
})
_ = messenger.Destination().Announce(false, nil, nil)
```

Other entry points:

- `lxmf.NewMessage` and `LXMessage.Pack` to assemble and sign
- `lxmf.Unpack` / `UnpackFromBytes` for bytes that did not arrive through Messenger
- `DisplayNameFromAppData`, `StampCostFromAppData`, and `EncodeAnnounceAppData*` for announce metadata

Python round-trips (`TestInterop`) need uv and an LXMF-ref clone of upstream LXMF.

```bash
task test:lxmf
task test:lxmf:interop
go test -v -count=1 ./pkg/lxmf/...
go test -v -count=1 -timeout 15m -run TestInterop ./pkg/lxmf/...
```

Live UDP cases (`TestInterop_Live_*`) are skipped under `-short` and are required when `CI=1`. See [examples/lxmf_send.go](../../examples/lxmf_send.go) for a two-node UDP sender.
