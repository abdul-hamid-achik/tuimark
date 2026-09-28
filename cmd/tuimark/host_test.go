package main

// Unit tests of the host bridge's reply handling (SPEC v0.3b §19.1
// "Acks", "Malformed input", "Events and replies"), with fd 4 a pipe this
// test reads: no terminal, no session. The pty tests (host_pty_test.go)
// cover the same rules end to end.

import (
	"bufio"
	"os"
	"strings"
	"testing"
	"time"
)

// bridgeRig is a bridge with no app and no fd 3, whose fd 4 this test
// reads line by line.
type bridgeRig struct {
	b  *hostBridge
	r  *os.File
	rd *bufio.Reader
}

func newBridgeRig(t *testing.T) *bridgeRig {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	b := newHostBridge(nil, nil, w, time.Second)
	t.Cleanup(func() {
		b.out.finish(nil, 0)
		_ = w.Close()
		_ = r.Close()
	})
	return &bridgeRig{b: b, r: r, rd: bufio.NewReader(r)}
}

// wait makes seq the pending event, as forward does before it writes the
// event.
func (g *bridgeRig) wait(seq int64) *hostWait {
	w := &hostWait{seq: seq, reply: make(chan hostReply, 1)}
	g.b.mu.Lock()
	g.b.seq = max(g.b.seq, seq)
	g.b.pending = w
	g.b.mu.Unlock()
	return w
}

// next reads the next fd 4 line, without its newline.
func (g *bridgeRig) next(t *testing.T) string {
	t.Helper()
	_ = g.r.SetReadDeadline(time.Now().Add(5 * time.Second))
	line, err := g.rd.ReadString('\n')
	if err != nil {
		t.Fatalf("reading fd 4: %v (got %q)", err, line)
	}
	return strings.TrimSuffix(line, "\n")
}

// A reply that carries an id is acked like every message with an id: the
// one that answers the waiting event gets {"type":"ack","id":…,"ok":true}
// (before the handler resumes), and every error about a reply echoes its
// id next to its seq. Without an id nothing changes: a matched reply gets
// nothing, and an error names the seq alone.
func TestHostReplyAcksItsID(t *testing.T) {
	g := newBridgeRig(t)
	w := g.wait(1)
	g.b.handleLine([]byte(`{"type":"reply","id":"a","seq":1}`))
	if got, want := g.next(t), `{"type":"ack","id":"a","ok":true}`; got != want {
		t.Errorf("matched reply with an id: %s, want %s", got, want)
	}
	select {
	case <-w.reply:
	default:
		t.Error("the matched reply was not handed to the waiting handler")
	}
	w = g.wait(2)
	g.b.handleLine([]byte(`{"type":"reply","id":7,"seq":2,"quit":true}`))
	if got, want := g.next(t), `{"type":"ack","id":7,"ok":true}`; got != want {
		t.Errorf("matched quit reply with an id: %s, want %s", got, want)
	}
	if rep := <-w.reply; !rep.quit {
		t.Error("the quit reply lost its quit")
	}
	w = g.wait(3)
	g.b.handleLine([]byte(`{"type":"reply","seq":3}`)) // no id: nothing on fd 4
	<-w.reply
	g.b.mu.Lock()
	g.b.timedOut[4] = true
	g.b.seq = 4
	g.b.mu.Unlock()
	for _, c := range []struct{ line, want string }{
		{`{"type":"reply","id":"b","seq":2}`, `{"type":"error","id":"b","seq":2,"error":"event already answered"}`},
		{`{"type":"reply","id":"c","seq":4}`, `{"type":"error","id":"c","seq":4,"error":"reply after the reply timeout"}`},
		{`{"type":"reply","id":"d","seq":99}`, `{"type":"error","id":"d","seq":99,"error":"no event with this seq"}`},
		{`{"type":"reply","seq":99}`, `{"type":"error","seq":99,"error":"no event with this seq"}`},
		{`{"type":"reply","id":"e","seq":1,"quit":"yes"}`, `{"type":"error","id":"e","seq":1,"error":"\"quit\" must be true or false"}`},
		{`{"type":"reply","id":"f"}`, `{"type":"error","id":"f","error":"\"reply\" needs an integer \"seq\""}`},
	} {
		g.b.handleLine([]byte(c.line))
		if got := g.next(t); got != c.want {
			t.Errorf("%s: %s, want %s", c.line, got, c.want)
		}
	}
}

// seq is an integer judged on its digits, never through float64: any
// integral form names its event (1.0, 1e0), a value outside int64 or with
// a fraction is the malformed-reply error, and an echoed seq is written
// exactly as the parent sent it.
func TestHostReplySeqIsExact(t *testing.T) {
	g := newBridgeRig(t)
	for _, c := range []struct{ line, want string }{
		{`{"type":"reply","seq":9007199254740993}`, `{"type":"error","seq":9007199254740993,"error":"no event with this seq"}`},
		{`{"type":"reply","seq":9223372036854775807}`, `{"type":"error","seq":9223372036854775807,"error":"no event with this seq"}`},
		{`{"type":"reply","seq":9223372036854775808}`, `{"type":"error","error":"\"reply\" needs an integer \"seq\""}`},
		{`{"type":"reply","seq":-9223372036854775809}`, `{"type":"error","error":"\"reply\" needs an integer \"seq\""}`},
		{`{"type":"reply","seq":1e19}`, `{"type":"error","error":"\"reply\" needs an integer \"seq\""}`},
		{`{"type":"reply","seq":1.5}`, `{"type":"error","error":"\"reply\" needs an integer \"seq\""}`},
		{`{"type":"reply","seq":"1"}`, `{"type":"error","error":"\"reply\" needs an integer \"seq\""}`},
		{`{"type":"reply","seq":null}`, `{"type":"error","error":"\"reply\" needs an integer \"seq\""}`},
		{`{"type":"reply","seq":1.0000000000000001}`, `{"type":"error","error":"\"reply\" needs an integer \"seq\""}`},
		{`{"type":"reply","seq":1e3}`, `{"type":"error","seq":1e3,"error":"no event with this seq"}`},
		{`{"type":"reply","seq":1.0,"quit":"yes"}`, `{"type":"error","seq":1.0,"error":"\"quit\" must be true or false"}`},
	} {
		g.b.handleLine([]byte(c.line))
		if got := g.next(t); got != c.want {
			t.Errorf("%s: %s, want %s", c.line, got, c.want)
		}
	}
	// An integral seq in another form still answers its event.
	for _, lit := range []string{"1.0", "1e0", "10e-1"} {
		w := g.wait(1)
		g.b.handleLine([]byte(`{"type":"reply","id":"x","seq":` + lit + `}`))
		if got, want := g.next(t), `{"type":"ack","id":"x","ok":true}`; got != want {
			t.Errorf("seq %s: %s, want %s", lit, got, want)
		}
		select {
		case <-w.reply:
		default:
			t.Errorf("seq %s did not answer event 1", lit)
		}
	}
}
