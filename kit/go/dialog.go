// dialog: open kit/components/confirm.tui and route its answer.
//
// Copy this file into your project and change the package name. It depends
// on github.com/abdul-hamid-achik/tuimark, and on route.go (Instance): copy
// that file with this one.
//
// The component reads confirm.open, confirm.title and confirm.message and the
// host writes confirm.open and confirm.result ("confirmed" or "cancelled").
// Its actions are dialog_confirm and dialog_cancel, fired by whichever button
// was pressed; one handler of each serves every dialog, told apart by the
// instance of the event's source (confirm__yes: confirm). A dialog is opened
// by your own code, from your own action, with Confirm.
package kit

import (
	"errors"
	"sync"

	"github.com/abdul-hamid-achik/tuimark"
)

// Request is a question to put to the user.
type Request struct {
	Title, Body string
	OnConfirm   func() error // the yes answer
	OnCancel    func() error // the no answer, esc included; optional
}

// Dialogs routes the answers of the dialogs of one app.
type Dialogs struct {
	app *tuimark.App

	mu      sync.Mutex
	pending map[string]Request // by instance
}

// NewDialogs creates the router for app. Call Register to install its
// handlers.
func NewDialogs(app *tuimark.App) *Dialogs {
	return &Dialogs{app: app, pending: map[string]Request{}}
}

// Register installs Confirmed and Cancelled as the handlers of
// dialog_confirm and dialog_cancel.
func (d *Dialogs) Register() {
	d.app.On("dialog_confirm", d.Confirmed)
	d.app.On("dialog_cancel", d.Cancelled)
}

// Confirm opens the dialog instance with a title and a message and runs
// onConfirm if the user confirms. Cancelling just closes it.
func (d *Dialogs) Confirm(inst, title, body string, onConfirm func() error) error {
	return d.Open(inst, Request{Title: title, Body: body, OnConfirm: onConfirm})
}

// Open is Confirm with both answers. Opening an instance that is already
// open replaces its question.
func (d *Dialogs) Open(inst string, r Request) error {
	d.mu.Lock()
	d.pending[inst] = r
	d.mu.Unlock()
	return d.app.Batch(func(b *tuimark.Batch) error {
		return errors.Join(
			b.Set(inst+".title", r.Title), b.Set(inst+".message", r.Body),
			b.Set(inst+".result", "none"), b.Set(inst+".open", true),
		)
	})
}

// Confirmed is the handler of dialog_confirm.
func (d *Dialogs) Confirmed(ev tuimark.Event) error { return d.answer(ev, "confirmed") }

// Cancelled is the handler of dialog_cancel.
func (d *Dialogs) Cancelled(ev tuimark.Event) error { return d.answer(ev, "cancelled") }

func (d *Dialogs) answer(ev tuimark.Event, result string) error {
	inst := Instance(ev.Source)
	d.mu.Lock()
	r, ok := d.pending[inst]
	delete(d.pending, inst)
	d.mu.Unlock()
	// Close first, so a callback that opens the dialog again is not undone.
	if err := d.app.Batch(func(b *tuimark.Batch) error {
		return errors.Join(b.Set(inst+".open", false), b.Set(inst+".result", result))
	}); err != nil {
		return err
	}
	if !ok {
		return nil // the dialog was already answered
	}
	if result == "confirmed" && r.OnConfirm != nil {
		return r.OnConfirm()
	}
	if result == "cancelled" && r.OnCancel != nil {
		return r.OnCancel()
	}
	return nil
}
