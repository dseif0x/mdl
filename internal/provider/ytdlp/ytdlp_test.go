package ytdlp

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestDownloadArgsJellyfinLayout(t *testing.T) {
	args := downloadArgs("https://example/watch?v=x", "/music")

	// The -o template must place files at <dest>/<Artist>/<Album>/<Track>.
	i := slices.Index(args, "-o")
	if i < 0 || i+1 >= len(args) {
		t.Fatal("missing -o template argument")
	}
	want := filepath.Join("/music",
		"%(artist,uploader,channel|Unknown Artist)s",
		"%(album,title)s",
		"%(track,title)s.%(ext)s",
	)
	if got := args[i+1]; got != want {
		t.Fatalf("output template = %q, want %q", got, want)
	}

	for _, flag := range []string{"--windows-filenames", "--embed-metadata", "--embed-thumbnail"} {
		if !slices.Contains(args, flag) {
			t.Errorf("expected %s in args", flag)
		}
	}

	if args[len(args)-1] != "https://example/watch?v=x" {
		t.Errorf("URL should be the final argument, got %q", args[len(args)-1])
	}
}
