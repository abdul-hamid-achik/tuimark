package css

import (
	"strings"
	"testing"
)

// CheckTokens must catch a bad token even when nothing at the current
// frame would ever surface it through Cascade.Compute: a rule that matches
// no node, a rule inside an @media block that does not match the current
// size, and a :root definition that is itself inside a non-matching
// @media (still a legitimate definition for some other size).
func TestCheckTokensCatchesUnmatchedAndMediaGatedRules(t *testing.T) {
	sh := mustSheet(t, `
#dlg { border-color: $acent; }
.zzz { color: $nope; }
@media (max-rows: 20) { #hdr { background: $pnael; } }
`)
	diags := CheckTokens([]*Sheet{sh}, "dark")
	got := map[string]bool{}
	for _, d := range diags {
		got[d.Msg] = true
	}
	for _, want := range []string{
		`border-color: unknown token $acent`,
		`color: unknown token $nope`,
		`background: unknown token $pnael`,
	} {
		if !got[want] {
			t.Errorf("CheckTokens missed %q; got %v", want, diags)
		}
	}
	for _, d := range diags {
		if d.Code != "V003" || d.Severity != "error" || d.File != "t.tcss" {
			t.Errorf("diag = %+v", d)
		}
	}
}

// A token defined by the theme, or by a :root block anywhere — including
// one gated by an @media the current frame does not match — is not
// reported, and neither is a var(--token) spelling of the same name.
func TestCheckTokensHonorsThemeAndRootAnyMedia(t *testing.T) {
	sh := mustSheet(t, `
@media (max-cols: 40) { :root { --brand: magenta; } }
#a { color: $accent; background: var(--brand); border-color: $brand; }
`)
	if diags := CheckTokens([]*Sheet{sh}, "dark"); len(diags) != 0 {
		t.Errorf("diags = %v, want none (theme token + :root token, even under a non-matching @media)", diags)
	}
	// An empty/unknown theme name falls back to dark, same as NewCascade.
	if diags := CheckTokens([]*Sheet{sh}, ""); len(diags) != 0 {
		t.Errorf("empty theme: diags = %v", diags)
	}
	if diags := CheckTokens([]*Sheet{sh}, "neon"); len(diags) != 0 {
		t.Errorf("unknown theme: diags = %v", diags)
	}
}

// The same bad token reported by two rules with the same file/line/col
// (or the same rule visited twice across sheets) is reported once.
func TestCheckTokensDedupes(t *testing.T) {
	sh := mustSheet(t, "#a { color: $nope; }")
	diags := CheckTokens([]*Sheet{sh, sh}, "dark")
	if len(diags) != 1 {
		t.Fatalf("diags = %v, want exactly one (deduped)", diags)
	}
}

func TestCheckTokensNoFalsePositives(t *testing.T) {
	sh := mustSheet(t, `
:root { --accent: magenta; }
#a { color: $accent; background: red; border: single; }
`)
	if diags := CheckTokens([]*Sheet{sh}, "dark"); len(diags) != 0 {
		t.Errorf("diags = %v", diags)
	}
	if diags := CheckTokens(nil, "dark"); len(diags) != 0 {
		t.Errorf("no sheets: diags = %v", diags)
	}
}

func TestCheckTokensMessageHasNoDollarSignDuplication(t *testing.T) {
	sh := mustSheet(t, "#a { color: $nope; }")
	diags := CheckTokens([]*Sheet{sh}, "dark")
	if len(diags) != 1 || !strings.HasSuffix(diags[0].Msg, "$nope") {
		t.Errorf("diags = %v", diags)
	}
}
