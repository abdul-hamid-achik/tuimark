package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"time"
)

// fakeAPI is the in-process server the demo sends to: a few /users routes,
// /health, /slow (sleeps, and gives up when the client does), /error (500),
// and a 404 for everything else. It is the standard library's httptest
// server, so the demo needs no network beyond 127.0.0.1.
type fakeAPI struct {
	srv  *httptest.Server
	slow time.Duration // how long /slow sleeps
	reqs atomic.Int64
}

func newFakeAPI(slow time.Duration) *fakeAPI {
	f := &fakeAPI{slow: slow}
	f.srv = httptest.NewServer(f)
	return f
}

// URL is the server's base URL, such as http://127.0.0.1:41233.
func (f *fakeAPI) URL() string { return f.srv.URL }

// Close shuts the server down. Open requests are cut, so /slow never holds
// it up.
func (f *fakeAPI) Close() {
	f.srv.CloseClientConnections()
	f.srv.Close()
}

var users = []map[string]any{
	{"id": 1, "name": "Grace Hopper", "email": "grace@example.com", "role": "admin"},
	{"id": 2, "name": "Alan Turing", "email": "alan@example.com", "role": "member"},
	{"id": 3, "name": "Margaret Hamilton", "email": "margaret@example.com", "role": "member"},
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	n := f.reqs.Add(1)
	w.Header().Set("Server", "fakeapi/1.0")
	w.Header().Set("X-Request-Id", fmt.Sprintf("req-%04d", n))
	path := strings.TrimSuffix(r.URL.Path, "/")
	if path == "" {
		path = "/"
	}
	switch {
	case path == "/health" && r.Method == http.MethodGet:
		reply(w, http.StatusOK, map[string]any{"status": "ok", "version": "1.4.2", "checks": map[string]any{"db": "ok", "cache": "ok"}})
	case path == "/users" && r.Method == http.MethodGet:
		w.Header().Set("Cache-Control", "max-age=30")
		reply(w, http.StatusOK, users)
	case path == "/users" && r.Method == http.MethodPost:
		in, ok := readJSONObject(w, r)
		if !ok {
			return
		}
		created := map[string]any{
			"id": 4, "roles": []string{"member"}, "active": true, "created_at": "2026-10-17T09:30:00Z",
			"links": map[string]any{"self": "/users/4", "orders": "/users/4/orders"},
		}
		for k, v := range in {
			created[k] = v
		}
		created["id"] = 4
		w.Header().Set("Location", "/users/4")
		reply(w, http.StatusCreated, created)
	case path == "/users/1" && r.Method == http.MethodGet:
		reply(w, http.StatusOK, users[0])
	case path == "/users/1" && (r.Method == http.MethodPut || r.Method == http.MethodPatch):
		in, ok := readJSONObject(w, r)
		if !ok {
			return
		}
		out := map[string]any{}
		if r.Method == http.MethodPatch {
			for k, v := range users[0] {
				out[k] = v
			}
		}
		for k, v := range in {
			out[k] = v
		}
		out["id"] = 1
		reply(w, http.StatusOK, out)
	case path == "/users/1" && r.Method == http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)
	case path == "/slow" && r.Method == http.MethodGet:
		select {
		case <-time.After(f.slow):
			reply(w, http.StatusOK, map[string]any{"slow": true, "waited_ms": f.slow.Milliseconds()})
		case <-r.Context().Done():
		}
	case path == "/error":
		reply(w, http.StatusInternalServerError, map[string]any{
			"error": "internal_error", "message": "the upstream database is unavailable", "request_id": w.Header().Get("X-Request-Id"),
		})
	default:
		reply(w, http.StatusNotFound, map[string]any{"error": "not_found", "method": r.Method, "path": r.URL.Path})
	}
}

// reply writes v as compact JSON; the client does the pretty-printing.
func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func readJSONObject(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	raw, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	var in map[string]any
	if err := json.Unmarshal(raw, &in); err != nil {
		reply(w, http.StatusBadRequest, map[string]any{"error": "invalid_json", "message": err.Error()})
		return nil, false
	}
	return in, true
}
