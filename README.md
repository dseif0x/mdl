# mdl

A small Go service for **searching and downloading music** from multiple
providers behind one REST API and a simple web UI.

Supported out of the box:

| Provider     | Search                         | Download                              |
|--------------|--------------------------------|---------------------------------------|
| YouTube      | yt-dlp (`ytsearch`)            | yt-dlp → MP3                          |
| SoundCloud   | yt-dlp (`scsearch`)            | yt-dlp → MP3                          |
| Apple Music  | iTunes Search API (no auth)    | [apple-music-downloader] via `exec`   |

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
  server/                REST handlers + access logging
web/                     embedded static frontend
```

## Running

### With Docker Compose (recommended)

This brings up `mdl` together with the `apple-music-downloader` container it
execs into for Apple Music downloads.

```bash
docker compose up --build
```

Then open <http://localhost:8080>. Downloads land in `./downloads`.

> The compose file builds the Apple Music image from
> `./apple-music-downloader/Dockerfile` (your existing Dockerfile, included
> here). Provide the credentials/config that
> [apple-music-downloader] requires for downloads to succeed.

### Locally (Go)

Requires [`yt-dlp`](https://github.com/yt-dlp/yt-dlp) and `ffmpeg` on `PATH`
for YouTube/SoundCloud. Apple Music search works with no extra setup;
Apple Music downloads need `apple-music-dl` reachable (see `MDL_APPLEMUSIC_CMD`).

```bash
go run ./cmd/mdl
# → listening on :8080
```

## Configuration

All configuration is via environment variables:

| Variable               | Default            | Description                                                        |
|------------------------|--------------------|--------------------------------------------------------------------|
| `MDL_ADDR`             | `:8080`            | Listen address.                                                    |
| `MDL_DOWNLOAD_DIR`     | `./downloads`      | Destination directory for downloads.                              |
| `MDL_YTDLP_BINARY`     | `yt-dlp`           | yt-dlp executable (name on `PATH` or absolute path).             |
| `MDL_APPLEMUSIC_CMD`   | `apple-music-dl`   | Command to run apple-music-dl; the track URL is appended.        |
| `MDL_DOWNLOAD_TIMEOUT` | `30m`              | Maximum duration of a single download.                            |

For the containerised setup, `MDL_APPLEMUSIC_CMD` is set to
`docker exec apple-music-downloader apple-music-dl`, so the provider runs the
downloader inside its sibling container.

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
Downloads a track. Supply either a full `track` (as returned by search) or a
`provider` + `url`.

```bash
curl -X POST localhost:8080/api/download \
  -H 'Content-Type: application/json' \
  -d '{"provider":"youtube","url":"https://www.youtube.com/watch?v=..."}'
```

```json
{ "files": ["/downloads/Artist - Title.mp3"], "log": "..." }
```

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

- Downloads are synchronous per request (bounded by `MDL_DOWNLOAD_TIMEOUT`).
  An async job queue is a natural next step.
- Apple Music downloads run inside the apple-music-downloader container, so
  `mdl` returns the downloader's log rather than enumerated file paths; the
  files appear in the shared `/downloads` volume.
- Respect the terms of service and copyright law of each provider. This tool
  is for downloading content you are entitled to access.
