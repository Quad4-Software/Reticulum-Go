module quad4/reticulum-go/examples/wasm

go 1.26.5

require (
	quad4/reticulum-go v1.0.0
	quad4/reticulum-go-protocols v0.0.0
)

require (
	github.com/dunglas/httpsfv v1.1.0 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/landlock-lsm/go-landlock v0.9.0 // indirect
	github.com/mdlayher/socket v0.6.1 // indirect
	github.com/mdlayher/vsock v1.3.0 // indirect
	github.com/quic-go/qpack v0.6.0 // indirect
	github.com/quic-go/quic-go v0.60.0 // indirect
	github.com/quic-go/webtransport-go v0.11.1 // indirect
	go.bug.st/serial v1.8.0 // indirect
	golang.org/x/crypto v0.54.0 // indirect
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	kernel.org/pub/linux/libs/security/libcap/psx v1.2.77 // indirect
	quad4/bzip2 v0.0.0 // indirect
	quad4/msgpack/v5 v5.8.2 // indirect
	quad4/tagparser v0.0.0 // indirect
)

replace (
	quad4/bzip2 => ../../../../Reticulum-Go-Projects/bzip2
	quad4/msgpack/v5 => ../../../../Reticulum-Go-Projects/msgpack
	quad4/pbt => ../../../../Reticulum-Go-Projects/pbt
	quad4/reticulum-go => ../../
	quad4/reticulum-go-protocols => ../../../../Reticulum-Go-Projects/reticulum-go-mf
	quad4/tagparser => ../../../../Reticulum-Go-Projects/tagparser
)
