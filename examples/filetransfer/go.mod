module example-filetransfer

go 1.27.1

require (
	quad4/msgpack/v5 v5.8.1
	quad4/reticulum-go v0.0.0
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
	golang.org/x/crypto v0.56.0 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/term v0.45.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	kernel.org/pub/linux/libs/security/libcap/psx v1.2.78 // indirect
	quad4/bzip2 v0.0.0 // indirect
	quad4/tagparser v0.0.0 // indirect
)

replace (
	quad4/bzip2 => ../../../../Reticulum-Go-Projects/bzip2
	quad4/msgpack/v5 => ../../../../Reticulum-Go-Projects/msgpack
	quad4/pbt => ../../../../Reticulum-Go-Projects/pbt
	quad4/reticulum-go => ../..
	quad4/tagparser => ../../../../Reticulum-Go-Projects/tagparser
)
