// toasts: a queue of toasts for kit/components/toast.tui, with expiry.
//
// Copy this file into your project and change the package name. It depends
// only on github.com/abdul-hamid-achik/tuimark.
//
// The component is one row: toast.tui shows the toast in toast.text while
// toast.show is true, with the level in toast.is_ok, toast.is_warn and
// toast.is_danger (none of them is info). This helper keeps a queue behind
// that row. The first toast shows at once and later ones wait their turn; each
// shows for its time to live, and Tick (called by Every) moves to the next.
// Esc, bound to toast_dismiss in the component, drops the one on screen.
package kit

import (
	"fmt"
	"sync"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

// toastSetter is *tuimark.App or *tuimark.Batch.
type toastSetter = interface {
	Set(path string, v any) error
}

// ToastOptions configures NewToasts. The zero value is usable.
type ToastOptions struct {
	Path string        // the store object the component reads (default "toast")
	Max  int           // the most toasts held, the one on screen included (default 5)
	TTL  time.Duration // the time to live of a toast pushed with ttl 0 (default 3s)
	Now  func() time.Time
}

type toastItem struct {
	kind, text string
	ttl        time.Duration
	deadline   time.Time // set when it goes on screen
}

// Toasts is the queue. All methods are safe to call from any goroutine.
type Toasts struct {
	app  *tuimark.App
	opts ToastOptions

	mu    sync.Mutex
	queue []toastItem // queue[0] is on screen
}

// NewToasts creates the queue for app and writes the paths the component
// reads (as hidden), unless the store already has them.
func NewToasts(app *tuimark.App, opts ToastOptions) (*Toasts, error) {
	if opts.Path == "" {
		opts.Path = "toast"
	}
	if opts.Max <= 0 {
		opts.Max = 5
	}
	if opts.TTL <= 0 {
		opts.TTL = 3 * time.Second
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	t := &Toasts{app: app, opts: opts}
	if _, ok := app.Get(opts.Path + ".show"); !ok {
		if err := app.Batch(func(b *tuimark.Batch) error { return t.write(b, nil) }); err != nil {
			return nil, err
		}
	}
	return t, nil
}

// Register installs the handler of toast_dismiss, which drops the toast on
// screen. A host with its own toast_dismiss handler calls Dismiss from it.
func (t *Toasts) Register() {
	t.app.On("toast_dismiss", func(tuimark.Event) error { return t.Dismiss() })
}

// Start calls Tick every d (a second is plenty for toasts counted in seconds)
// through app.Every, and returns the function that stops it.
func (t *Toasts) Start(d time.Duration) (stop func()) {
	return t.app.Every(d, t.Tick)
}

// Push queues a toast of kind "info", "ok", "warn" or "danger". ttl is how
// long it stays on screen once it is shown (0 is the default). If the queue
// is full, the oldest toast that is waiting is dropped; if only one toast is
// allowed, the new one replaces it.
func (t *Toasts) Push(kind, text string, ttl time.Duration) error {
	return t.app.Batch(func(b *tuimark.Batch) error { return t.PushTo(b, kind, text, ttl) })
}

// PushTo is Push into a Batch (or an App) you are already writing, as a
// handler that sets other paths in the same frame does.
func (t *Toasts) PushTo(s toastSetter, kind, text string, ttl time.Duration) error {
	switch kind {
	case "info", "ok", "warn", "danger":
	default:
		return fmt.Errorf("kit: toast kind %q is not info, ok, warn or danger", kind)
	}
	if ttl <= 0 {
		ttl = t.opts.TTL
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	item := toastItem{kind: kind, text: text, ttl: ttl}
	switch {
	case len(t.queue) < t.opts.Max:
		t.queue = append(t.queue, item)
	case len(t.queue) > 1:
		t.queue = append(append(t.queue[:1:1], t.queue[2:]...), item) // drop the oldest waiting
	default:
		t.queue = []toastItem{item} // Max is 1: replace
	}
	if len(t.queue) == 1 || t.opts.Max == 1 {
		return t.showHead(s)
	}
	return nil
}

// Dismiss drops the toast on screen; the next one, if any, takes its place.
func (t *Toasts) Dismiss() error {
	return t.app.Batch(func(b *tuimark.Batch) error { return t.DismissTo(b) })
}

// DismissTo is Dismiss into a Batch you are already writing.
func (t *Toasts) DismissTo(s toastSetter) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.queue) == 0 {
		return nil
	}
	t.queue = t.queue[1:]
	return t.showHead(s)
}

// Tick drops the toast on screen when its time is up and shows the next. Call
// it from app.Every (Start does), or from a test with a fixed clock. It writes
// only when something changes.
func (t *Toasts) Tick(b *tuimark.Batch) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.queue) == 0 || t.opts.Now().Before(t.queue[0].deadline) {
		return nil
	}
	t.queue = t.queue[1:]
	return t.showHead(b)
}

// Len is the number of toasts held, the one on screen included.
func (t *Toasts) Len() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.queue)
}

// showHead puts queue[0] on screen (starting its clock), or hides the row when
// the queue is empty. The caller holds t.mu.
func (t *Toasts) showHead(s toastSetter) error {
	if len(t.queue) == 0 {
		return t.write(s, nil)
	}
	head := &t.queue[0]
	head.deadline = t.opts.Now().Add(head.ttl)
	return t.write(s, head)
}

func (t *Toasts) write(s toastSetter, item *toastItem) error {
	p := t.opts.Path
	text, kind := "", ""
	if item != nil {
		text, kind = item.text, item.kind
	}
	for _, w := range []struct {
		path string
		v    any
	}{
		{p + ".text", text},
		{p + ".is_ok", kind == "ok"},
		{p + ".is_warn", kind == "warn"},
		{p + ".is_danger", kind == "danger"},
		{p + ".show", item != nil},
	} {
		if err := s.Set(w.path, w.v); err != nil {
			return err
		}
	}
	return nil
}
