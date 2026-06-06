// Package soundcloud implements the Provider interface backed by yt-dlp.
package soundcloud

import (
	"context"

	"github.com/dseif0x/mdl/internal/provider"
	"github.com/dseif0x/mdl/internal/provider/ytdlp"
)

// Provider downloads and searches SoundCloud via yt-dlp.
type Provider struct {
	client      *ytdlp.Client
	downloadDir string
}

// New builds a SoundCloud provider. downloadDir is the default destination.
func New(client *ytdlp.Client, downloadDir string) *Provider {
	return &Provider{client: client, downloadDir: downloadDir}
}

func (p *Provider) Name() string        { return "soundcloud" }
func (p *Provider) DisplayName() string { return "SoundCloud" }

func (p *Provider) Capabilities() provider.Capabilities {
	return provider.Capabilities{Search: true, Download: true}
}

func (p *Provider) Search(ctx context.Context, query string, opts provider.SearchOptions) ([]provider.Track, error) {
	if opts.Type != "" && opts.Type != provider.SearchSongs {
		return nil, provider.ErrNotSupported
	}
	return p.client.Search(ctx, "scsearch", p.Name(), query, opts.Limit)
}

func (p *Provider) Download(ctx context.Context, track provider.Track, opts provider.DownloadOptions) (*provider.DownloadResult, error) {
	dest := opts.DestDir
	if dest == "" {
		dest = p.downloadDir
	}
	return p.client.Download(ctx, track.URL, dest)
}
