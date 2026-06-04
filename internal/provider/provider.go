// Package provider defines the abstraction that every music service plugs
// into. A new service (e.g. Deezer) only needs to implement Provider and be
// registered once in main; nothing else in the codebase has to change.
package provider

import (
	"context"
	"errors"
)

// ErrNotSupported is returned by a provider when an operation (such as search)
// is not available for that service.
var ErrNotSupported = errors.New("operation not supported by this provider")

// Track is a single searchable / downloadable item. Fields are best-effort:
// providers fill in what they can and leave the rest empty.
type Track struct {
	// ID is the provider-local identifier (video id, track id, ...).
	ID string `json:"id"`
	// Provider is the Name() of the provider this track came from.
	Provider string `json:"provider"`
	Title    string `json:"title"`
	Artist   string `json:"artist,omitempty"`
	Album    string `json:"album,omitempty"`
	// Duration is the track length in seconds (0 if unknown).
	Duration int `json:"duration,omitempty"`
	// URL is the canonical web URL used to download the track.
	URL        string `json:"url"`
	ArtworkURL string `json:"artwork_url,omitempty"`
}

// SearchOptions controls a search request.
type SearchOptions struct {
	// Limit is the maximum number of results to return. Zero means provider default.
	Limit int
}

// DownloadOptions controls a download request.
type DownloadOptions struct {
	// DestDir overrides the configured download directory for this request.
	// Empty means use the provider's default.
	DestDir string
}

// DownloadResult describes the outcome of a download.
type DownloadResult struct {
	// Files lists the paths that were written, when the provider can determine them.
	Files []string `json:"files,omitempty"`
	// Log is human-readable output from the underlying downloader.
	Log string `json:"log,omitempty"`
}

// Provider is implemented by every music service.
type Provider interface {
	// Name is the stable machine identifier (e.g. "youtube"). Used in the API.
	Name() string
	// DisplayName is the human-friendly label shown in the UI.
	DisplayName() string
	// Capabilities advertises what the provider can do, so the UI can adapt.
	Capabilities() Capabilities
	// Search returns matching tracks for a free-text query. Providers that do
	// not support search return ErrNotSupported.
	Search(ctx context.Context, query string, opts SearchOptions) ([]Track, error)
	// Download fetches the given track to disk.
	Download(ctx context.Context, track Track, opts DownloadOptions) (*DownloadResult, error)
}

// Capabilities describes the optional features a provider implements.
type Capabilities struct {
	Search   bool `json:"search"`
	Download bool `json:"download"`
}
