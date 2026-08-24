# Builds the published App image (App-distribution decision,
# docs/development/phases/00-spike-foundations.md). Built by
# .github/workflows/release.yml from the repository root as build context —
# never by Supervisor, which only ever pulls the image config.yaml's `image:`
# names (docs/research/2026-08-24-supervisor-addon-build-context.md).

FROM golang:1.25-alpine AS build
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
RUN chmod +x /run.sh

ENTRYPOINT ["/run.sh"]
