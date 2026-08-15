module quad4/reticulum-go

go 1.26.5

toolchain go1.26.6

require (
	github.com/creack/pty v1.1.24
	github.com/godbus/dbus/v5 v5.1.0
	github.com/landlock-lsm/go-landlock v0.9.0
	github.com/mdlayher/vsock v1.2.1
	github.com/miekg/dns v1.1.68
	github.com/quic-go/quic-go v0.60.0
	github.com/quic-go/webtransport-go v0.11.1
	go.bug.st/serial v1.6.2
	golang.org/x/crypto v0.53.0
	golang.org/x/sys v0.46.0
	golang.org/x/term v0.44.0
	golang.org/x/tools v0.47.0
	quad4/bzip2 v0.0.0
	quad4/msgpack/v5 v5.8.1
	quad4/pbt v0.0.0
)

require (
	github.com/creack/goselect v0.1.2 // indirect
	github.com/dunglas/httpsfv v1.1.0 // indirect
	github.com/mdlayher/socket v0.4.1 // indirect
	github.com/quic-go/qpack v0.6.0 // indirect
	golang.org/x/mod v0.37.0 // indirect
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/sync v0.21.0 // indirect
	golang.org/x/text v0.39.0 // indirect
	kernel.org/pub/linux/libs/security/libcap/psx v1.2.77 // indirect
	quad4/tagparser v0.0.0 // indirect
)

replace (
	quad4/bzip2 => ./vendor/quad4/bzip2
	quad4/msgpack/v5 => ./vendor/quad4/msgpack/v5
	quad4/pbt => ./vendor/quad4/pbt
	quad4/reticulum-go-protocols => ./vendor/quad4/reticulum-go-protocols
	quad4/tagparser => ./vendor/quad4/tagparser
)
