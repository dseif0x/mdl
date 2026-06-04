package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/dseif0x/mdl/internal/provider"
)

// fakeProvider is a controllable Provider for tests.
type fakeProvider struct {
	tracks []provider.Track
	called bool
}

func (f *fakeProvider) Name() string        { return "fake" }
func (f *fakeProvider) DisplayName() string { return "Fake" }
func (f *fakeProvider) Capabilities() provider.Capabilities {
	return provider.Capabilities{Search: true, Download: true}
}
func (f *fakeProvider) Search(_ context.Context, _ string, _ provider.SearchOptions) ([]provider.Track, error) {
	return f.tracks, nil
}
func (f *fakeProvider) Download(_ context.Context, t provider.Track, _ provider.DownloadOptions) (*provider.DownloadResult, error) {
	f.called = true
	return &provider.DownloadResult{Files: []string{"/downloads/" + t.Title + ".mp3"}}, nil
}

func newTestServer(p provider.Provider) http.Handler {
	reg := provider.NewRegistry()
	reg.Register(p)
	static := fstest.MapFS{"index.html": {Data: []byte("ok")}}
	return New(reg, static, time.Minute, nil).Handler()
}

func TestProvidersEndpoint(t *testing.T) {
	h := newTestServer(&fakeProvider{})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/providers", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var body struct {
		Providers []providerInfo `json:"providers"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Providers) != 1 || body.Providers[0].Name != "fake" {
		t.Fatalf("unexpected providers: %+v", body.Providers)
	}
}

func TestSearchRequiresParams(t *testing.T) {
	h := newTestServer(&fakeProvider{})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/search?provider=fake", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestSearchUnknownProvider(t *testing.T) {
	h := newTestServer(&fakeProvider{})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/search?provider=nope&q=x", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func TestSearchReturnsTracks(t *testing.T) {
	fp := &fakeProvider{tracks: []provider.Track{{Title: "Song", Provider: "fake", URL: "u"}}}
	h := newTestServer(fp)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/search?provider=fake&q=song", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var body struct {
		Tracks []provider.Track `json:"tracks"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Tracks) != 1 || body.Tracks[0].Title != "Song" {
		t.Fatalf("unexpected tracks: %+v", body.Tracks)
	}
}

func TestDownloadByURL(t *testing.T) {
	fp := &fakeProvider{}
	h := newTestServer(fp)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/download",
		strings.NewReader(`{"provider":"fake","url":"https://example/x"}`))
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rr.Code, rr.Body.String())
	}
	if !fp.called {
		t.Fatal("provider Download was not called")
	}
}

func TestDownloadRequiresURL(t *testing.T) {
	h := newTestServer(&fakeProvider{})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/download", strings.NewReader(`{"provider":"fake"}`))
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}
