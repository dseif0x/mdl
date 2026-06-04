// Command mdl is a music search-and-download service supporting multiple
// providers behind a single REST API and web UI.
package main

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	webui "github.com/dseif0x/mdl/web"

	"github.com/dseif0x/mdl/internal/config"
	"github.com/dseif0x/mdl/internal/provider"
	"github.com/dseif0x/mdl/internal/provider/applemusic"
	"github.com/dseif0x/mdl/internal/provider/soundcloud"
	"github.com/dseif0x/mdl/internal/provider/youtube"
	"github.com/dseif0x/mdl/internal/provider/ytdlp"
	"github.com/dseif0x/mdl/internal/server"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg := config.Load()

	// Register providers. Adding a new service (e.g. Deezer) is a one-liner here
	// plus its package implementing provider.Provider.
	registry := provider.NewRegistry()
	ytdlpClient := ytdlp.New(cfg.YtdlpBinary)
	registry.Register(youtube.New(ytdlpClient, cfg.DownloadDir))
	registry.Register(soundcloud.New(ytdlpClient, cfg.DownloadDir))
	registry.Register(applemusic.New(cfg.AppleMusicCmd))

	for _, p := range registry.List() {
		log.Info("registered provider", "name", p.Name(), "display", p.DisplayName())
	}

	static, err := fs.Sub(webui.FS, ".")
	if err != nil {
		return err
	}

	srv := server.New(registry, static, cfg.DownloadTimeout, log)
	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		// WriteTimeout is intentionally unset: downloads can be long-running and
		// are bounded by cfg.DownloadTimeout instead.
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.Addr, "download_dir", cfg.DownloadDir)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	}
}
