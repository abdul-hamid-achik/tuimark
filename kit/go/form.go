// form: per-field validation rules for kit/components/field.tui.
//
// Copy this file into your project and change the package name. It depends
// on github.com/abdul-hamid-achik/tuimark, and on route.go (Instance, Part):
// copy that file with this one.
//
// A form is named by the id prefix of its inputs: the form "profile" owns
// the inputs profile__name and profile__email and the store object
// forms.profile:
//
//	forms.profile.<field>.value   written by the input's bind
//	forms.profile.<field>.error   "" or the first failing rule's message
//	forms.profile.valid           true when no field fails a rule
//
// The component's actions are form_validate (the inputs' on:change) and
// form_submit (on:submit, the Save button, ctrl+s). One handler of each serves
// every form, routed by the instance of the event's source.
package kit

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/abdul-hamid-achik/tuimark"
)

// Rule checks a value and returns the message to show, or "" when it is fine.
// Write your own as a function; the ones below cover the usual cases. Except
// Required, they accept an empty value, so an optional field can still have a
// format.
type Rule func(value string) string

// Required fails on an empty value or one of only spaces.
func Required(msg string) Rule {
	return func(v string) string {
		if strings.TrimSpace(v) == "" {
			return msg
		}
		return ""
	}
}

// Pattern fails when a non-empty value does not match re.
func Pattern(re *regexp.Regexp, msg string) Rule {
	return func(v string) string {
		if v != "" && !re.MatchString(v) {
			return msg
		}
		return ""
	}
}

// MinLen fails when a non-empty value has fewer than n characters.
func MinLen(n int, msg string) Rule {
	return func(v string) string {
		if v != "" && utf8.RuneCountInString(v) < n {
			return msg
		}
		return ""
	}
}

// MaxLen fails when a value has more than n characters.
func MaxLen(n int, msg string) Rule {
	return func(v string) string {
		if utf8.RuneCountInString(v) > n {
			return msg
		}
		return ""
	}
}

// Forms holds the forms of one app.
type Forms struct {
	app *tuimark.App

	mu    sync.Mutex
	forms map[string]*Form
}

// NewForms creates the registry for app.
func NewForms(app *tuimark.App) *Forms {
	return &Forms{app: app, forms: map[string]*Form{}}
}

// Form is one form: its fields, their rules and what to do on a valid submit.
type Form struct {
	fs       *Forms
	name     string
	fields   []string
	rules    map[string][]Rule
	onSubmit func(values map[string]string) error
}

// Add creates the form called name (the id prefix of its inputs). Describe it
// with Field and OnSubmit, then call Reset to create its paths.
func (fs *Forms) Add(name string) *Form {
	f := &Form{fs: fs, name: name, rules: map[string][]Rule{}}
	fs.mu.Lock()
	fs.forms[name] = f
	fs.mu.Unlock()
	return f
}

// Field declares a field and the rules it must pass, checked in order; the
// first failure is its error.
func (f *Form) Field(name string, rules ...Rule) *Form {
	if _, ok := f.rules[name]; !ok {
		f.fields = append(f.fields, name)
	}
	f.rules[name] = rules
	return f
}

// OnSubmit sets what runs when the form is submitted with every rule passing.
// values holds each field's text.
func (f *Form) OnSubmit(fn func(values map[string]string) error) *Form {
	f.onSubmit = fn
	return f
}

// Check is the first failing rule's message for a field's value, or "". An
// unknown field has no rules.
func (f *Form) Check(field, value string) string {
	for _, r := range f.rules[field] {
		if msg := r(value); msg != "" {
			return msg
		}
	}
	return ""
}

func (f *Form) path(field string) string { return "forms." + f.name + "." + field }

// values reads every field's text from the store, with overrides on top.
func (f *Form) values(override map[string]string) map[string]string {
	out := make(map[string]string, len(f.fields))
	for _, name := range f.fields {
		v, ok := override[name]
		if !ok {
			s, _ := f.fs.app.Get(f.path(name) + ".value")
			v, _ = s.(string)
		}
		out[name] = v
	}
	return out
}

// Reset writes the form's paths: each field's value (from initial, and "" for
// a field it does not mention), no errors, and valid computed from those
// values.
func (f *Form) Reset(initial map[string]string) error {
	values := make(map[string]string, len(f.fields))
	for _, name := range f.fields {
		values[name] = initial[name]
	}
	return f.fs.app.Batch(func(b *tuimark.Batch) error {
		var errs []error
		for _, name := range f.fields {
			errs = append(errs, b.Set(f.path(name)+".value", values[name]), b.Set(f.path(name)+".error", ""))
		}
		return errors.Join(append(errs, b.Set("forms."+f.name+".valid", f.valid(values)))...)
	})
}

func (f *Form) valid(values map[string]string) bool {
	for _, name := range f.fields {
		if f.Check(name, values[name]) != "" {
			return false
		}
	}
	return true
}

// formFor is the form an event belongs to: the one named by the instance of
// its source, else the only form there is (ctrl+s fires with whatever has
// focus as its source).
func (fs *Forms) formFor(source string) (*Form, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if f, ok := fs.forms[Instance(source)]; ok {
		return f, nil
	}
	if len(fs.forms) == 1 {
		for _, f := range fs.forms {
			return f, nil
		}
	}
	return nil, fmt.Errorf("kit: no form for source %q", source)
}

// Validate is the handler of form_validate: it checks the field that changed
// (its name is the part of the source id), shows or clears its error, and
// updates valid. Other fields' errors are left alone, so a field the user
// has not reached yet does not turn red.
func (fs *Forms) Validate(ev tuimark.Event) error {
	f, err := fs.formFor(ev.Source)
	if err != nil {
		return nil // not ours: an error from a handler would end Run
	}
	field := Part(ev.Source)
	value, _ := ev.Value.(string)
	if _, ok := f.rules[field]; !ok {
		return nil // an input of the form that has no rules
	}
	values := f.values(map[string]string{field: value})
	return fs.app.Batch(func(b *tuimark.Batch) error {
		return errors.Join(
			b.Set(f.path(field)+".error", f.Check(field, value)),
			b.Set("forms."+f.name+".valid", f.valid(values)),
		)
	})
}

// Submit is the handler of form_submit: it checks every field and shows every
// error. With none, it calls the form's OnSubmit. Otherwise it moves focus to
// the first field that failed (the input <form>__<field>).
func (fs *Forms) Submit(ev tuimark.Event) error {
	f, err := fs.formFor(ev.Source)
	if err != nil {
		return nil // not ours (ctrl+s with focus outside any form, and several forms): an error would end Run
	}
	values := f.values(nil)
	first := ""
	err = fs.app.Batch(func(b *tuimark.Batch) error {
		errs := []error{}
		for _, name := range f.fields {
			msg := f.Check(name, values[name])
			if msg != "" && first == "" {
				first = name
			}
			errs = append(errs, b.Set(f.path(name)+".error", msg))
		}
		return errors.Join(append(errs, b.Set("forms."+f.name+".valid", first == ""))...)
	})
	if err != nil {
		return err
	}
	if first != "" {
		return fs.app.Set("@focus", "#"+f.name+"__"+first)
	}
	if f.onSubmit != nil {
		return f.onSubmit(values)
	}
	return nil
}

// Register installs Validate and Submit as the handlers of form_validate and
// form_submit.
func (fs *Forms) Register() {
	fs.app.On("form_validate", fs.Validate)
	fs.app.On("form_submit", fs.Submit)
}
