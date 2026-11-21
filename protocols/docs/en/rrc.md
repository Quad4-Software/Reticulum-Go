# pkg/rrc

CBOR envelopes on a Reticulum Link to destination rrc.hub. Spec is [0.1.3](https://rrc.kc1awv.net/). The Go hub and client cover HELLO/WELCOME, JOIN/PART, MSG/NOTICE/ACTION, PING/PONG, ERROR, plus rrcd extensions: envelope dest key 8, resource envelope type 50, and WELCOME capability flags.

```go
dest, err := rrc.NewHubDestination(id, tr)
if err != nil {
	log.Fatal(err)
}
hub, err := rrc.NewHub(tr, dest, rrc.HubConfig{Name: "go-hub", Version: "0.1.0"})
if err != nil {
	log.Fatal(err)
}
hub.Start()
_ = dest.Announce(false, nil, nil)
```

```go
client, err := rrc.Dial(tr, id, hubHash, rrc.ClientConfig{
	Nick: "alice",
	Name: "my-client",
	Handlers: rrc.ClientHandlers{
		OnMsg: func(env *rrc.Envelope) {
			text, _ := rrc.BodyAsString(env.Body)
			log.Printf("[%s] %s", env.Room, text)
		},
	},
})
if err != nil {
	log.Fatal(err)
}
defer client.Close()
if err := client.Join("#lobby"); err != nil {
	log.Fatal(err)
}
```

Wait for `OnJoined` before `SendMsg`.

```bash
task test:rrc
make test
go test -v -count=1 ./pkg/rrc/...
```

Python codec checks use [rrcd](https://github.com/kc1awv/rrcd) via uv. Clone it to RRC-ref, then:

```bash
task test:rrc:interop
```

Live UDP cases (`TestInterop_Live_PythonClientGoHub`, `TestInterop_Live_GoClientPythonHub`, `TestInterop_Live_PythonClientGoDaemon`) are skipped under `-short` and are required when `CI_REQUIRE_INTEROP=1`.

The interop package also includes a Python RRC client at [pkg/rrc/interoptest/client.py](../../pkg/rrc/interoptest/client.py) used to drive gorrcd: HELLO/WELCOME, JOIN/MSG, `/list` `/who`, unknown slash commands, PING/PONG, and ACTION.

```bash
task test:live
```

`pkg/rrc.Hub` is the wire protocol core. Optional `HubPolicy` is used by [cmd/gorrcd](../../cmd/gorrcd) for slash commands, IRC-style room modes, trusted/banned lists, MOTD, hub ping, and RNS Resource blob transfer. See [gorrcd.md](gorrcd.md).
