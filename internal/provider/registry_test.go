package provider

import (
	"context"
	"testing"
)

type stubProvider struct{ name string }

func (s stubProvider) Name() string               { return s.name }
func (s stubProvider) DisplayName() string        { return s.name }
func (s stubProvider) Capabilities() Capabilities { return Capabilities{} }
func (s stubProvider) Search(context.Context, string, SearchOptions) ([]Track, error) {
	return nil, nil
}
func (s stubProvider) Download(context.Context, Track, DownloadOptions) (*DownloadResult, error) {
	return nil, nil
}

func TestRegistryGetAndList(t *testing.T) {
	r := NewRegistry()
	r.Register(stubProvider{name: "youtube"})
	r.Register(stubProvider{name: "applemusic"})

	if _, ok := r.Get("youtube"); !ok {
		t.Fatal("expected to find youtube")
	}
	if _, ok := r.Get("missing"); ok {
		t.Fatal("did not expect to find missing")
	}

	list := r.List()
	if len(list) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(list))
	}
	// List is sorted by name.
	if list[0].Name() != "applemusic" || list[1].Name() != "youtube" {
		t.Fatalf("list not sorted: %s, %s", list[0].Name(), list[1].Name())
	}
}

func TestRegistryDuplicatePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on duplicate registration")
		}
	}()
	r := NewRegistry()
	r.Register(stubProvider{name: "dup"})
	r.Register(stubProvider{name: "dup"})
}
