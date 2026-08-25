# Builds the published App image (App-distribution decision,
# docs/development/phases/00-spike-foundations.md). Built by
# .github/workflows/release.yml from the repository root as build context —
# never by Supervisor, which only ever pulls the image config.yaml's `image:`
# names (docs/research/2026-08-24-supervisor-addon-build-context.md).

# Build stage pinned to the host's native platform, not the target one: Go
# cross-compiles natively via GOARCH, so running the toolchain itself under
# QEMU emulation (the default when a multi-platform build's stages all follow
# --platform) is both slower and, observed in practice, prone to segfaults in
# `asm`/`compile` under emulation. Only the tiny final stage needs to actually
# be the target arch, and it runs nothing — it just copies a static binary.
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/ha-inspector-mcp ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /out/ha-inspector-mcp /usr/bin/ha-inspector-mcp
COPY addon/rootfs/run.sh /run.sh
RUN chmod +x /usr/bin/ha-inspector-mcp /run.sh

ENTRYPOINT ["/run.sh"]
