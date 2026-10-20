package kit

import (
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

func confirmApp(t *testing.T) (*tuimark.App, *Dialogs, *int) {
	t.Helper()
	app := loadComponent(t, "confirm", `{"confirm": {"open": false, "title": "", "body": "", "result": "none"}}`)
	d := NewDialogs(app)
	d.Register()
	deleted := 0
	// The component's trigger button fires dialog_open; the app answers it by
	// asking its question.
	app.On("dialog_open", func(ev tuimark.Event) error {
		return d.Confirm(Instance(ev.Source), "Delete project", "This permanently deletes orbit-api.", func() error {
			deleted++
			return nil
		})
	})
	return app, d, &deleted
}

func TestConfirmOpensTheDialogAndYRunsOnConfirm(t *testing.T) {
	app, _, deleted := confirmApp(t)
	_, text := play(t, app, "enter") // the screen focuses #confirm__open
	contains(t, text, "Delete project")
	contains(t, text, "This permanently deletes orbit-api.")
	// The template's buttons read their labels: Confirm writes the defaults.
	contains(t, text, "[ Cancel ]")
	contains(t, text, "[ Confirm ]")
	if !getBool(t, app, "confirm.open") {
		t.Fatal("the dialog should be open")
	}
	_, text = play(t, app, "y")
	if *deleted != 1 {
		t.Errorf("onConfirm ran %d times", *deleted)
	}
	if getBool(t, app, "confirm.open") || getStr(t, app, "confirm.result") != "confirmed" {
		t.Errorf("open %v result %q", getBool(t, app, "confirm.open"), getStr(t, app, "confirm.result"))
	}
	contains(t, text, "result: confirmed")
}

func TestNAndEscCancelWithoutRunningOnConfirm(t *testing.T) {
	app, _, deleted := confirmApp(t)
	for _, key := range []string{"n", "esc"} {
		play(t, app, "enter", key)
		if *deleted != 0 || getBool(t, app, "confirm.open") || getStr(t, app, "confirm.result") != "cancelled" {
			t.Fatalf("%s: deleted %d open %v result %q", key, *deleted, getBool(t, app, "confirm.open"), getStr(t, app, "confirm.result"))
		}
	}
	// The safe button has focus, so a stray enter cancels too.
	play(t, app, "enter", "enter")
	if *deleted != 0 || getStr(t, app, "confirm.result") != "cancelled" {
		t.Error("enter on the safe button must cancel")
	}
}

func TestAnswersRouteByInstance(t *testing.T) {
	app := bareApp(t)
	d := NewDialogs(app)
	var got []string
	_ = d.Open("one", Request{Title: "1", OnConfirm: func() error { got = append(got, "one yes"); return nil },
		OnCancel: func() error { got = append(got, "one no"); return nil }})
	_ = d.Confirm("two", "2", "", func() error { got = append(got, "two yes"); return nil })
	if !getBool(t, app, "one.open") || !getBool(t, app, "two.open") {
		t.Fatal("both are open")
	}
	_ = d.Cancelled(tuimark.Event{Source: "one__no"})
	_ = d.Confirmed(tuimark.Event{Source: "two__yes"})
	if !equal(got, []string{"one no", "two yes"}) {
		t.Errorf("callbacks: %v", got)
	}
	if getBool(t, app, "one.open") || getStr(t, app, "one.result") != "cancelled" || getStr(t, app, "two.result") != "confirmed" {
		t.Error("each instance keeps its own state")
	}
	// A second answer to a dialog that was answered runs nothing.
	_ = d.Confirmed(tuimark.Event{Source: "two__yes"})
	if len(got) != 2 {
		t.Errorf("a repeated answer ran a callback: %v", got)
	}
}

func TestOpenWritesTheButtonLabels(t *testing.T) {
	app := bareApp(t)
	d := NewDialogs(app)
	_ = d.Open("c", Request{Title: "Delete", Yes: "Delete", No: "Keep"})
	if getStr(t, app, "c.yes") != "Delete" || getStr(t, app, "c.no") != "Keep" {
		t.Errorf("labels %q / %q", getStr(t, app, "c.yes"), getStr(t, app, "c.no"))
	}
	_ = d.Confirm("c", "Again", "", nil)
	if getStr(t, app, "c.yes") != "Confirm" || getStr(t, app, "c.no") != "Cancel" {
		t.Errorf("default labels %q / %q", getStr(t, app, "c.yes"), getStr(t, app, "c.no"))
	}
}

func TestACallbackMayOpenTheDialogAgain(t *testing.T) {
	app := bareApp(t)
	d := NewDialogs(app)
	_ = d.Confirm("c", "first", "", func() error { return d.Confirm("c", "second", "", nil) })
	_ = d.Confirmed(tuimark.Event{Source: "c__yes"})
	if !getBool(t, app, "c.open") || getStr(t, app, "c.title") != "second" {
		t.Errorf("open %v title %q: the follow-up question must stay open", getBool(t, app, "c.open"), getStr(t, app, "c.title"))
	}
}
