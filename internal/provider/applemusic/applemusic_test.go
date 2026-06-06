package applemusic

import (
	"testing"

	"github.com/dseif0x/mdl/internal/provider"
)

func TestParseItemsMixedWrapperTypes(t *testing.T) {
	// Shape mirrors an iTunes lookup response: the looked-up artist first, then
	// an album, then a song.
	data := []byte(`{"resultCount":3,"results":[
		{"wrapperType":"artist","artistId":111,"artistName":"Daft Punk","artistLinkUrl":"https://music.apple.com/us/artist/daft-punk/111"},
		{"wrapperType":"collection","collectionId":222,"collectionName":"Discovery","artistName":"Daft Punk","collectionViewUrl":"https://music.apple.com/us/album/discovery/222","artworkUrl100":"https://x/100x100bb.jpg","trackCount":14,"releaseDate":"2001-03-12T08:00:00Z"},
		{"wrapperType":"track","trackId":333,"trackName":"One More Time","artistName":"Daft Punk","collectionName":"Discovery","trackViewUrl":"https://music.apple.com/us/album/discovery/222?i=333","trackTimeMillis":320357,"artworkUrl100":"https://x/100x100bb.jpg"}
	]}`)

	items, err := parseItems(data, "applemusic")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("got %d items, want 3", len(items))
	}

	artist := items[0]
	if artist.Kind != provider.KindArtist || artist.ID != "111" || artist.Title != "Daft Punk" {
		t.Errorf("bad artist: %+v", artist)
	}

	album := items[1]
	if album.Kind != provider.KindAlbum || album.ID != "222" || album.TrackCount != 14 || album.Year != "2001" {
		t.Errorf("bad album: %+v", album)
	}
	if album.ArtworkURL != "https://x/600x600bb.jpg" {
		t.Errorf("artwork not upgraded: %q", album.ArtworkURL)
	}

	track := items[2]
	if track.Kind != provider.KindTrack || track.ID != "333" || track.Duration != 320 {
		t.Errorf("bad track: %+v", track)
	}
}

func TestParseItemsSkipsEntriesWithoutURL(t *testing.T) {
	data := []byte(`{"results":[{"wrapperType":"track","trackId":1,"trackName":"No URL"}]}`)
	items, err := parseItems(data, "applemusic")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("expected entries without a URL to be skipped, got %+v", items)
	}
}

func TestSearchEntity(t *testing.T) {
	cases := map[provider.SearchType]string{
		"":                     "song",
		provider.SearchSongs:   "song",
		provider.SearchAlbums:  "album",
		provider.SearchArtists: "musicArtist",
	}
	for in, want := range cases {
		got, ok := searchEntity(in)
		if !ok || got != want {
			t.Errorf("searchEntity(%q) = %q,%v want %q,true", in, got, ok, want)
		}
	}
	if _, ok := searchEntity("nonsense"); ok {
		t.Error("expected unknown search type to be rejected")
	}
}
