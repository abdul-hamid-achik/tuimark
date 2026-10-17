package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

// The HTTP side of the app: no Tuimark in this file. send turns a request
// into a response (or into an error shown as one), and everything it
// returns is already text: the pretty-printed body, "1.2 KB", "143 ms".

const (
	userAgent    = "tuimark-http/0.4"
	maxBodyBytes = 1 << 20 // read at most 1 MiB of a response
	maxBodyLines = 400     // show at most this many lines of it
)

// request is what the editor holds, with the URL already resolved.
type request struct {
	Method  string
	URL     string
	Headers []string // "Key: Value" lines
	Body    string
}

// header is one response header.
type header struct{ Name, Value string }

// response is the outcome of one request. A failed request (bad URL,
// timeout, connection refused, cancellation) is a response too: Err holds
// the short reason and Detail the Go error.
type response struct {
	Status  int
	Reason  string
	Dur     time.Duration
	Size    int
	CType   string
	Body    string // pretty-printed, sanitized, capped
	Headers []header
	Err     string
	Detail  string
}

// send performs r under ctx with a timeout. It never returns an error: a
// failure is a response with Err set.
func send(ctx context.Context, hc *http.Client, r request, timeout time.Duration) response {
	start := time.Now()
	fail := func(err error, why string) response {
		short := why
		if short == "" {
			short = classify(ctx, err, timeout)
		}
		return response{
			Reason: short, Err: short, Dur: time.Since(start), CType: "-",
			Body:   "request failed: " + short + "\n\n" + sanitize(err.Error()),
			Detail: err.Error(),
		}
	}

	rctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	target := normalizeURL(r.URL)
	req, err := http.NewRequestWithContext(rctx, strings.ToUpper(strings.TrimSpace(r.Method)), target, strings.NewReader(r.Body))
	if err != nil {
		return fail(err, "bad URL or method")
	}
	for _, line := range r.Headers {
		if k, v, ok := parseHeaderLine(line); ok {
			req.Header.Set(k, v)
		}
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", userAgent)
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "*/*")
	}
	if r.Body != "" && req.Header.Get("Content-Type") == "" && json.Valid([]byte(r.Body)) {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := hc.Do(req)
	if err != nil {
		return fail(err, "")
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	truncated := len(raw) > maxBodyBytes
	if truncated {
		raw = raw[:maxBodyBytes]
	}
	if err != nil && len(raw) == 0 {
		return fail(err, "")
	}

	out := response{
		Status: resp.StatusCode,
		Reason: reasonOf(resp),
		Dur:    time.Since(start),
		Size:   len(raw),
		CType:  mediaType(resp.Header.Get("Content-Type")),
		Body:   prettyBody(raw, resp.Header.Get("Content-Type"), truncated),
	}
	if out.CType == "" {
		out.CType = "-"
	}
	for name, vals := range resp.Header {
		for _, v := range vals {
			out.Headers = append(out.Headers, header{name, sanitize(v)})
		}
	}
	sort.SliceStable(out.Headers, func(i, j int) bool { return out.Headers[i].Name < out.Headers[j].Name })
	return out
}

// normalizeURL adds http:// to a URL typed without a scheme, so
// "localhost:8080/x" is not read as the scheme "localhost".
func normalizeURL(s string) string {
	s = strings.TrimSpace(s)
	if s != "" && !strings.Contains(s, "://") {
		return "http://" + s
	}
	return s
}

// parseHeaderLine splits "Key: Value".
func parseHeaderLine(line string) (k, v string, ok bool) {
	k, v, found := strings.Cut(line, ":")
	k, v = strings.TrimSpace(k), strings.TrimSpace(v)
	if !found || k == "" || strings.ContainsAny(k, " \t") {
		return "", "", false
	}
	return k, v, true
}

func reasonOf(resp *http.Response) string {
	if t := http.StatusText(resp.StatusCode); t != "" {
		return t
	}
	return strings.TrimSpace(strings.TrimPrefix(resp.Status, fmt.Sprint(resp.StatusCode)))
}

func mediaType(ct string) string {
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i]
	}
	return strings.TrimSpace(ct)
}

// classify turns a transport error into a short reason for the status line.
func classify(ctx context.Context, err error, timeout time.Duration) string {
	var dns *net.DNSError
	var uerr *url.Error
	switch {
	case ctx.Err() != nil && errors.Is(ctx.Err(), context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timed out after " + timeout.String()
	case errors.Is(err, syscall.ECONNREFUSED) || strings.Contains(err.Error(), "connection refused"):
		return "connection refused"
	case errors.As(err, &dns):
		return "cannot resolve " + dns.Name
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.As(err, &uerr) && (strings.Contains(uerr.Error(), "unsupported protocol scheme") ||
		strings.Contains(uerr.Error(), "no Host")):
		return "bad URL"
	}
	return "request failed"
}

// prettyBody indents a JSON body, passes text through, and names a binary
// one. The result has no tabs or control characters, and at most
// maxBodyLines lines.
func prettyBody(raw []byte, contentType string, truncated bool) string {
	if len(raw) == 0 {
		return "(empty body)"
	}
	if !utf8.Valid(raw) || bytes.IndexByte(raw, 0) >= 0 {
		return fmt.Sprintf("(binary body, %s)", humanSize(len(raw)))
	}
	text := string(raw)
	if !truncated && (strings.Contains(contentType, "json") || json.Valid(raw)) {
		var buf bytes.Buffer
		if json.Indent(&buf, bytes.TrimSpace(raw), "", "  ") == nil {
			text = buf.String()
		}
	}
	text = sanitize(text)
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) > maxBodyLines {
		more := len(lines) - maxBodyLines
		lines = append(lines[:maxBodyLines], fmt.Sprintf("… %d more lines", more))
	}
	if truncated {
		lines = append(lines, fmt.Sprintf("… body cut at %s", humanSize(maxBodyBytes)))
	}
	return strings.Join(lines, "\n")
}

// sanitize keeps newlines, turns tabs into two spaces, and drops every
// other control character, so a hostile body cannot reach the screen as
// escape sequences.
func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\n':
			b.WriteRune(r)
		case r == '\t':
			b.WriteString("  ")
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) || r == 0xfffd:
			// dropped
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func humanSize(n int) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
}

func humanDuration(d time.Duration) string {
	switch {
	case d < time.Second:
		return fmt.Sprintf("%d ms", d.Milliseconds())
	case d < time.Minute:
		s := fmt.Sprintf("%.2f", d.Seconds())
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
		return s + " s"
	default:
		return d.Round(time.Second).String()
	}
}

// statusClass is "2xx", "3xx", "4xx", "5xx", or "err".
func statusClass(r response) string {
	switch {
	case r.Err != "":
		return "err"
	case r.Status >= 500:
		return "5xx"
	case r.Status >= 400:
		return "4xx"
	case r.Status >= 300:
		return "3xx"
	default:
		return "2xx"
	}
}
