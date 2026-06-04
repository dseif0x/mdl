# syntax=docker/dockerfile:1
ARG GOVERSION=1.24

# --- build the mdl binary --------------------------------------------------
FROM --platform=$BUILDPLATFORM golang:${GOVERSION}-alpine AS builder
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
# Cache modules first (this project is stdlib-only, but keeps the layer stable).
COPY go.mod ./
RUN go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod/ \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w" -o /bin/mdl ./cmd/mdl

# --- runtime ---------------------------------------------------------------
FROM debian:bookworm-slim
ENV DEBIAN_FRONTEND=noninteractive

# ffmpeg + python3 power yt-dlp (YouTube/SoundCloud); ca-certificates for HTTPS.
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        ffmpeg python3 ca-certificates curl && \
    curl -fsSL https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp \
        -o /usr/local/bin/yt-dlp && \
    chmod +x /usr/local/bin/yt-dlp && \
    apt-get purge -y curl && apt-get autoremove -y && \
    rm -rf /var/lib/apt/lists/*

# Docker CLI so the Apple Music provider can `docker exec` into the
# apple-music-downloader container (see docker-compose.yml).
COPY --from=docker:cli /usr/local/bin/docker /usr/local/bin/docker

COPY --from=builder /bin/mdl /usr/local/bin/mdl

ENV MDL_ADDR=:8080 \
    MDL_DOWNLOAD_DIR=/downloads
EXPOSE 8080
VOLUME ["/downloads"]
ENTRYPOINT ["/usr/local/bin/mdl"]
