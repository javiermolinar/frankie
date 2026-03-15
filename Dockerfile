# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

WORKDIR /src

RUN apk add --no-cache ca-certificates tzdata
RUN mkdir -p /config && touch /config/.keep

COPY go.mod ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY src ./src
COPY assets ./assets

ARG TARGETOS=linux
ARG TARGETARCH=amd64

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w -buildid=" -o /out/frankie ./src

FROM scratch

WORKDIR /app

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /out/frankie /app/frankie
COPY --from=builder --chown=65532:65532 /config /config
COPY --from=builder /src/assets /app/assets

ENV PORT=3593
ENV CONFIG_FILE=/config/config.json

EXPOSE 3593
VOLUME ["/config"]

USER 65532:65532

ENTRYPOINT ["/app/frankie"]
