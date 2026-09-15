module github.com/Quad4-Software/Reticulum-Go/examples/wasm

go 1.27.1

require (
	github.com/Quad4-Software/Reticulum-Go v1.0.0
	quad4/reticulum-go-protocols v0.0.0
)

require (
	github.com/dunglas/httpsfv v1.1.1 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/landlock-lsm/go-landlock v0.10.0 // indirect
	github.com/mdlayher/socket v0.7.0 // indirect
	github.com/mdlayher/vsock v1.3.0 // indirect
	github.com/quic-go/qpack v0.6.0 // indirect
	github.com/quic-go/quic-go v0.62.0 // indirect
	github.com/quic-go/webtransport-go v0.13.0 // indirect
	go.bug.st/serial v1.8.0 // indirect
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/term v0.46.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	kernel.org/pub/linux/libs/security/libcap/psx v1.2.78 // indirect
	github.com/Quad4-Software/bzip2 v0.0.0 // indirect
	github.com/Quad4-Software/msgpack/v5 v5.8.2 // indirect
	github.com/Quad4-Software/pbt v0.0.0 // indirect
	github.com/Quad4-Software/tagparser v0.0.0 // indirect
)

replace (
	github.com/Quad4-Software/Reticulum-Go => ../../
	github.com/Quad4-Software/bzip2 => ../../../../Reticulum-Go-Projects/bzip2
	github.com/Quad4-Software/msgpack/v5 => ../../../../Reticulum-Go-Projects/msgpack
	github.com/Quad4-Software/pbt => ../../../../Reticulum-Go-Projects/pbt
	github.com/Quad4-Software/tagparser => ../../../../Reticulum-Go-Projects/tagparser
	quad4/reticulum-go-protocols => ../../../../Reticulum-Go-Projects/reticulum-go-mf
	// quad4/* names below satisfy reticulum-go-protocols' own requires, which
	// still use the old module paths upstream.
	quad4/bzip2 => github.com/Quad4-Software/bzip2 v0.0.0-20260704225916-ca8b2bb66059
	quad4/msgpack/v5 => github.com/Quad4-Software/msgpack/v5 v5.8.2
	quad4/pbt => github.com/Quad4-Software/pbt v0.0.0-20260614183135-abe0cfc4e604
	quad4/reticulum-go => github.com/Quad4-Software/Reticulum-Go v1.1.1
	quad4/tagparser => github.com/Quad4-Software/tagparser v0.1.3-0.20260614183136-daa4d5f437ce
)
