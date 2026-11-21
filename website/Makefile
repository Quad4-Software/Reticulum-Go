BINARY := hugo-reticulum
PKG := ./...

.PHONY: build test lint vet fmt docs serve clean

build:
	go build -o $(BINARY) ./cmd/hugo-reticulum

test:
	go test $(PKG)

vet:
	go vet $(PKG)

lint: vet
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed:" && gofmt -l . && exit 1)

fmt:
	gofmt -w .

docs: build
	go run ./cmd/docs-bundle
	cd site && hugo --source . && ../$(BINARY) build

serve: build
	cd site && ../$(BINARY) serve --watch

clean:
	rm -f $(BINARY)
	rm -rf site/public site/resources bin
