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
	"github.com/dseif0x/mdl/internal/queue"
)

// fakeProvider is a controllable Provider for tests. Download runs in a worker
// goroutine, so it signals completion over a channel.
type fakeProvider struct {
	tracks   []provider.Track
	children []provider.Track
	called   chan provider.Track
}

func newFakeProvider() *fakeProvider {
	return &fakeProvider{called: make(chan provider.Track, 1)}
}

func (f *fakeProvider) Name() string        { return "fake" }
func (f *fakeProvider) DisplayName() string { return "Fake" }
func (f *fakeProvider) Capabilities() provider.Capabilities {
	return provider.Capabilities{Search: true, Download: true, SearchAlbums: true, Browse: true}
}
func (f *fakeProvider) Search(_ context.Context, _ string, _ provider.SearchOptions) ([]provider.Track, error) {
	return f.tracks, nil
}
func (f *fakeProvider) Browse(_ context.Context, _ provider.Track) ([]provider.Track, error) {
	return f.children, nil
}
func (f *fakeProvider) Download(_ context.Context, t provider.Track, _ provider.DownloadOptions) (*provider.DownloadResult, error) {
	if f.called != nil {
		f.called <- t
	}
	return &provider.DownloadResult{Files: []string{"/downloads/" + t.Title + ".mp3"}}, nil
}

func newTestServer(t *testing.T, p provider.Provider) http.Handler {
	t.Helper()
	reg := provider.NewRegistry()
	reg.Register(p)
	q := queue.New(
		func(ctx context.Context, name string, track provider.Track) (*provider.DownloadResult, error) {
			pr, ok := reg.Get(name)
			if !ok {
				return nil, context.Canceled
			}
			return pr.Download(ctx, track, provider.DownloadOptions{})
		},
		queue.Options{Workers: 1, Timeout: time.Minute},
	)
	q.Start(context.Background())
	t.Cleanup(q.Shutdown)
	static := fstest.MapFS{"index.html": {Data: []byte("ok")}}
	return New(reg, q, static, nil).Handler()
}

func TestProvidersEndpoint(t *testing.T) {
	h := newTestServer(t, newFakeProvider())
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
	h := newTestServer(t, newFakeProvider())
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/search?provider=fake", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestSearchUnknownProvider(t *testing.T) {
	h := newTestServer(t, newFakeProvider())
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/search?provider=nope&q=x", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func TestSearchReturnsTracks(t *testing.T) {
	fp := newFakeProvider()
	fp.tracks = []provider.Track{{Title: "Song", Provider: "fake", URL: "u"}}
	h := newTestServer(t, fp)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/search?provider=fake&q=song", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var body struct {
		Items []provider.Track `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 1 || body.Items[0].Title != "Song" {
		t.Fatalf("unexpected items: %+v", body.Items)
	}
}

func TestBrowseReturnsChildren(t *testing.T) {
	fp := newFakeProvider()
	fp.children = []provider.Track{{Kind: provider.KindTrack, Title: "Track 1", Provider: "fake", URL: "u"}}
	h := newTestServer(t, fp)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/browse?provider=fake&kind=album&id=42", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rr.Code, rr.Body.String())
	}
	var body struct {
		Items []provider.Track `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 1 || body.Items[0].Title != "Track 1" {
		t.Fatalf("unexpected items: %+v", body.Items)
	}
}

func TestBrowseRequiresKind(t *testing.T) {
	h := newTestServer(t, newFakeProvider())
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/browse?provider=fake", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestDownloadByURL(t *testing.T) {
	fp := newFakeProvider()
	h := newTestServer(t, fp)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/download",
		strings.NewReader(`{"provider":"fake","url":"https://example/x"}`))
	h.ServeHTTP(rr, req)

	// Download is async: the API accepts the job and a worker runs it.
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202 (body=%s)", rr.Code, rr.Body.String())
	}
	var job queue.Job
	if err := json.Unmarshal(rr.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	if job.ID == "" || job.Status != queue.StatusQueued {
		t.Fatalf("unexpected job: %+v", job)
	}

	select {
	case got := <-fp.called:
		if got.URL != "https://example/x" {
			t.Fatalf("download got URL %q", got.URL)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("provider Download was not called")
	}
}

func TestDownloadRequiresURL(t *testing.T) {
	h := newTestServer(t, newFakeProvider())
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/download", strings.NewReader(`{"provider":"fake"}`))
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}
