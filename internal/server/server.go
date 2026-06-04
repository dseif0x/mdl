// Package server exposes the REST API and serves the embedded frontend.
package server

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/dseif0x/mdl/internal/provider"
	"github.com/dseif0x/mdl/internal/queue"
)

// Server wires the provider registry, download queue and static assets to HTTP
// handlers.
type Server struct {
	registry *provider.Registry
	queue    *queue.Queue
	static   fs.FS
	log      *slog.Logger
}

// New builds a Server. static is the filesystem holding the frontend assets.
func New(registry *provider.Registry, q *queue.Queue, static fs.FS, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{
		registry: registry,
		queue:    q,
		static:   static,
		log:      log,
	}
}

// Handler returns the root http.Handler with all routes mounted.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/providers", s.handleProviders)
	mux.HandleFunc("GET /api/search", s.handleSearch)
	mux.HandleFunc("POST /api/download", s.handleDownload)
	mux.HandleFunc("GET /api/jobs", s.handleListJobs)
	mux.HandleFunc("GET /api/jobs/{id}", s.handleGetJob)
	mux.HandleFunc("POST /api/jobs/{id}/cancel", s.handleCancelJob)
	mux.Handle("GET /", http.FileServer(http.FS(s.static)))
	return logRequests(s.log, mux)
}

// providerInfo is the public shape of a provider in the API.
type providerInfo struct {
	Name         string                `json:"name"`
	DisplayName  string                `json:"display_name"`
	Capabilities provider.Capabilities `json:"capabilities"`
}

func (s *Server) handleProviders(w http.ResponseWriter, _ *http.Request) {
	list := s.registry.List()
	out := make([]providerInfo, 0, len(list))
	for _, p := range list {
		out = append(out, providerInfo{
			Name:         p.Name(),
			DisplayName:  p.DisplayName(),
			Capabilities: p.Capabilities(),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"providers": out})
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("provider")
	query := r.URL.Query().Get("q")
	if name == "" || query == "" {
		writeError(w, http.StatusBadRequest, "both 'provider' and 'q' query parameters are required")
		return
	}
	p, ok := s.registry.Get(name)
	if !ok {
		writeError(w, http.StatusNotFound, "unknown provider: "+name)
		return
	}

	limit := parseLimit(r.URL.Query().Get("limit"), 10)
	tracks, err := p.Search(r.Context(), query, provider.SearchOptions{Limit: limit})
	if err != nil {
		if errors.Is(err, provider.ErrNotSupported) {
			writeError(w, http.StatusNotImplemented, "provider does not support search")
			return
		}
		s.log.Error("search failed", "provider", name, "err", err)
		writeError(w, http.StatusBadGateway, "search failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tracks": tracks})
}

// downloadRequest is the body for POST /api/download. Provide a full track
// (as returned by search) or just a provider + url.
type downloadRequest struct {
	Provider string          `json:"provider"`
	URL      string          `json:"url"`
	Track    *provider.Track `json:"track"`
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	var req downloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	name := req.Provider
	if name == "" && req.Track != nil {
		name = req.Track.Provider
	}
	if name == "" {
		writeError(w, http.StatusBadRequest, "'provider' is required")
		return
	}
	if _, ok := s.registry.Get(name); !ok {
		writeError(w, http.StatusNotFound, "unknown provider: "+name)
		return
	}

	track := provider.Track{}
	if req.Track != nil {
		track = *req.Track
	}
	if req.URL != "" {
		track.URL = req.URL
	}
	if track.URL == "" {
		writeError(w, http.StatusBadRequest, "a track 'url' is required")
		return
	}
	track.Provider = name

	// Enqueue and return immediately; the client polls the job for progress.
	job := s.queue.Submit(name, track)
	s.log.Info("download enqueued", "job", job.ID, "provider", name, "url", track.URL)
	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) handleListJobs(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"jobs": s.queue.List()})
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	job, ok := s.queue.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "unknown job")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleCancelJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.queue.Cancel(id) {
		writeError(w, http.StatusConflict, "job is unknown or already finished")
		return
	}
	job, _ := s.queue.Get(id)
	writeJSON(w, http.StatusOK, job)
}
