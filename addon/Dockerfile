# Declared before the first FROM so it's available to both build stages.
ARG BUILD_FROM

# Build stage: compile a static linux/{arm64,amd64} binary. CGO_ENABLED=0 per
# CLAUDE.md — this image runs alongside Home Assistant on a Raspberry Pi.
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/ha-inspector-mcp ./cmd/server

# Runtime stage: the Home Assistant base image for the target arch, selected
# by build.yaml's build_from. No shell tooling beyond what the base ships.
FROM ${BUILD_FROM}

COPY --from=build /out/ha-inspector-mcp /usr/bin/ha-inspector-mcp
COPY addon/rootfs/run.sh /run.sh
RUN chmod +x /run.sh

ENTRYPOINT ["/run.sh"]
