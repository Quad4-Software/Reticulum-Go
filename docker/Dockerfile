# golang:1.27.1-alpine pinned by digest; update tag comment and digest together
FROM golang:1.27.1-alpine@sha256:4cb7ac979db5fcc41cae44b2227ba5ab8a51e8807f40d9ba4dee20a0ad960b5b AS builder

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ENV CGO_ENABLED=0
ENV GOOS=${TARGETOS}
ENV GOARCH=${TARGETARCH}
ENV GOAMD64=v1
ENV GOFLAGS=-mod=vendor
ENV GOPROXY=off


WORKDIR /build

COPY go.mod go.sum ./
COPY vendor/ vendor/

COPY cmd/ cmd/
COPY internal/ internal/
COPY pkg/ pkg/

RUN go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo \
    -o reticulum-go \
    ./cmd/reticulum-go

FROM busybox:1.37.0@sha256:870e815c3a50dd0f6b40efddb319c72c32c3ee340b5a3e8945904232ccd12f44

RUN adduser -D -s /bin/sh app

COPY --from=builder /build/reticulum-go /usr/local/bin/reticulum-go

RUN chmod +x /usr/local/bin/reticulum-go
RUN mkdir -p /app && chown app:app /app

USER app
WORKDIR /app

EXPOSE 4242

ENTRYPOINT ["/usr/local/bin/reticulum-go"]
