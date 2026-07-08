module quad4/reticulum-go

go 1.26.4

require (
	golang.org/x/crypto v0.52.0
	golang.org/x/sys v0.45.0
	quad4/bzip2 v0.0.0
	quad4/msgpack/v5 v5.8.0
	quad4/pbt v0.0.0
)

require quad4/tagparser v0.0.0 // indirect

replace (
	quad4/bzip2 => ../../Reticulum-Go-Projects/bzip2
	quad4/msgpack/v5 => ../../Reticulum-Go-Projects/msgpack
	quad4/pbt => ../../Reticulum-Go-Projects/pbt
	quad4/reticulum-go-mf => ../../Reticulum-Go-Projects/reticulum-go-mf
	quad4/tagparser => ../../Reticulum-Go-Projects/tagparser
)
