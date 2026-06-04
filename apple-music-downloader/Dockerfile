ARG GOVERSION=1.25.5

FROM --platform=$BUILDPLATFORM golang:${GOVERSION}-alpine AS builder
ARG TARGETOS
ARG TARGETARCH
ARG REPO_URL=https://github.com/zhaarey/apple-music-downloader.git
ARG REPO_REF=main
RUN apk add --no-cache git
WORKDIR /app
RUN git clone --depth 1 --branch ${REPO_REF} ${REPO_URL} .
RUN --mount=type=cache,target=/go/pkg/mod/ \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /bin/apple-music-dl main.go

FROM gpac/ubuntu
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && \
    apt-get install -y --no-install-recommends ffmpeg && \
    rm -rf /var/lib/apt/lists/*
COPY --from=builder /bin/apple-music-dl /usr/local/bin/apple-music-dl
WORKDIR /app
COPY --from=builder /app/config.yaml.example config.yaml
RUN echo 'alac-save-folder: "/downloads/ALAC"' >> config.yaml \
    && echo 'atmos-save-folder: "/downloads/Atmos"' >> config.yaml \
    && echo 'aac-save-folder: "/downloads/AAC"' >> config.yaml
# Keep the container alive so it can be exec'd into to run apple-music-dl manually.
ENTRYPOINT ["sleep", "infinity"]
