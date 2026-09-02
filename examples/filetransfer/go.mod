module example-filetransfer

go 1.26.6

require (
	quad4/msgpack/v5 v5.8.1
	quad4/reticulum-go v0.0.0
)

require (
	github.com/quic-go/quic-go v0.60.0 // indirect
	golang.org/x/crypto v0.55.0 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
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
