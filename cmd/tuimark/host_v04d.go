package main

// The 0.4d additions to the host protocol (SPEC v0.4d §19.1): the fd 3
// messages `append` and `commit`, the `op` of a `batch` entry, and the
// `messages` member of `ready` (decision 254). The protocol stays 1.

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
)

// hostMessages is `ready.messages`: the fd 3 types this child accepts, with
// or without --inline (SPEC v0.4d §19.1). A child older than 0.4d has no
// such member and accepts exactly the first five.
var hostMessages = []string{"set", "bind", "batch", "get", "reply", "append", "commit"}

// flagWasSet reports whether the flag called name was given on the command
// line, so that an explicit empty value is told apart from an absent flag.
// fs must already have been parsed.
func flagWasSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}

// The `op` of a `batch` entry (SPEC v0.4d §19.1): "set", the default, or
// "append".
const (
	opSet    = "set"
	opAppend = "append"
)

// entryOp reads the `op` of one `batch` entry: absent is "set"; anything
// but the string "set" or "append" (null included) is not an op.
func entryOp(it wireMsg) (string, bool) {
	if _, present := it["op"]; !present {
		return opSet, true
	}
	op, ok := it.str("op")
	if !ok || (op != opSet && op != opAppend) {
		return "", false
	}
	return op, true
}

// handleAppend is `append` (SPEC v0.4d §19.1): {"type": "append", "path",
// "value", optional "id"}, acked as a `set` is.
func (b *hostBridge) handleAppend(id json.RawMessage, msg wireMsg) {
	path, ok := msg.str("path")
	if !ok {
		b.sendError(id, nil, `"append" needs a string "path"`)
		return
	}
	raw, ok := msg["value"]
	if !ok {
		b.sendError(id, nil, `"append" needs "value"`)
		return
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		b.sendError(id, nil, fmt.Sprintf(`bad "value": %v`, err))
		return
	}
	b.ack(id, b.app.Append(path, v))
}

// commitRequest is one parsed `commit` message (SPEC v0.4d §19.1): the
// members of §18.3's Commit, each as the wire gave it. A member the
// message does not carry is its zero value; whether lines and template are
// the right ones is the rule of §18.3, judged when the commit is applied,
// not by the parse.
type commitRequest struct {
	Lines    []any
	Template string
	With     string
	Props    map[string]string
}

// parseCommit reads the members of a `commit` message. It is malformed
// (the error is the text of the `error` message) when `lines` is not an
// array, `template` or `with` is not a string, or `props` is not an
// object of strings; every member is optional here.
func parseCommit(msg wireMsg) (commitRequest, error) {
	var c commitRequest
	if raw, ok := msg["lines"]; ok {
		if jsonKind(raw) != 'a' {
			return c, errors.New(`"commit" needs "lines" to be an array`)
		}
		if err := json.Unmarshal(raw, &c.Lines); err != nil {
			return c, fmt.Errorf(`bad "lines": %v`, err)
		}
		if c.Lines == nil {
			c.Lines = []any{}
		}
	}
	if _, ok := msg["template"]; ok {
		s, ok := msg.str("template")
		if !ok {
			return c, errors.New(`"commit" needs "template" to be a string`)
		}
		c.Template = s
	}
	if _, ok := msg["with"]; ok {
		s, ok := msg.str("with")
		if !ok {
			return c, errors.New(`"commit" needs "with" to be a string`)
		}
		c.With = s
	}
	if raw, ok := msg["props"]; ok {
		if jsonKind(raw) != 'o' {
			return c, errors.New(`"commit" needs "props" to be an object of strings`)
		}
		var props map[string]json.RawMessage
		if err := json.Unmarshal(raw, &props); err != nil {
			return c, fmt.Errorf(`bad "props": %v`, err)
		}
		c.Props = make(map[string]string, len(props))
		for k, v := range props {
			var s string
			if jsonKind(v) != 's' || json.Unmarshal(v, &s) != nil {
				return c, errors.New(`"commit" needs "props" to be an object of strings`)
			}
			c.Props[k] = s
		}
	}
	return c, nil
}

// handleCommit is `commit` (SPEC v0.4d §19.1): {"type": "commit", "lines"
// or "template" with optional "with" and "props", optional "id"}. A
// malformed one gets an `error` message; a well-formed one is applied, and
// acked (once its rows are written) or answered with the rule's error.
func (b *hostBridge) handleCommit(id json.RawMessage, msg wireMsg) {
	c, err := parseCommit(msg)
	if err != nil {
		b.sendError(id, nil, err.Error())
		return
	}
	b.ack(id, b.applyCommit(c))
}

// applyCommit is Commit (SPEC v0.4d §18.3) for the bridge, and the one
// place the inline session joins the bridge. Until the bridge runs a
// session inline every session is the full-screen one, where a commit is
// the error below and prints nothing.
func (b *hostBridge) applyCommit(c commitRequest) error {
	_ = c
	return errors.New("commit: not in inline mode")
}
