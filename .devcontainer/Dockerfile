# Dev Container image for Reticulum-Go (Go, Task, revive, staticcheck, shellcheck, yamllint).
FROM golang:1.27.3-bookworm@sha256:972b4110ba0f47539a975c31e52c64765e1ff19f2d74469ae4a595c6676e80a5

ARG TASK_VERSION=3.46.3
ARG REVIVE_VERSION=v1.15.0
ARG STATICCHECK_VERSION=2026.2.1

RUN apt-get update -qq \
    && apt-get install -y --no-install-recommends \
        curl \
        ca-certificates \
        shellcheck \
        yamllint \
        python3 \
        python3-yaml \
        git \
    && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL "https://github.com/go-task/task/releases/download/v${TASK_VERSION}/task_linux_amd64.tar.gz" \
        -o /tmp/task.tar.gz \
    && tar -xzf /tmp/task.tar.gz -C /usr/local/bin task \
    && rm -f /tmp/task.tar.gz

ENV GOFLAGS=-mod=vendor
ENV GOPROXY=off
ENV GOSUMDB=off

RUN go install "github.com/mgechev/revive@${REVIVE_VERSION}" \
    && go install "honnef.co/go/tools/cmd/staticcheck@${STATICCHECK_VERSION}"

WORKDIR /workspaces/Reticulum-Go
