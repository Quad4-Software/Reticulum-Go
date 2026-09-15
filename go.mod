module github.com/Quad4-Software/Reticulum-Go

go 1.27.1

require (
	github.com/Quad4-Software/bzip2 v0.0.0
	github.com/Quad4-Software/msgpack/v5 v5.8.1
	github.com/Quad4-Software/pbt v0.0.0
	github.com/creack/pty v1.1.24
	github.com/ebitengine/purego v0.11.0
	github.com/godbus/dbus/v5 v5.2.2
	github.com/landlock-lsm/go-landlock v0.10.0
	github.com/mdlayher/vsock v1.3.0
	github.com/miekg/dns v1.1.73
	github.com/quic-go/quic-go v0.62.0
	github.com/quic-go/webtransport-go v0.13.0
	go.bug.st/serial v1.8.0
	golang.org/x/crypto v0.57.0
	golang.org/x/sys v0.48.0
	golang.org/x/term v0.46.0
)

require (
	github.com/Quad4-Software/tagparser v0.0.0 // indirect
	github.com/dunglas/httpsfv v1.1.1 // indirect
	github.com/mdlayher/socket v0.7.0 // indirect
	github.com/quic-go/qpack v0.6.0 // indirect
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	kernel.org/pub/linux/libs/security/libcap/psx v1.2.78 // indirect
)

replace (
	github.com/Quad4-Software/bzip2 => ./vendor/github.com/Quad4-Software/bzip2
	github.com/Quad4-Software/msgpack/v5 => ./vendor/github.com/Quad4-Software/msgpack/v5
	github.com/Quad4-Software/pbt => ./vendor/github.com/Quad4-Software/pbt
	github.com/Quad4-Software/tagparser => ./vendor/github.com/Quad4-Software/tagparser
)
