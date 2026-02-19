# syntax=docker/dockerfile:1.7

FROM golang:1.24-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
COPY cmd/server ./cmd/server
COPY internal ./internal

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/vpn-server ./cmd/server

FROM debian:stable-slim

ARG SING_BOX_VERSION=1.12.21

RUN apt-get update && apt-get install -y --no-install-recommends \
    bash \
    ca-certificates \
    curl \
    iproute2 \
    iptables \
    procps \
    tar \
    && curl -fsSL "https://github.com/SagerNet/sing-box/releases/download/v${SING_BOX_VERSION}/sing-box-${SING_BOX_VERSION}-linux-amd64.tar.gz" -o /tmp/sing-box.tar.gz \
    && tar -xzf /tmp/sing-box.tar.gz -C /tmp \
    && install -m 0755 "/tmp/sing-box-${SING_BOX_VERSION}-linux-amd64/sing-box" /usr/local/bin/sing-box \
    && rm -rf /tmp/sing-box.tar.gz "/tmp/sing-box-${SING_BOX_VERSION}-linux-amd64" /var/lib/apt/lists/*

WORKDIR /app

COPY --from=build /out/vpn-server /app/vpn-server
COPY docker/entrypoint.sh /entrypoint.sh

RUN chmod +x /app/vpn-server /entrypoint.sh \
    && mkdir -p /etc/vpn/clients /etc/vpn/tls /var/log \
    && chmod 700 /etc/vpn /etc/vpn/clients /etc/vpn/tls

EXPOSE 443
EXPOSE 8080

ENTRYPOINT ["/entrypoint.sh"]
