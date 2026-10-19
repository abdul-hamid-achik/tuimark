package parse_test

import (
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

func firstProgress(n *ir.Node) *ir.Node {
	if n == nil {
		return nil
	}
	if n.Tag == "progress" {
		return n
	}
	for _, c := range n.Children {
		if p := firstProgress(c); p != nil {
			return p
		}
	}
	return nil
}

// SPEC v0.4b §6.4.1: indeterminate="true" makes a progress a spinner only
// in the current language, without bind and value, whatever the order of
// the attributes; every other case is a bar, with the diagnostics of the
// section.
func TestIndeterminateMakesASpinner(t *testing.T) {
	const withdrawn = `indeterminate="true" cannot be combined with bind or value`
	for _, tc := range []struct {
		name, tui, attrs string
		spinner          bool
		diags            []string
	}{
		{"plain", "<tui>", `indeterminate="true"`, true, nil},
		{"false", "<tui>", `indeterminate="false"`, false, nil},
		{"false with value", "<tui>", `indeterminate="false" value="40"`, false, nil},
		{"value after", "<tui>", `indeterminate="true" value="40"`, false, []string{"V003 " + withdrawn}},
		{"value before", "<tui>", `value="40" indeterminate="true"`, false, []string{"V003 " + withdrawn}},
		{"bind", "<tui>", `indeterminate="true" bind="p"`, false, []string{"V003 " + withdrawn}},
		{"an invalid bind path still counts", "<tui>", `bind="1 2" indeterminate="true"`, false,
			[]string{`V003 bind="1 2" is not a path`, "V003 " + withdrawn}},
		{"yes", "<tui>", `indeterminate="yes"`, false, []string{`V003 indeterminate="yes": want true or false`}},
		{"a path", "<tui>", `indeterminate="p"`, false, []string{`V003 indeterminate="p": want true or false`}},
		{"empty", "<tui>", `indeterminate=""`, false, []string{`V003 indeterminate="": want true or false`}},
		{"version 3", `<tui version="3">`, `indeterminate="true"`, false, []string{`V002 attribute "indeterminate" on <progress>` + ir.VersionHintCurrent}},
		{"version 2", `<tui version="2">`, `indeterminate="true"`, false, []string{`V002 attribute "indeterminate" on <progress>` + ir.VersionHintCurrent}},
		{"version 1", `<tui version="1">`, `indeterminate="true"`, false, []string{`V002 attribute "indeterminate" on <progress>` + ir.VersionHintCurrent}},
		// A pinned document checks the item no further.
		{"version 3, a bad value", `<tui version="3">`, `indeterminate="yes"`, false, []string{`V002 attribute "indeterminate" on <progress>` + ir.VersionHintCurrent}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := parse.Parse([]byte(tc.tui+`<screen id="s"><progress id="p" `+tc.attrs+`/></screen></tui>`), "t.tui")
			var got []string
			for _, x := range d.Diags {
				got = append(got, x.Code+" "+x.Msg)
			}
			if strings.Join(got, "\n") != strings.Join(tc.diags, "\n") {
				t.Errorf("diagnostics:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(tc.diags, "\n"))
			}
			p := firstProgress(d.Root)
			if p == nil {
				t.Fatal("no progress node")
			}
			if p.Spinner != tc.spinner {
				t.Errorf("Spinner = %v, want %v", p.Spinner, tc.spinner)
			}
			if v, ok := p.Attrs["indeterminate"]; !ok || v == "" && !strings.Contains(tc.attrs, `indeterminate=""`) {
				t.Errorf("the attribute stays in attrs as written: %q %v", v, ok)
			}
		})
	}
}

// indeterminate on any other tag is an attribute that tag does not take.
func TestIndeterminateOnOtherTagsIsV002(t *testing.T) {
	d := parse.Parse([]byte(`<tui><screen id="s"><box id="p" indeterminate="true"/><text indeterminate="true">x</text></screen></tui>`), "t.tui")
	n := 0
	for _, x := range d.Diags {
		if x.Code == "V002" && strings.Contains(x.Msg, `"indeterminate"`) {
			n++
		}
	}
	if n != 2 {
		t.Errorf("%d V002 for indeterminate, want 2: %v", n, d.Diags)
	}
}

// The spinner is legal anywhere a progress is: an each template and a
// list <item> included, with no V002 or V004 (SPEC §6.4.1, test 210).
func TestSpinnerInTemplateAndListItem(t *testing.T) {
	d := parse.Parse([]byte(`<tui><screen id="s">
<list id="l" each="rows as r" key="r.id"><item><progress indeterminate="true"/><text>{r.name}</text></item></list>
<col each="rows as r"><progress indeterminate="true"/></col>
</screen></tui>`), "t.tui")
	if len(d.Diags) != 0 {
		t.Errorf("diagnostics: %v", d.Diags)
	}
}
