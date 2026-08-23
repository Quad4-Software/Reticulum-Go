# Dev Container image for Reticulum-Go (Go, Task, revive, staticcheck, shellcheck, yamllint).
FROM golang:1.26.6-bookworm

ARG TASK_VERSION=3.46.3
ARG REVIVE_VERSION=v1.15.0
ARG STATICCHECK_VERSION=v0.6.1

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
