package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// These tests send to the in-process fake API (127.0.0.1 only).

func newTestAPI(t *testing.T, slow time.Duration) (*fakeAPI, *http.Client) {
	t.Helper()
	api := newFakeAPI(slow)
	t.Cleanup(api.Close)
	return api, &http.Client{}
}

func do(t *testing.T, api *fakeAPI, hc *http.Client, method, path, body string, headers ...string) response {
	t.Helper()
	return send(context.Background(), hc, request{Method: method, URL: api.URL() + path, Body: body, Headers: headers}, 5*time.Second)
}

func headerValue(r response, name string) string {
	for _, h := range r.Headers {
		if h.Name == name {
			return h.Value
		}
	}
	return ""
}

func TestGetPrettyPrintsJSON(t *testing.T) {
	api, hc := newTestAPI(t, time.Millisecond)
	r := do(t, api, hc, "GET", "/users/1", "")
	if r.Err != "" || r.Status != 200 || r.Reason != "OK" || statusClass(r) != "2xx" {
		t.Fatalf("response = %+v", r)
	}
	if r.CType != "application/json" {
		t.Errorf("CType = %q", r.CType)
	}
	want := "{\n  \"email\": \"grace@example.com\",\n  \"id\": 1,\n  \"name\": \"Grace Hopper\",\n  \"role\": \"admin\"\n}"
	if r.Body != want {
		t.Errorf("body:\n%s\nwant:\n%s", r.Body, want)
	}
	if r.Size == 0 || humanSize(r.Size) == "" {
		t.Errorf("Size = %d", r.Size)
	}
	if headerValue(r, "Server") != "fakeapi/1.0" || !strings.HasPrefix(headerValue(r, "X-Request-Id"), "req-") {
		t.Errorf("headers = %v", r.Headers)
	}
	for i := 1; i < len(r.Headers); i++ {
		if r.Headers[i-1].Name > r.Headers[i].Name {
			t.Errorf("headers are not sorted: %v", r.Headers)
		}
	}
}

func TestPostEchoesTheCreatedUser(t *testing.T) {
	api, hc := newTestAPI(t, time.Millisecond)
	r := do(t, api, hc, "POST", "/users", `{"name":"Ada","email":"ada@example.com"}`)
	if r.Status != 201 || r.Reason != "Created" || statusClass(r) != "2xx" {
		t.Fatalf("response = %+v", r)
	}
	if !strings.Contains(r.Body, "\"name\": \"Ada\"") || !strings.Contains(r.Body, "\"id\": 4") {
		t.Errorf("body = %s", r.Body)
	}
	if headerValue(r, "Location") != "/users/4" {
		t.Errorf("Location = %q", headerValue(r, "Location"))
	}
	// Not JSON: the fake API refuses it with a 400.
	r = do(t, api, hc, "POST", "/users", "not json")
	if r.Status != 400 || statusClass(r) != "4xx" || !strings.Contains(r.Body, "invalid_json") {
		t.Errorf("bad body response = %d %s", r.Status, r.Body)
	}
}

func TestPutPatchDelete(t *testing.T) {
	api, hc := newTestAPI(t, time.Millisecond)
	put := do(t, api, hc, "PUT", "/users/1", `{"name":"G"}`)
	patch := do(t, api, hc, "PATCH", "/users/1", `{"name":"G"}`)
	if put.Status != 200 || patch.Status != 200 {
		t.Fatalf("put %d patch %d", put.Status, patch.Status)
	}
	if strings.Contains(put.Body, "email") || !strings.Contains(patch.Body, "email") {
		t.Errorf("PUT replaces, PATCH merges:\nput: %s\npatch: %s", put.Body, patch.Body)
	}
	del := do(t, api, hc, "DELETE", "/users/1", "")
	if del.Status != 204 || del.Body != "(empty body)" || del.Size != 0 {
		t.Errorf("delete = %+v", del)
	}
}

func TestErrorAndNotFoundPaths(t *testing.T) {
	api, hc := newTestAPI(t, time.Millisecond)
	e := do(t, api, hc, "GET", "/error", "")
	if e.Status != 500 || e.Reason != "Internal Server Error" || statusClass(e) != "5xx" || e.Err != "" {
		t.Errorf("/error = %+v", e)
	}
	if !strings.Contains(e.Body, `"error": "internal_error"`) {
		t.Errorf("/error body = %s", e.Body)
	}
	n := do(t, api, hc, "GET", "/nope", "")
	if n.Status != 404 || statusClass(n) != "4xx" || !strings.Contains(n.Body, `"path": "/nope"`) {
		t.Errorf("/nope = %+v", n)
	}
	// Wrong method on a known path is also a 404 here.
	if r := do(t, api, hc, "DELETE", "/health", ""); r.Status != 404 {
		t.Errorf("DELETE /health = %d", r.Status)
	}
}

func TestRequestHeadersAndContentType(t *testing.T) {
	var seen http.Header
	srv := newEchoServer(t, &seen)
	hc := &http.Client{}
	send(context.Background(), hc, request{
		Method: "POST", URL: srv, Body: `{"a":1}`,
		Headers: []string{"X-Token: abc", "not a header", "Accept: text/plain"},
	}, 5*time.Second)
	if seen.Get("X-Token") != "abc" || seen.Get("Accept") != "text/plain" {
		t.Errorf("headers = %v", seen)
	}
	if seen.Get("Content-Type") != "application/json" || !strings.HasPrefix(seen.Get("User-Agent"), "tuimark-http/") {
		t.Errorf("defaults missing: %v", seen)
	}
	// An explicit Content-Type wins.
	send(context.Background(), hc, request{Method: "POST", URL: srv, Body: `{"a":1}`, Headers: []string{"content-type: text/x"}}, 5*time.Second)
	if seen.Get("Content-Type") != "text/x" {
		t.Errorf("Content-Type = %q", seen.Get("Content-Type"))
	}
}

func TestSlowEndpointTimesOut(t *testing.T) {
	api, hc := newTestAPI(t, 2*time.Second)
	start := time.Now()
	r := send(context.Background(), hc, request{Method: "GET", URL: api.URL() + "/slow"}, 80*time.Millisecond)
	if r.Err != "timed out after 80ms" || statusClass(r) != "err" || r.Status != 0 {
		t.Errorf("response = %+v", r)
	}
	if !strings.HasPrefix(r.Body, "request failed: timed out after 80ms") {
		t.Errorf("body = %q", r.Body)
	}
	if time.Since(start) > time.Second {
		t.Errorf("took %v: the timeout did not cut the request", time.Since(start))
	}
}

func TestCancelStopsARequest(t *testing.T) {
	api, hc := newTestAPI(t, 2*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(40*time.Millisecond, cancel)
	start := time.Now()
	r := send(ctx, hc, request{Method: "GET", URL: api.URL() + "/slow"}, 5*time.Second)
	if r.Err != "canceled" || r.Reason != "canceled" {
		t.Errorf("response = %+v", r)
	}
	if time.Since(start) > time.Second {
		t.Errorf("cancel took %v", time.Since(start))
	}
}

func TestSlowEndpointCompletesWhenGivenTime(t *testing.T) {
	api, hc := newTestAPI(t, 60*time.Millisecond)
	r := do(t, api, hc, "GET", "/slow", "")
	if r.Status != 200 || r.Dur < 50*time.Millisecond || !strings.Contains(r.Body, `"slow": true`) {
		t.Errorf("response = %+v", r)
	}
}

func TestConnectionRefused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close() // nothing listens here any more
	r := send(context.Background(), &http.Client{}, request{Method: "GET", URL: "http://" + addr + "/x"}, 5*time.Second)
	if r.Err != "connection refused" || r.CType != "-" {
		t.Errorf("response = %+v", r)
	}
	if !strings.Contains(r.Body, "connection refused") || r.Detail == "" {
		t.Errorf("body = %q", r.Body)
	}
}

func TestBadURLs(t *testing.T) {
	for _, u := range []string{"http://[::1", "ftp://example.com/x", "http://"} {
		r := send(context.Background(), &http.Client{}, request{Method: "GET", URL: u}, time.Second)
		if r.Err == "" || !strings.HasPrefix(r.Err, "bad URL") {
			t.Errorf("%q: Err = %q", u, r.Err)
		}
	}
	if r := send(context.Background(), &http.Client{}, request{Method: "NOT A METHOD", URL: "http://127.0.0.1:1"}, time.Second); r.Err == "" {
		t.Error("an invalid method should fail")
	}
}

func TestURLWithoutAScheme(t *testing.T) {
	api, hc := newTestAPI(t, time.Millisecond)
	host := strings.TrimPrefix(api.URL(), "http://")
	r := send(context.Background(), hc, request{Method: "GET", URL: host + "/health"}, 5*time.Second)
	if r.Status != 200 {
		t.Errorf("response = %+v", r)
	}
	if got := normalizeURL("  localhost:8080/x "); got != "http://localhost:8080/x" {
		t.Errorf("normalizeURL = %q", got)
	}
}

func TestSanitizeAndBinaryBodies(t *testing.T) {
	if got := sanitize("a\tb\x1b[31mred\x00\r\nz"); got != "a  b[31mred\nz" {
		t.Errorf("sanitize = %q", got)
	}
	if got := prettyBody([]byte{0xff, 0xfe, 0x00}, "application/octet-stream", false); got != "(binary body, 3 B)" {
		t.Errorf("binary = %q", got)
	}
	if got := prettyBody([]byte("plain text\n"), "text/plain", false); got != "plain text" {
		t.Errorf("text = %q", got)
	}
	big := strings.Repeat("line\n", maxBodyLines+25)
	out := prettyBody([]byte(big), "text/plain", false)
	if lines := strings.Split(out, "\n"); len(lines) != maxBodyLines+1 || lines[maxBodyLines] != "… 25 more lines" {
		t.Errorf("capped body has %d lines, last %q", len(lines), lines[len(lines)-1])
	}
}

func TestFormatting(t *testing.T) {
	for n, want := range map[int]string{0: "0 B", 512: "512 B", 1024: "1.0 KB", 1536: "1.5 KB", 3 << 20: "3.0 MB"} {
		if got := humanSize(n); got != want {
			t.Errorf("humanSize(%d) = %q, want %q", n, got, want)
		}
	}
	for d, want := range map[time.Duration]string{
		0: "0 ms", 143 * time.Millisecond: "143 ms", 1500 * time.Millisecond: "1.5 s",
		2 * time.Second: "2 s", 1234 * time.Millisecond: "1.23 s",
	} {
		if got := humanDuration(d); got != want {
			t.Errorf("humanDuration(%v) = %q, want %q", d, got, want)
		}
	}
	for line, ok := range map[string]bool{"A: b": true, "A:b": true, "A:": true, "no colon": false, ": v": false, "Bad Key: v": false} {
		if _, _, got := parseHeaderLine(line); got != ok {
			t.Errorf("parseHeaderLine(%q) ok = %v, want %v", line, got, ok)
		}
	}
}

// newEchoServer records the headers of the last request it got.
func newEchoServer(t *testing.T, seen *http.Header) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*seen = r.Header.Clone()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}
