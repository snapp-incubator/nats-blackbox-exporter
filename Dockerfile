# syntax=docker/dockerfile:1.10

FROM --platform=$BUILDPLATFORM golang:1.26-alpine3.23 AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

COPY . .

ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" \
    -o /out/nats-blackbox-exporter ./cmd/nats-blackbox-exporter

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /out/nats-blackbox-exporter /usr/local/bin/nats-blackbox-exporter

USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/nats-blackbox-exporter"]
CMD ["--configPath=/app/setting/config.yaml"]
