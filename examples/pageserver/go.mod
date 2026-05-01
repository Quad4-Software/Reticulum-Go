module example-pageserver

go 1.26.2

replace git.quad4.io/Networks/Reticulum-Go => ../..

require (
	git.quad4.io/Go-Libs/msgpack/v5 v5.7.0
	git.quad4.io/Networks/Reticulum-Go v0.9.1
)

require (
	git.quad4.io/Go-Libs/bzip2 v1.1.0 // indirect
	git.quad4.io/Go-Libs/tagparser/v2 v2.1.0 // indirect
	golang.org/x/crypto v0.50.0 // indirect
)
