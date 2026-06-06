// Package ytdlp wraps the yt-dlp command-line tool. It is shared by every
// provider that yt-dlp can handle (YouTube, SoundCloud, and many more), so
// those providers stay tiny: they only supply yt-dlp's search prefix.
package ytdlp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dseif0x/mdl/internal/provider"
)

// Client invokes a yt-dlp binary.
type Client struct {
	// Binary is the yt-dlp executable (name on PATH or absolute path).
	Binary string
}

// New returns a Client. An empty binary defaults to "yt-dlp".
func New(binary string) *Client {
	if binary == "" {
		binary = "yt-dlp"
	}
	return &Client{Binary: binary}
}

// entry mirrors the subset of yt-dlp's --dump-json output that we consume.
type entry struct {
	ID         string  `json:"id"`
	Title      string  `json:"title"`
	Uploader   string  `json:"uploader"`
	Channel    string  `json:"channel"`
	Artist     string  `json:"artist"`
	Album      string  `json:"album"`
	Duration   float64 `json:"duration"`
	WebpageURL string  `json:"webpage_url"`
	URL        string  `json:"url"`
	Thumbnail  string  `json:"thumbnail"`
}

func (e entry) toTrack(providerName string) provider.Track {
	url := e.WebpageURL
	if url == "" {
		url = e.URL
	}
	artist := e.Artist
	if artist == "" {
		artist = e.Uploader
	}
	if artist == "" {
		artist = e.Channel
	}
	return provider.Track{
		Kind:       provider.KindTrack,
		ID:         e.ID,
		Provider:   providerName,
		Title:      e.Title,
		Artist:     artist,
		Album:      e.Album,
		Duration:   int(e.Duration),
		URL:        url,
		ArtworkURL: e.Thumbnail,
	}
}

// Search runs `yt-dlp --flat-playlist --dump-json "<prefix><limit>:<query>"`
// and parses the newline-delimited JSON it prints. searchPrefix is yt-dlp's
// extractor search prefix, e.g. "ytsearch" or "scsearch".
func (c *Client) Search(ctx context.Context, searchPrefix, providerName, query string, limit int) ([]provider.Track, error) {
	if limit <= 0 {
		limit = 10
	}
	target := fmt.Sprintf("%s%d:%s", searchPrefix, limit, query)
	cmd := exec.CommandContext(ctx, c.Binary, "--flat-playlist", "--dump-json", target)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, execErr("yt-dlp search", err, stderr.String())
	}

	var tracks []provider.Track
	sc := bufio.NewScanner(bytes.NewReader(out))
	// Search results can carry large metadata blobs; allow long lines.
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue // skip malformed lines rather than failing the whole search
		}
		tracks = append(tracks, e.toTrack(providerName))
	}
	return tracks, sc.Err()
}

// Download fetches url as an MP3 into destDir and returns the written paths.
func (c *Client) Download(ctx context.Context, url, destDir string) (*provider.DownloadResult, error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, fmt.Errorf("create download dir: %w", err)
	}
	cmd := exec.CommandContext(ctx, c.Binary, downloadArgs(url, destDir)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, execErr("yt-dlp download", err, stderr.String())
	}

	var files []string
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			files = append(files, line)
		}
	}
	return &provider.DownloadResult{Files: files, Log: stderr.String()}, nil
}

// downloadArgs builds the yt-dlp argument list for an audio download laid out
// the way Jellyfin expects: <destDir>/<Artist>/<Album>/<Track>.mp3.
//
// Jellyfin scrapes embedded tags rather than file names, so we embed metadata
// and cover art; the folder structure is for tidy browsing. Field fallbacks
// cover YouTube/SoundCloud items that lack album/artist tags, and
// --windows-filenames strips the characters Jellyfin flags as problematic
// (< > : " / \ | ? *).
func downloadArgs(url, destDir string) []string {
	outTmpl := filepath.Join(destDir,
		"%(artist,uploader,channel|Unknown Artist)s",
		"%(album,title)s",
		"%(track,title)s.%(ext)s",
	)
	return []string{
		"--no-playlist",
		"--windows-filenames",
		"--extract-audio", "--audio-format", "mp3", "--audio-quality", "0",
		"--embed-metadata", "--embed-thumbnail",
		"--no-simulate", "--print", "after_move:filepath",
		"-o", outTmpl,
		url,
	}
}

// execErr enriches an exec failure with the tool's stderr, which is where
// yt-dlp writes the actual reason for a failure.
func execErr(what string, err error, stderr string) error {
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		return fmt.Errorf("%s: %w", what, err)
	}
	return fmt.Errorf("%s: %w: %s", what, err, stderr)
}
