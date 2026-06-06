// Package provider defines the abstraction that every music service plugs
// into. A new service (e.g. Deezer) only needs to implement Provider and be
// registered once in main; nothing else in the codebase has to change.
package provider

import (
	"context"
	"errors"
)

// ErrNotSupported is returned by a provider when an operation (such as album
// search or browsing) is not available for that service.
var ErrNotSupported = errors.New("operation not supported by this provider")

// Kind distinguishes the type of an Item.
type Kind string

const (
	KindTrack  Kind = "track"
	KindAlbum  Kind = "album"
	KindArtist Kind = "artist"
)

// SearchType selects what a search query looks for.
type SearchType string

const (
	SearchSongs   SearchType = "song"
	SearchAlbums  SearchType = "album"
	SearchArtists SearchType = "artist"
)

// Track is a single item returned by search or browse: a track, an album, or
// an artist (see Kind). The name is historical — it is the unit the queue and
// download API operate on. Fields are best-effort; providers fill in what they
// can and leave the rest empty.
type Track struct {
	// Kind is "track" (default), "album" or "artist".
	Kind Kind `json:"kind,omitempty"`
	// ID is the provider-local identifier (video id, track/collection/artist id).
	ID string `json:"id"`
	// Provider is the Name() of the provider this item came from.
	Provider string `json:"provider"`
	// Title is the track title, album name, or artist name depending on Kind.
	Title  string `json:"title"`
	Artist string `json:"artist,omitempty"`
	Album  string `json:"album,omitempty"`
	// Duration is the track length in seconds (0 if unknown / not a track).
	Duration int `json:"duration,omitempty"`
	// URL is the canonical web URL used to download or browse the item.
	URL        string `json:"url"`
	ArtworkURL string `json:"artwork_url,omitempty"`
	// TrackCount is the number of tracks in an album (0 if unknown / not an album).
	TrackCount int `json:"track_count,omitempty"`
	// Year is the release year of an album (empty if unknown).
	Year string `json:"year,omitempty"`
}

// SearchOptions controls a search request.
type SearchOptions struct {
	// Limit is the maximum number of results to return. Zero means provider default.
	Limit int
	// Type selects songs (default), albums, or artists.
	Type SearchType
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
	// Search returns matching items for a free-text query. The Type in opts
	// selects songs/albums/artists; providers that don't support a type return
	// ErrNotSupported.
	Search(ctx context.Context, query string, opts SearchOptions) ([]Track, error)
	// Download fetches the given item to disk. For album/artist items the
	// provider downloads all contained tracks.
	Download(ctx context.Context, track Track, opts DownloadOptions) (*DownloadResult, error)
}

// Browser is implemented by providers that can list the children of an item:
// the tracks of an album, or the albums of an artist.
type Browser interface {
	Browse(ctx context.Context, item Track) ([]Track, error)
}

// Capabilities describes the optional features a provider implements, so the
// UI can show only what each provider can do.
type Capabilities struct {
	// Search reports song search.
	Search bool `json:"search"`
	// Download reports the ability to download.
	Download bool `json:"download"`
	// SearchAlbums / SearchArtists report typed search support.
	SearchAlbums  bool `json:"search_albums"`
	SearchArtists bool `json:"search_artists"`
	// Browse reports the ability to list an album's tracks / an artist's albums.
	Browse bool `json:"browse"`
}
