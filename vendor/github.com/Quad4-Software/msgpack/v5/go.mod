module github.com/Quad4-Software/msgpack/v5

go 1.27.1

require (
	github.com/Quad4-Software/pbt v0.0.0
	github.com/Quad4-Software/tagparser v0.0.0
)

replace (
	github.com/Quad4-Software/pbt => ../pbt
	github.com/Quad4-Software/tagparser => ../tagparser
)
