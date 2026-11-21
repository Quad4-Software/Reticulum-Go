# pkg/mf

Wire layout is a 16-byte sender hash followed by UTF-8 text. An optional group hash tags fan-out on a 1:1 destination. `mf.Messenger` sends and receives that format over an existing transport.

```go
msg := &mf.Message{SenderHash: senderHash, Text: "hello"}
packed, err := msg.Pack()
if err != nil {
	log.Fatal(err)
}
got, err := mf.Unpack(packed)
if err != nil {
	log.Fatal(err)
}
_ = got.Text
```

```go
messenger := mf.NewMessenger(tr, dest)
if err := messenger.SendMessage(remoteHash, "hello"); err != nil {
	log.Fatal(err)
}
```

```bash
task example
task example:messenger
go run ./examples/example.go
go run ./examples/messenger.go
go test -v -run TestMessenger_TwoWayCommunication ./pkg/mf/...
```
