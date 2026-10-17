package main

import (
	"fmt"
	"math/rand"
	"strings"
	"time"
)

// Levels, from the loudest to the quietest. The names double as the keys
// of the "levels" toggles in the store and of the class guards in app.tui.
const (
	lvlError = "error"
	lvlWarn  = "warn"
	lvlInfo  = "info"
	lvlDebug = "debug"
)

var levelNames = []string{lvlError, lvlWarn, lvlInfo, lvlDebug}

// entry is one log line as a source reports it. The host adds the id.
// Cont marks a continuation line (a stack frame): it belongs to the entry
// before it and is shown under it without a timestamp or level.
type entry struct {
	TS    string // "15:04:05.000"
	Level string
	Svc   string
	Msg   string
	Cont  bool
}

// source produces the next entries of the stream. Both implementations
// are driven by the host's Every tick; neither knows about the view.
type source interface {
	Name() string
	Next() ([]entry, error)
}

// tsLayout is the timestamp the view shows: wall time with milliseconds.
const tsLayout = "15:04:05.000"

// fakeEpoch is where the simulated clock starts, so a seed gives the same
// timestamps on every machine and every run.
var fakeEpoch = time.Date(2026, 10, 17, 9, 41, 0, 0, time.UTC)

// tickStep is how much simulated time one generator tick covers; it is
// also the default -interval, so the stream runs about as fast as its clock.
const tickStep = 250 * time.Millisecond

// fake is the seeded generator of realistic API-service logs: requests
// through the gateway, database queries, cache hits and misses, retries,
// and the occasional warning or error with a stack trace.
type fake struct {
	rng *rand.Rand
	now time.Time
}

func newFake(seed int64) *fake {
	return &fake{rng: rand.New(rand.NewSource(seed)), now: fakeEpoch}
}

func (g *fake) Name() string { return "api-gateway (demo)" }

// Next advances the simulated clock by one tick and returns that tick's
// lines: one to four events, an event being one line or a line with its
// stack.
func (g *fake) Next() ([]entry, error) {
	n := 1 + g.rng.Intn(4)
	if g.rng.Intn(40) == 0 {
		n += 6 + g.rng.Intn(6) // a burst
	}
	var out []entry
	t := g.now
	for i := 0; i < n; i++ {
		t = t.Add(time.Duration(g.rng.Intn(int(tickStep/time.Millisecond)/n+1)) * time.Millisecond)
		out = append(out, g.event(t.Format(tsLayout))...)
	}
	g.now = g.now.Add(tickStep)
	return out, nil
}

func (g *fake) pick(xs []string) string { return xs[g.rng.Intn(len(xs))] }
func (g *fake) between(lo, hi int) int  { return lo + g.rng.Intn(hi-lo+1) }
func (g *fake) hex(n int) string {
	const digits = "0123456789abcdef"
	b := make([]byte, n)
	for i := range b {
		b[i] = digits[g.rng.Intn(16)]
	}
	return string(b)
}

var routes = []string{
	"GET /api/orders/%d", "GET /api/users/%d", "GET /api/products?page=%d",
	"POST /api/orders", "POST /api/checkout", "PUT /api/cart/%d",
	"DELETE /api/sessions/%d", "GET /api/inventory/%d", "GET /healthz",
}

var tables = []string{"orders", "users", "inventory", "sessions", "carts", "invoices"}

// event returns the lines of one random event. The branch weights keep
// the stream mostly info and debug, with about one line in ten a warning
// or an error.
func (g *fake) event(ts string) []entry {
	line := func(level, svc, msg string) []entry {
		return []entry{{TS: ts, Level: level, Svc: svc, Msg: msg}}
	}
	switch r := g.rng.Intn(100); {
	case r < 30: // a request through the gateway
		route := g.pick(routes)
		if strings.Contains(route, "%d") {
			route = fmt.Sprintf(route, g.between(1, 9999))
		}
		status, level := 200, lvlInfo
		switch s := g.rng.Intn(100); {
		case s < 3:
			status, level = []int{500, 502, 504}[g.rng.Intn(3)], lvlError
		case s < 11:
			status, level = []int{404, 429, 401}[g.rng.Intn(3)], lvlWarn
		case s < 18:
			status = 304
		case strings.HasPrefix(route, "POST"):
			status = 201
		}
		ms := g.between(2, 90)
		if level == lvlError {
			ms = g.between(900, 3000)
		}
		return line(level, "gateway", fmt.Sprintf("%s %d %dms req=%s", route, status, ms, g.hex(6)))
	case r < 45: // a database query
		if g.rng.Intn(25) == 0 {
			return line(lvlWarn, "postgres", fmt.Sprintf("slow query (%dms): SELECT o.* FROM orders o JOIN invoices i ON i.order_id = o.id", g.between(500, 1400)))
		}
		t := g.pick(tables)
		q := g.pick([]string{
			"SELECT * FROM %s WHERE id = $1",
			"UPDATE %s SET updated_at = now() WHERE id = $1",
			"SELECT count(*) FROM %s WHERE status = $1",
			"INSERT INTO %s (id, payload) VALUES ($1, $2)",
		})
		return line(lvlDebug, "postgres", fmt.Sprintf(q+" [%d.%dms]", t, g.between(0, 6), g.between(0, 9)))
	case r < 60: // the cache
		key := fmt.Sprintf("%s:%d", g.pick([]string{"user", "product", "cart", "session"}), g.between(1, 2000))
		if g.rng.Intn(3) == 0 {
			return line(lvlDebug, "cache", "miss key="+key+" ttl=300s")
		}
		return line(lvlDebug, "cache", "hit key="+key)
	case r < 68: // auth
		if g.rng.Intn(6) == 0 {
			return line(lvlWarn, "auth", fmt.Sprintf("invalid token signature from 10.0.%d.%d", g.between(0, 9), g.between(2, 250)))
		}
		return line(lvlInfo, "auth", fmt.Sprintf("token validated sub=user:%d scope=orders:read", g.between(1, 2000)))
	case r < 78: // background jobs
		job := g.pick([]string{"email.send", "report.build", "inventory.sync", "search.reindex"})
		return line(lvlInfo, "worker", fmt.Sprintf("job %s id=%d done in %d.%ds", job, g.between(8000, 9999), g.between(0, 4), g.between(0, 9)))
	case r < 85: // an upstream retry
		return line(lvlWarn, "billing", fmt.Sprintf("upstream timeout after 2000ms, retry %d/3 in %dms", g.between(1, 3), g.between(1, 8)*100))
	case r < 89: // business events
		return line(lvlInfo, "orders", fmt.Sprintf("order created id=%d items=%d total=$%d.%02d", g.between(100000, 999999), g.between(1, 6), g.between(5, 400), g.between(0, 99)))
	case r < 92: // a plain error
		return line(lvlError, "billing", fmt.Sprintf("charge failed: card_declined order=%d", g.between(100000, 999999)))
	case r < 96: // an error with a stack
		return g.stack(ts)
	default:
		return line(lvlInfo, "system", fmt.Sprintf("gc pause %dus heap=%dMB goroutines=%d", g.between(80, 900), g.between(120, 480), g.between(60, 400)))
	}
}

// stackTraces are the errors that come with detail lines.
var stackTraces = []struct {
	svc, msg string
	frames   []string
}{
	{"orders", "panic: nil pointer dereference in OrderHandler.Get", []string{
		"goroutine %d [running]:",
		"orders.(*Handler).Get(handler.go:142)",
		"net/http.HandlerFunc.ServeHTTP(server.go:2136)",
		"gateway.withAuth.func1(middleware.go:58)",
	}},
	{"postgres", "deadlock detected: process 4821 waits for ShareLock on transaction 90113", []string{
		"while updating tuple (12,7) in relation \"inventory\"",
		"store.(*Inventory).Reserve(inventory.go:88)",
		"orders.(*Service).Place(service.go:211)",
	}},
	{"billing", "context deadline exceeded calling payments-api", []string{
		"payments.(*Client).Capture(client.go:97)",
		"billing.(*Worker).capture(worker.go:164)",
		"retry attempts exhausted (3/3)",
	}},
}

func (g *fake) stack(ts string) []entry {
	s := stackTraces[g.rng.Intn(len(stackTraces))]
	out := []entry{{TS: ts, Level: lvlError, Svc: s.svc, Msg: s.msg}}
	for _, f := range s.frames {
		if strings.Contains(f, "%d") {
			f = fmt.Sprintf(f, g.between(100, 900))
		}
		out = append(out, entry{Level: lvlError, Msg: "↳ " + f, Cont: true})
	}
	return out
}
