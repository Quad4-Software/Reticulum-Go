module quad4/reticulum-go

go 1.26.4

require (
	github.com/creack/pty v1.1.24
	github.com/mdlayher/vsock v1.2.1
	github.com/miekg/dns v1.1.68
	github.com/quic-go/quic-go v0.60.0
	github.com/quic-go/webtransport-go v0.11.1
	go.bug.st/serial v1.6.2
	golang.org/x/crypto v0.52.0
	golang.org/x/sys v0.45.0
	quad4/bzip2 v0.0.0
	quad4/msgpack/v5 v5.8.1
	quad4/pbt v0.0.0
)

require (
	github.com/creack/goselect v0.1.2 // indirect
	github.com/dunglas/httpsfv v1.1.0 // indirect
	github.com/mdlayher/socket v0.4.1 // indirect
	github.com/quic-go/qpack v0.6.0 // indirect
	golang.org/x/mod v0.35.0 // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	golang.org/x/tools v0.44.0 // indirect
	quad4/tagparser v0.0.0 // indirect
)

replace (
	quad4/bzip2 => ../../Reticulum-Go-Projects/bzip2
	quad4/msgpack/v5 => ../../Reticulum-Go-Projects/msgpack
	quad4/pbt => ../../Reticulum-Go-Projects/pbt
	quad4/reticulum-go-mf => ../../Reticulum-Go-Projects/reticulum-go-mf
	quad4/tagparser => ../../Reticulum-Go-Projects/tagparser
)
