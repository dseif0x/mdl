// Package config loads service configuration from environment variables.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime settings.
type Config struct {
	// Addr is the listen address, e.g. ":8080".
	Addr string
	// DownloadDir is the default destination for downloads.
	DownloadDir string
	// YtdlpBinary is the yt-dlp executable (name on PATH or absolute path).
	YtdlpBinary string
	// AppleMusicCmd is the apple-music-dl invocation, split into argv.
	AppleMusicCmd []string
	// DownloadTimeout caps how long a single download may run.
	DownloadTimeout time.Duration
	// DownloadWorkers is the number of concurrent downloads.
	DownloadWorkers int
}

// Load reads configuration from the environment, applying defaults.
func Load() Config {
	return Config{
		Addr:            env("MDL_ADDR", ":8080"),
		DownloadDir:     env("MDL_DOWNLOAD_DIR", "./downloads"),
		YtdlpBinary:     env("MDL_YTDLP_BINARY", "yt-dlp"),
		AppleMusicCmd:   strings.Fields(env("MDL_APPLEMUSIC_CMD", "apple-music-dl")),
		DownloadTimeout: envDuration("MDL_DOWNLOAD_TIMEOUT", 30*time.Minute),
		DownloadWorkers: envInt("MDL_DOWNLOAD_WORKERS", 2),
	}
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
