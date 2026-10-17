package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// maxBody bounds a message body (64 MiB, the bound the other stdio tools
// of this repository use for a line). A larger Content-Length is a
// malformed header: the server cannot skip a body it will not read.
const maxBody = 64 << 20

// errMalformedHeader is a header the base protocol cannot recover from
// (SPEC §15.12 "Errors"): the server exits 1.
var errMalformedHeader = errors.New("malformed header")

// readMessage reads one message of the LSP base protocol: header lines
// ending in CRLF, a blank line, then Content-Length bytes. io.EOF means
// the input ended between messages (or inside one: nothing more can be
// read either way); a header that is not a "Name: value" line, a missing,
// non-numeric, or oversized Content-Length wraps errMalformedHeader.
// Header names are case-insensitive, and Content-Type is read and
// ignored.
func readMessage(br *bufio.Reader) ([]byte, error) {
	length := -1
	first := true
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			if first && line == "" || err != io.EOF {
				return nil, err
			}
			return nil, io.EOF // a header cut off by the end of input
		}
		first = false
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("%w: %q is not a header line", errMalformedHeader, line)
		}
		if strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || n < 0 || n > maxBody || strings.TrimSpace(value) != strconv.Itoa(n) {
				return nil, fmt.Errorf("%w: bad Content-Length %q", errMalformedHeader, strings.TrimSpace(value))
			}
			length = n
		}
	}
	if length < 0 {
		return nil, fmt.Errorf("%w: no Content-Length", errMalformedHeader)
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(br, body); err != nil {
		return nil, io.EOF // a body cut off by the end of input
	}
	return body, nil
}

// frame is body preceded by its Content-Length header.
func frame(body []byte) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "Content-Length: %d\r\n\r\n", len(body))
	b.Write(body)
	return b.Bytes()
}

// rpcError is a JSON-RPC error object.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// JSON-RPC and LSP error codes the server answers with.
const (
	codeParse          = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeNotInitialized = -32002
)

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result"`
	Error   *rpcError       `json:"error,omitempty"`
}

// marshalResponse encodes a response. A successful one always carries a
// "result" (null for none), which encoding/json's omitempty would drop.
func marshalResponse(id json.RawMessage, result any, e *rpcError) ([]byte, error) {
	if e != nil {
		return json.Marshal(struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Error   *rpcError       `json:"error"`
		}{"2.0", id, e})
	}
	return json.Marshal(response{JSONRPC: "2.0", ID: id, Result: result})
}
