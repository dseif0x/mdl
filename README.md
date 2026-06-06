# mdl

A small Go service for **searching and downloading music** from multiple
providers behind one REST API and a simple web UI.

Supported out of the box:

| Provider     | Search                         | Download                              |
|--------------|--------------------------------|---------------------------------------|
| YouTube      | yt-dlp (`ytsearch`)            | yt-dlp → MP3                          |
| SoundCloud   | yt-dlp (`scsearch`)            | yt-dlp → MP3                          |
| Apple Music  | iTunes Search API (no auth)    | bundled [apple-music-downloader]      |

Adding a new provider (Deezer, Tidal, …) is intentionally cheap — see
[Adding a provider](#adding-a-provider).

[apple-music-downloader]: https://github.com/zhaarey/apple-music-downloader

## Architecture

```
                       ┌──────────────┐
   browser  ──────────▶│  web UI      │  (static HTML/JS, embedded in binary)
                       └──────┬───────┘
                              │ REST
                       ┌──────▼───────┐
                       │  HTTP server │  internal/server
                       └──────┬───────┘
                              │ provider.Provider interface
        ┌─────────────────────┼─────────────────────┐
        ▼                     ▼                       ▼
   youtube/              soundcloud/             applemusic/
        │                     │                       │
        └──────── yt-dlp ─────┘            iTunes API + apple-music-dl
```

Every service implements the `provider.Provider` interface
(`internal/provider/provider.go`) and is registered once in
`cmd/mdl/main.go`. The HTTP layer and the frontend are provider-agnostic: they
discover what is available through `GET /api/providers`.

```
cmd/mdl/                 main: config, provider registration, HTTP server
internal/
  config/                env-var configuration
  provider/              Provider interface + concurrency-safe registry
    ytdlp/               shared yt-dlp wrapper (search + download)
    youtube/             YouTube provider (yt-dlp)
    soundcloud/          SoundCloud provider (yt-dlp)
    applemusic/          Apple Music provider (iTunes API + apple-music-dl)
  queue/                 background download queue (worker pool + job tracking)
  server/                REST handlers + access logging
web/                     embedded static frontend
```

## Running

### With Docker (recommended)

The image bundles everything: the `mdl` binary, `yt-dlp` + `ffmpeg`
(YouTube/SoundCloud) and `apple-music-dl` (Apple Music, built in its own stage
from [apple-music-downloader] on a `gpac/ubuntu` base for MP4Box).

```bash
docker compose up --build
```

Then open <http://localhost:8080>. Downloads land in `./downloads`.

> Provide the credentials/config that [apple-music-downloader] requires for
> Apple Music *downloads* to succeed (search needs nothing). A published image
> is available at `ghcr.io/dseif0x/mdl` — see [Continuous delivery](#continuous-delivery).

### Locally (Go)

Requires [`yt-dlp`](https://github.com/yt-dlp/yt-dlp) and `ffmpeg` on `PATH`
for YouTube/SoundCloud. Apple Music search works with no extra setup;
Apple Music downloads need `apple-music-dl` reachable (see `MDL_APPLEMUSIC_CMD`).

```bash
go run ./cmd/mdl
# → listening on :8080
```

### Kubernetes (Helm)

A Helm chart is staged in [`deploy/helm/mdl`](deploy/helm/mdl) (intended to be
published from the separate `helm-charts` repo). It deploys the `mdl` image
alongside the [wrapper] decryption service that the bundled apple-music-dl
talks to, and exposes a configurable Ingress for the web UI/API
(`mdl.ingress.enabled`). See the chart's `values.yaml` for options.

[wrapper]: https://github.com/WorldObservationLog/wrapper

## Configuration

All configuration is via environment variables:

| Variable               | Default            | Description                                                        |
|------------------------|--------------------|--------------------------------------------------------------------|
| `MDL_ADDR`             | `:8080`            | Listen address.                                                    |
| `MDL_DOWNLOAD_DIR`     | `./downloads`      | Destination directory for downloads.                              |
| `MDL_YTDLP_BINARY`     | `yt-dlp`           | yt-dlp executable (name on `PATH` or absolute path).             |
| `MDL_APPLEMUSIC_CMD`   | `apple-music-dl`   | Command to run apple-music-dl; the track URL is appended.        |
| `MDL_DOWNLOAD_TIMEOUT` | `30m`              | Maximum duration of a single download.                            |
| `MDL_DOWNLOAD_WORKERS` | `2`                | Number of concurrent downloads (worker-pool size).               |

`MDL_APPLEMUSIC_CMD` is split on spaces, so it can also point elsewhere — e.g.
`docker exec some-container apple-music-dl` to run the downloader in a separate
container instead of the bundled binary.

## REST API

### `GET /api/providers`
Lists registered providers and their capabilities.

```json
{ "providers": [
  { "name": "youtube", "display_name": "YouTube",
    "capabilities": { "search": true, "download": true } }
] }
```

### `GET /api/search?provider=<name>&q=<query>&limit=<n>`
Searches a provider. `limit` defaults to 10 (max 50).

```json
{ "tracks": [
  { "id": "...", "provider": "youtube", "title": "...", "artist": "...",
    "album": "...", "duration": 213, "url": "https://...",
    "artwork_url": "https://..." }
] }
```

### `POST /api/download`
Enqueues a download and returns immediately with the created **job** (HTTP
`202 Accepted`). Downloads run on a background worker pool; poll the job for
progress. Supply either a full `track` (as returned by search) or a
`provider` + `url`.

```bash
curl -X POST localhost:8080/api/download \
  -H 'Content-Type: application/json' \
  -d '{"provider":"youtube","url":"https://www.youtube.com/watch?v=..."}'
```

```json
{ "id": "c56f92d998f22ae9", "provider": "youtube", "status": "queued",
  "track": { "url": "https://..." }, "created_at": "..." }
```

### `GET /api/jobs`
Lists download jobs, newest first. A job's `status` is one of `queued`,
`running`, `completed`, `failed`, `cancelled`.

```json
{ "jobs": [
  { "id": "c56f9…", "provider": "youtube", "status": "completed",
    "result": { "files": ["/downloads/Artist - Title.mp3"] },
    "created_at": "...", "started_at": "...", "finished_at": "..." }
] }
```

### `GET /api/jobs/{id}`
Returns a single job (404 if unknown). Poll this until `status` is terminal.

### `POST /api/jobs/{id}/cancel`
Cancels a queued or running job. Returns `409` if the job is unknown or already
finished.

## Adding a provider

1. Create `internal/provider/<name>/<name>.go` implementing
   `provider.Provider` (`Name`, `DisplayName`, `Capabilities`, `Search`,
   `Download`). If the service is supported by yt-dlp, reuse
   `internal/provider/ytdlp` and just supply the search prefix — see the
   YouTube/SoundCloud providers (each ~40 lines).
2. Register it in `cmd/mdl/main.go`:
   ```go
   registry.Register(deezer.New(...))
   ```

That's it — the API and UI pick it up automatically.

## Development

```bash
go test ./...      # run tests
go vet ./...       # static checks
gofmt -l .         # formatting (should print nothing)
```

## Notes & limitations

- Downloads run asynchronously on an in-memory worker pool
  (`MDL_DOWNLOAD_WORKERS`), each bounded by `MDL_DOWNLOAD_TIMEOUT`. Because the
  queue is in-memory, jobs do not survive a restart; a persistent store would
  be the next step if durability is needed.
- For Apple Music, `mdl` returns apple-music-dl's log rather than enumerated
  file paths; the files appear under `/downloads` (ALAC/Atmos/AAC subfolders).
- Respect the terms of service and copyright law of each provider. This tool
  is for downloading content you are entitled to access.

## Continuous delivery

`.github/workflows/docker-publish.yml` builds the image and pushes it to the
GitHub Container Registry on every push to `main` (e.g. a merged PR), tagging
it `latest` and with the commit SHA:

```bash
docker pull ghcr.io/dseif0x/mdl:latest
```

The workflow uses the built-in `GITHUB_TOKEN` (no secrets to configure) and
builds `linux/amd64`. It builds the same multi-stage `Dockerfile`, including the
`apple-music-dl` stage.
