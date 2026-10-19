package kit

import (
	"regexp"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

var emailRE = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// profileForm is the form of kit/components/field.tui.
func profileForm(t *testing.T, initial map[string]string) (*tuimark.App, *Forms, *[]map[string]string) {
	t.Helper()
	app := loadComponent(t, "field", "")
	forms := NewForms(app)
	var submitted []map[string]string
	forms.Add("profile").
		Field("name", Required("Name is required."), MaxLen(20, "Keep it under 20 characters.")).
		Field("email", Required("Email is required."), Pattern(emailRE, "Enter a full address like name@host.")).
		OnSubmit(func(v map[string]string) error {
			submitted = append(submitted, v)
			return nil
		})
	forms.Register()
	f := forms.forms["profile"]
	if err := f.Reset(initial); err != nil {
		t.Fatal(err)
	}
	return app, forms, &submitted
}

func TestFormValidatesAsYouType(t *testing.T) {
	app, _, _ := profileForm(t, map[string]string{"name": "Ada", "email": ""})
	// field.tui focuses profile__name; tab moves to profile__email.
	_, text := play(t, app, "tab", "text:ada@")
	if got := getStr(t, app, "forms.profile.email.error"); got != "Enter a full address like name@host." {
		t.Fatalf("email error = %q", got)
	}
	contains(t, text, "Enter a full address like name@host.")
	notContains(t, text, "We never share it.")
	if getBool(t, app, "forms.profile.valid") {
		t.Error("a half-typed address leaves the form invalid")
	}
	_, text = play(t, app, "text:x.io")
	if got := getStr(t, app, "forms.profile.email.error"); got != "" {
		t.Errorf("a full address should clear the error, got %q", got)
	}
	contains(t, text, "We never share it.")
	if !getBool(t, app, "forms.profile.valid") {
		t.Error("both fields pass: valid")
	}
}

func TestFormLeavesUntouchedFieldsAlone(t *testing.T) {
	app, _, _ := profileForm(t, nil) // both empty and so both invalid
	play(t, app, "text:A")           // typing in name only
	if got := getStr(t, app, "forms.profile.email.error"); got != "" {
		t.Errorf("a field the user has not reached shows no error, got %q", got)
	}
	if getBool(t, app, "forms.profile.valid") {
		t.Error("the empty email still makes the form invalid")
	}
}

func TestSubmitShowsEveryErrorAndFocusesTheFirst(t *testing.T) {
	app, _, submitted := profileForm(t, map[string]string{"name": "", "email": "nope"})
	res, text := play(t, app, "ctrl+s")
	if len(*submitted) != 0 {
		t.Fatal("an invalid form must not submit")
	}
	contains(t, text, "Name is required.")
	contains(t, text, "Enter a full address like name@host.")
	if res.Dump.Focus == nil || *res.Dump.Focus != "profile__name" {
		t.Errorf("focus = %v, want profile__name", res.Dump.Focus)
	}
}

func TestSubmitCallsOnSubmitWithTheValues(t *testing.T) {
	app, _, submitted := profileForm(t, map[string]string{"name": "Ada", "email": "ada@example.com"})
	res, _ := play(t, app, "tab", "enter") // enter in an input fires on:submit
	var fired bool
	for _, e := range res.Events {
		fired = fired || e.Action == "form_submit"
	}
	if !fired || len(*submitted) != 1 {
		t.Fatalf("form_submit fired %v, submitted %v", fired, *submitted)
	}
	if got := (*submitted)[0]; got["name"] != "Ada" || got["email"] != "ada@example.com" {
		t.Errorf("values = %v", got)
	}
	if !getBool(t, app, "forms.profile.valid") {
		t.Error("valid after a good submit")
	}
}

func TestRules(t *testing.T) {
	if Required("r")("  ") != "r" || Required("r")("x") != "" {
		t.Error("Required")
	}
	if MinLen(3, "m")("ab") != "m" || MinLen(3, "m")("abc") != "" || MinLen(3, "m")("") != "" {
		t.Error("MinLen: short fails, empty is left to Required")
	}
	if MaxLen(3, "m")("abcd") != "m" || MaxLen(3, "m")("héé") != "" { // counts characters, not bytes
		t.Error("MaxLen")
	}
	if Pattern(emailRE, "p")("a@b") != "p" || Pattern(emailRE, "p")("") != "" {
		t.Error("Pattern")
	}
	custom := Rule(func(v string) string {
		if v == "admin" {
			return "That name is taken."
		}
		return ""
	})
	f := NewForms(bareApp(t)).Add("x").Field("u", Required("req"), custom)
	if f.Check("u", "") != "req" || f.Check("u", "admin") != "That name is taken." || f.Check("u", "ada") != "" {
		t.Error("rules run in order, the first failure wins")
	}
	if f.Check("unknown", "") != "" {
		t.Error("an unknown field has no rules")
	}
}

func TestEventsRouteByFormAndFallBackToTheOnlyForm(t *testing.T) {
	app := bareApp(t)
	forms := NewForms(app)
	a := forms.Add("a").Field("f", Required("a needs f"))
	b := forms.Add("b").Field("f", Required("b needs f"))
	_, _ = a, b
	_ = a.Reset(nil)
	_ = b.Reset(map[string]string{"f": "ok"})
	if err := forms.Submit(tuimark.Event{Source: "a__save"}); err != nil {
		t.Fatal(err)
	}
	if getStr(t, app, "forms.a.f.error") != "a needs f" || getStr(t, app, "forms.b.f.error") != "" {
		t.Error("only form a is checked")
	}
	if err := forms.Submit(tuimark.Event{Source: "elsewhere"}); err != nil {
		t.Errorf("with two forms, an unrelated source belongs to none and is ignored: %v", err)
	}
}

// An error from a handler ends Run, so an event that is not a form's own
// (ctrl+s with focus on a nav button, several forms) must be a no-op.
func TestForeignEventsDoNotEndTheSession(t *testing.T) {
	app, err := tuimark.Parse(strings.NewReader(`<tui>
  <keymap>
    <bind keys="ctrl+s" action="form_submit"/>
    <bind keys="v" action="form_validate"/>
    <bind keys="s" action="table_sort"/>
  </keymap>
  <screen id="s" focus="#nav">
    <col>
      <button id="nav" label="Nav" on:click="nav_click"/>
      <input id="a__f" bind="forms.a.f.value"/>
      <input id="b__f" bind="forms.b.f.value"/>
    </col>
  </screen>
</tui>`))
	if err != nil {
		t.Fatal(err)
	}
	forms := NewForms(app)
	forms.Add("a").Field("f", Required("a needs f"))
	forms.Add("b").Field("f", Required("b needs f"))
	forms.Register()
	_ = app.Bind("", map[string]any{"all": []any{map[string]any{"id": "x", "n": 1.0}}})
	sp, err := NewSortPage(app, SortPageOptions{Source: "all", Out: "rows", Meta: "meta", Columns: []Column{{"n", "N"}}})
	if err != nil {
		t.Fatal(err)
	}
	app.On("table_sort", func(ev tuimark.Event) error { return sp.Cycle(Part(ev.Source)) }) // Part("nav") is ""
	res, _ := play(t, app, "ctrl+s", "v", "s")
	if res.Quit {
		t.Error("a foreign event ended the session")
	}
	if got := getStr(t, app, "meta.titles.n"); got != "N" {
		t.Errorf("a click on a non-column must not sort: title %q", got)
	}
}

func TestResetClearsAFieldTheInitialValuesDoNotMention(t *testing.T) {
	app, forms, _ := profileForm(t, map[string]string{"name": "Ada", "email": "ada@example.com"})
	_ = forms.Submit(tuimark.Event{Source: "profile__save"})
	_ = app.Set("forms.profile.email.error", "stale")
	if err := forms.forms["profile"].Reset(map[string]string{"name": "Grace"}); err != nil {
		t.Fatal(err)
	}
	if got := getStr(t, app, "forms.profile.name.value"); got != "Grace" {
		t.Errorf("name = %q", got)
	}
	if getStr(t, app, "forms.profile.email.value") != "" || getStr(t, app, "forms.profile.email.error") != "" {
		t.Error("a field the initial values do not mention goes back to an empty value and no error")
	}
	if getBool(t, app, "forms.profile.valid") {
		t.Error("valid is computed from the new values: the email is empty")
	}
}
