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

# --- build apple-music-dl (zhaarey/apple-music-downloader) -----------------
# Built in its own stage, mirroring the upstream Dockerfile, then copied into
# the runtime image so the Apple Music provider can run it as a local process.
FROM --platform=$BUILDPLATFORM golang:${GOVERSION}-alpine AS amd-builder
ARG TARGETOS
ARG TARGETARCH
ARG AMD_REPO_URL=https://github.com/zhaarey/apple-music-downloader.git
ARG AMD_REPO_REF=main
RUN apk add --no-cache git
WORKDIR /amd
RUN git clone --depth 1 --branch ${AMD_REPO_REF} ${AMD_REPO_URL} .
RUN --mount=type=cache,target=/go/pkg/mod/ \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -o /bin/apple-music-dl main.go

# --- runtime ---------------------------------------------------------------
# gpac/ubuntu provides MP4Box, required by apple-music-dl (same base the
# upstream image uses). We add ffmpeg + yt-dlp for YouTube/SoundCloud.
FROM gpac/ubuntu
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        ffmpeg python3 ca-certificates curl && \
    curl -fsSL https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp \
        -o /usr/local/bin/yt-dlp && \
    chmod +x /usr/local/bin/yt-dlp && \
    apt-get purge -y curl && apt-get autoremove -y && \
    rm -rf /var/lib/apt/lists/*

# apple-music-dl binary + its config. apple-music-dl already lays out
# Artist/Album/Track via its *-folder-format options; point every save folder at
# the download root so the result matches Jellyfin's expected structure
# (<root>/Artist/Album/Track) alongside the yt-dlp downloads.
COPY --from=amd-builder /bin/apple-music-dl /usr/local/bin/apple-music-dl
COPY --from=amd-builder /amd/config.yaml.example /app/config.yaml
RUN echo 'alac-save-folder: "/downloads"' >> /app/config.yaml \
    && echo 'atmos-save-folder: "/downloads"' >> /app/config.yaml \
    && echo 'aac-save-folder: "/downloads"' >> /app/config.yaml \
    && sed -i 's/^exit-on-error:.*/exit-on-error: true/' /app/config.yaml

COPY --from=builder /bin/mdl /usr/local/bin/mdl

# apple-music-dl reads ./config.yaml from its working directory.
WORKDIR /app
ENV MDL_ADDR=:8080 \
    MDL_DOWNLOAD_DIR=/downloads \
    MDL_APPLEMUSIC_CMD=apple-music-dl
EXPOSE 8080
VOLUME ["/downloads"]
ENTRYPOINT ["/usr/local/bin/mdl"]
