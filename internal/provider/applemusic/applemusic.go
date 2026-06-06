// Package applemusic implements the Provider interface.
//
// Apple Music has no public download API, so this provider splits the work:
//
//   - Search uses Apple's public iTunes Search API (no authentication needed)
//     to resolve a query into music.apple.com track URLs.
//   - Download shells out to zhaarey's apple-music-downloader ("apple-music-dl").
//     The command is configurable so it can run the binary directly or, in the
//     containerised setup, as `docker exec <container> apple-music-dl`.
package applemusic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/dseif0x/mdl/internal/provider"
)

// Provider searches Apple Music and drives apple-music-dl for downloads.
type Provider struct {
	// downloadCmd is the base command; the track URL is appended as the final arg.
	downloadCmd []string
	httpClient  *http.Client
}

// New builds an Apple Music provider. downloadCmd is the apple-music-dl
// invocation (e.g. ["apple-music-dl"] or
// ["docker", "exec", "apple-music-downloader", "apple-music-dl"]).
func New(downloadCmd []string) *Provider {
	if len(downloadCmd) == 0 {
		downloadCmd = []string{"apple-music-dl"}
	}
	return &Provider{
		downloadCmd: downloadCmd,
		httpClient:  &http.Client{Timeout: 15 * time.Second},
	}
}

func (p *Provider) Name() string        { return "applemusic" }
func (p *Provider) DisplayName() string { return "Apple Music" }

func (p *Provider) Capabilities() provider.Capabilities {
	return provider.Capabilities{Search: true, Download: true}
}

// itunesResponse is the subset of the iTunes Search API payload we read.
type itunesResponse struct {
	Results []struct {
		TrackID        int64   `json:"trackId"`
		TrackName      string  `json:"trackName"`
		ArtistName     string  `json:"artistName"`
		CollectionName string  `json:"collectionName"`
		TrackViewURL   string  `json:"trackViewUrl"`
		ArtworkURL100  string  `json:"artworkUrl100"`
		TrackTimeMs    float64 `json:"trackTimeMillis"`
	} `json:"results"`
}

func (p *Provider) Search(ctx context.Context, query string, opts provider.SearchOptions) ([]provider.Track, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 10
	}
	q := url.Values{}
	q.Set("term", query)
	q.Set("entity", "song")
	q.Set("media", "music")
	q.Set("limit", strconv.Itoa(limit))
	endpoint := "https://itunes.apple.com/search?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("itunes search: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("itunes search: unexpected status %s", resp.Status)
	}

	var data itunesResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("itunes search: decode: %w", err)
	}

	tracks := make([]provider.Track, 0, len(data.Results))
	for _, r := range data.Results {
		if r.TrackViewURL == "" {
			continue // cannot be downloaded without a URL
		}
		tracks = append(tracks, provider.Track{
			ID:         strconv.FormatInt(r.TrackID, 10),
			Provider:   p.Name(),
			Title:      r.TrackName,
			Artist:     r.ArtistName,
			Album:      r.CollectionName,
			Duration:   int(r.TrackTimeMs / 1000),
			URL:        r.TrackViewURL,
			ArtworkURL: artworkLarge(r.ArtworkURL100),
		})
	}
	return tracks, nil
}

func (p *Provider) Download(ctx context.Context, track provider.Track, _ provider.DownloadOptions) (*provider.DownloadResult, error) {
	if track.URL == "" {
		return nil, fmt.Errorf("applemusic: track URL is required")
	}
	args := append(append([]string{}, p.downloadCmd[1:]...), track.URL)
	// NOTE: apple-music-dl must be configured with `exit-on-error: true`. With
	// false it prompts (fmt.Scanln) and retries on any error; run without a TTY
	// that loops forever and the job never finishes. Stdin is left nil (the null
	// device) so it can never block on input either.
	cmd := exec.CommandContext(ctx, p.downloadCmd[0], args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("apple-music-dl: %w: %s", err, strings.TrimSpace(string(out)))
	}
	// apple-music-dl writes into folders inside its own container, so we cannot
	// reliably enumerate the resulting files from here. Surface its log instead.
	return &provider.DownloadResult{Log: strings.TrimSpace(string(out))}, nil
}

// artworkLarge upgrades the iTunes 100x100 artwork URL to a larger variant.
func artworkLarge(u string) string {
	if u == "" {
		return ""
	}
	return strings.Replace(u, "100x100", "600x600", 1)
}
