// Package applemusic implements the Provider interface.
//
// Apple Music has no public download API, so this provider splits the work:
//
//   - Search and Browse use Apple's public iTunes Search/Lookup API (no
//     authentication) to find songs, albums and artists and to list an album's
//     tracks or an artist's albums.
//   - Download shells out to zhaarey's apple-music-downloader ("apple-music-dl"),
//     which accepts song, album and artist music.apple.com URLs.
package applemusic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/dseif0x/mdl/internal/provider"
)

const itunesBase = "https://itunes.apple.com"

// Provider searches Apple Music and drives apple-music-dl for downloads.
type Provider struct {
	// downloadCmd is the base command; flags and the URL are appended.
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
	return provider.Capabilities{
		Search:        true,
		Download:      true,
		SearchAlbums:  true,
		SearchArtists: true,
		Browse:        true,
	}
}

// searchEntity maps a SearchType to the iTunes Search API entity.
func searchEntity(t provider.SearchType) (string, bool) {
	switch t {
	case "", provider.SearchSongs:
		return "song", true
	case provider.SearchAlbums:
		return "album", true
	case provider.SearchArtists:
		return "musicArtist", true
	default:
		return "", false
	}
}

func (p *Provider) Search(ctx context.Context, query string, opts provider.SearchOptions) ([]provider.Track, error) {
	entity, ok := searchEntity(opts.Type)
	if !ok {
		return nil, provider.ErrNotSupported
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 25
	}
	q := url.Values{}
	q.Set("term", query)
	q.Set("entity", entity)
	q.Set("media", "music")
	q.Set("limit", strconv.Itoa(limit))

	data, err := p.get(ctx, itunesBase+"/search?"+q.Encode())
	if err != nil {
		return nil, err
	}
	return parseItems(data, p.Name())
}

// Browse lists the tracks of an album or the albums of an artist.
func (p *Provider) Browse(ctx context.Context, item provider.Track) ([]provider.Track, error) {
	if item.ID == "" {
		return nil, fmt.Errorf("applemusic: item id is required to browse")
	}
	q := url.Values{}
	q.Set("id", item.ID)
	q.Set("limit", "200")

	var keep provider.Kind
	switch item.Kind {
	case provider.KindAlbum:
		q.Set("entity", "song")
		keep = provider.KindTrack
	case provider.KindArtist:
		q.Set("entity", "album")
		keep = provider.KindAlbum
	default:
		return nil, provider.ErrNotSupported
	}

	data, err := p.get(ctx, itunesBase+"/lookup?"+q.Encode())
	if err != nil {
		return nil, err
	}
	items, err := parseItems(data, p.Name())
	if err != nil {
		return nil, err
	}
	// The lookup's first result is the album/artist itself; keep only children.
	out := items[:0]
	for _, it := range items {
		if it.Kind == keep {
			out = append(out, it)
		}
	}
	return out, nil
}

func (p *Provider) Download(ctx context.Context, track provider.Track, _ provider.DownloadOptions) (*provider.DownloadResult, error) {
	if track.URL == "" {
		return nil, fmt.Errorf("applemusic: track URL is required")
	}
	// --all-album makes apple-music-dl download every album for an artist URL
	// non-interactively (without it, it prompts on stdin and would hang); it is
	// a no-op for album/song URLs.
	args := append(append([]string{}, p.downloadCmd[1:]...), "--all-album", track.URL)

	// NOTE: apple-music-dl must be configured with `exit-on-error: true`. With
	// false it prompts (fmt.Scanln) and retries on any error; run without a TTY
	// that loops forever and the job never finishes. Stdin is left nil (the null
	// device) so it can never block on input either.
	cmd := exec.CommandContext(ctx, p.downloadCmd[0], args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("apple-music-dl: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return &provider.DownloadResult{Log: strings.TrimSpace(string(out))}, nil
}

// get performs an HTTP GET and returns the body, or an error.
func (p *Provider) get(ctx context.Context, endpoint string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("itunes: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("itunes: unexpected status %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("itunes: read body: %w", err)
	}
	return body, nil
}

// itunesResult mirrors the subset of the iTunes API payload we read. The same
// shape covers songs (wrapperType "track"), albums ("collection") and artists
// ("artist").
type itunesResult struct {
	WrapperType       string  `json:"wrapperType"`
	ArtistID          int64   `json:"artistId"`
	ArtistName        string  `json:"artistName"`
	ArtistLinkURL     string  `json:"artistLinkUrl"`
	CollectionID      int64   `json:"collectionId"`
	CollectionName    string  `json:"collectionName"`
	CollectionViewURL string  `json:"collectionViewUrl"`
	TrackID           int64   `json:"trackId"`
	TrackName         string  `json:"trackName"`
	TrackViewURL      string  `json:"trackViewUrl"`
	TrackCount        int     `json:"trackCount"`
	TrackTimeMs       float64 `json:"trackTimeMillis"`
	ArtworkURL100     string  `json:"artworkUrl100"`
	ReleaseDate       string  `json:"releaseDate"`
}

// parseItems converts an iTunes Search/Lookup response into Tracks.
func parseItems(data []byte, providerName string) ([]provider.Track, error) {
	var resp struct {
		Results []itunesResult `json:"results"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("itunes: decode: %w", err)
	}

	out := make([]provider.Track, 0, len(resp.Results))
	for _, r := range resp.Results {
		switch r.WrapperType {
		case "track":
			if r.TrackViewURL == "" {
				continue
			}
			out = append(out, provider.Track{
				Kind:       provider.KindTrack,
				ID:         strconv.FormatInt(r.TrackID, 10),
				Provider:   providerName,
				Title:      r.TrackName,
				Artist:     r.ArtistName,
				Album:      r.CollectionName,
				Duration:   int(r.TrackTimeMs / 1000),
				URL:        r.TrackViewURL,
				ArtworkURL: artworkLarge(r.ArtworkURL100),
			})
		case "collection":
			if r.CollectionViewURL == "" {
				continue
			}
			out = append(out, provider.Track{
				Kind:       provider.KindAlbum,
				ID:         strconv.FormatInt(r.CollectionID, 10),
				Provider:   providerName,
				Title:      r.CollectionName,
				Artist:     r.ArtistName,
				URL:        r.CollectionViewURL,
				ArtworkURL: artworkLarge(r.ArtworkURL100),
				TrackCount: r.TrackCount,
				Year:       year(r.ReleaseDate),
			})
		case "artist":
			if r.ArtistLinkURL == "" {
				continue
			}
			out = append(out, provider.Track{
				Kind:     provider.KindArtist,
				ID:       strconv.FormatInt(r.ArtistID, 10),
				Provider: providerName,
				Title:    r.ArtistName,
				URL:      r.ArtistLinkURL,
			})
		}
	}
	return out, nil
}

// artworkLarge upgrades the iTunes 100x100 artwork URL to a larger variant.
func artworkLarge(u string) string {
	if u == "" {
		return ""
	}
	return strings.Replace(u, "100x100", "600x600", 1)
}

// year extracts the 4-digit year from an ISO release date.
func year(releaseDate string) string {
	if len(releaseDate) >= 4 {
		return releaseDate[:4]
	}
	return ""
}
