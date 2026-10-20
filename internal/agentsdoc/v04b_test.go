package agentsdoc

import (
	"strings"
	"testing"
)

// SPEC §22 (v0.4b): the generated part begins, right after the frozen block,
// with the precedence line; the project briefing carries it too.
func TestPrecedenceLineFollowsTheFrozenBlock(t *testing.T) {
	const line = "Where the frozen lines above and the reference below differ, the reference below wins."
	if PrecedenceLine != line {
		t.Fatalf("PrecedenceLine = %q", PrecedenceLine)
	}
	cases := []struct{ name, md, end string }{
		{"Markdown", Markdown(), spec22Block(t)},
		// The project briefing replaces two lists of the block (SPEC v0.4a
		// §15.11), so look for the line after the last line of the block.
		{"ProjectMarkdown", ProjectMarkdown(), "## Actions\non:select=\"open\" is a name. The host implements it.\n"},
	}
	for _, c := range cases {
		i := strings.Index(c.md, c.end)
		if i < 0 {
			t.Fatalf("%s does not contain the end of the frozen block", c.name)
		}
		rest := c.md[i+len(c.end):]
		if !strings.HasPrefix(rest, "\n"+line+"\n\n") {
			t.Errorf("%s: the generated part does not begin with the precedence line:\n%.200s", c.name, rest)
		}
		if n := strings.Count(c.md, line); n != 1 {
			t.Errorf("%s: the precedence line occurs %d times, want 1", c.name, n)
		}
	}
	// It must also not leave the frozen lines reading as a ban on the
	// current language's templates and tags.
	md := Markdown()
	for _, want := range []string{
		"they do not forbid what the current language has",
		"`<checkbox>`'s `label`", "`<select>`'s `label`", "`<button>`'s body or `label`",
		"`checkbox`, `select`, `include`, `fragment`, `template`, `use`",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("Markdown() is missing %q after the frozen block", want)
		}
	}
}

// SPEC §22 (v0.4b): the current language's defaults are a section of their
// own, one item per change of §31.7, and the Notes do not repeat them.
func TestDefaultsSectionHasOneItemPerChange(t *testing.T) {
	for name, md := range map[string]string{"Markdown": Markdown(), "ProjectMarkdown": ProjectMarkdown()} {
		start := strings.Index(md, "### v0.4b: the defaults of the current language\n")
		notes := strings.Index(md, "### Notes\n")
		comp := strings.Index(md, "### v0.4b: composition\n")
		if start < 0 || notes < 0 || comp < 0 || !(comp < start && start < notes) {
			t.Fatalf("%s: the defaults section is missing or out of order (composition %d, defaults %d, Notes %d)", name, comp, start, notes)
		}
		sec := md[start:notes]
		for _, item := range []string{
			"- **Content-sized `col` and `row`.**",
			"- **The sibling-fr rule, for viewports only.**",
			"- **The `fr` freeze.**",
			"- **Content-sized modals and hints.**",
			"- **`esc` closes a modal.**",
			"- **The modal trap and `<keymap when=\"modal\">`.**",
			"- **`tabs` in the focus cycle, focus kept on the strip.**",
			"- **The built-in focus look.**",
			"- **An `input`'s width.**",
		} {
			if strings.Count(sec, item) != 1 {
				t.Errorf("%s: the defaults section has %d of %q, want 1", name, strings.Count(sec, item), item)
			}
		}
		for _, moved := range []string{"**Layout defaults of the current language.**", "**Keys and focus of the current language.**"} {
			if strings.Contains(md, moved) {
				t.Errorf("%s: %s is still in the Notes", name, moved)
			}
		}
	}
}

// SPEC §22 (v0.4b): the composition conventions are stated.
func TestCompositionConventions(t *testing.T) {
	for name, md := range map[string]string{"Markdown": Markdown(), "ProjectMarkdown": ProjectMarkdown()} {
		for _, want := range []string{
			"`#U__x`", "never `#U`",
			"`with=\"form.email as f\"` makes `f` the path, not a value",
			"static `class` on its `<use>`",
			"Pass a flag as an optional member of the `with` object",
			"Write the size of every template root",
			"`, placed by <use …> at FILE:LINE:COL`",
		} {
			if !strings.Contains(md, want) {
				t.Errorf("%s: the composition conventions are missing %q", name, want)
			}
		}
	}
}

// SPEC §22 (v0.4b): the Notes the current language makes false say which
// language they describe.
func TestNotesNameTheirLanguage(t *testing.T) {
	md := Markdown()
	for _, want := range []string{
		// cross-axis note
		"a modal with no size is 80%×80%; in a `<tui>` without `version`",
		// built-in quit under a modal
		"Under an open modal that routing differs by language",
		// the focus advice
		"a `<tui>` without `version` already reverses a focused `button`, `checkbox`, or `select`",
		// tabs: no focusable=\"true\" needed in the current language
		"a `tabs` of a `<tui>` without `version` is focusable by itself",
		"in a `<tui>` without `version` focus on the strip stays there",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("Markdown() is missing %q", want)
		}
	}
}
