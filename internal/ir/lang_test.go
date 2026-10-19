package ir

import "testing"

// SPEC v0.4b §5.1: the four languages and what each holds. Lang 0 (a
// Document built without Parse) reads as version="1".
func TestLangFlags(t *testing.T) {
	for _, c := range []struct {
		l          Lang
		v2, v3, v4 bool
	}{
		{0, false, false, false},
		{Lang1, false, false, false},
		{Lang2, true, false, false},
		{Lang3, true, true, false},
		{Lang4, true, true, true},
	} {
		if c.l.V2() != c.v2 || c.l.V3() != c.v3 || c.l.V4() != c.v4 {
			t.Errorf("Lang(%d): V2=%v V3=%v V4=%v, want %v %v %v", c.l, c.l.V2(), c.l.V3(), c.l.V4(), c.v2, c.v3, c.v4)
		}
	}
	if LangCurrent != 4 || VersionV4 != "0.4" {
		t.Errorf("LangCurrent = %d, VersionV4 = %q; want 4 and 0.4", LangCurrent, VersionV4)
	}
	if VersionHintCurrent != " (requires a <tui> without version)" {
		t.Errorf("VersionHintCurrent = %q", VersionHintCurrent)
	}
}

// IsKindLang adds KindsCurrent only in the current language. F0 leaves
// KindsCurrent empty; a test-only entry shows the gate.
func TestIsKindLang(t *testing.T) {
	if len(KindsCurrent) != 0 {
		t.Fatalf("KindsCurrent = %v: F0 declares no current-language tag", KindsCurrent)
	}
	for _, k := range append(append([]string(nil), Kinds...), KindsV2...) {
		for _, l := range []Lang{Lang1, Lang2, Lang3, Lang4} {
			if IsKindLang(k, l) != IsKindIn(k, l.V2()) {
				t.Errorf("IsKindLang(%q, %d) = %v, want IsKindIn's %v", k, l, IsKindLang(k, l), IsKindIn(k, l.V2()))
			}
		}
	}
	saved := KindsCurrent
	t.Cleanup(func() { KindsCurrent = saved })
	KindsCurrent = []string{"zzcurrent"}
	if !IsKindCurrent("zzcurrent") || !IsKindLang("zzcurrent", Lang4) {
		t.Error("a current-language tag is not a kind of the current language")
	}
	for _, l := range []Lang{Lang1, Lang2, Lang3} {
		if IsKindLang("zzcurrent", l) {
			t.Errorf("a current-language tag is a kind of the pinned Lang(%d)", l)
		}
	}
}

// SPEC v0.4b §8.1, §15.10: the three tokens that never reach a keymap,
// what each arrives as, and the language that rejects them.
func TestNeverMatchKeys(t *testing.T) {
	want := map[string]string{"ctrl+i": "tab", "ctrl+j": "enter", "ctrl+m": "enter"}
	if len(NeverMatchKeys) != len(want) {
		t.Fatalf("NeverMatchKeys = %v", NeverMatchKeys)
	}
	for _, k := range NeverMatchKeys {
		as, ok := ArrivesAs(k)
		if !ok || as != want[k] || !ValidKey(k) {
			t.Errorf("%s: ArrivesAs = %q, %v; ValidKey = %v", k, as, ok, ValidKey(k))
		}
	}
	for _, k := range []string{"ctrl+a", "ctrl+h", "tab", "enter", "i", ""} {
		if _, ok := ArrivesAs(k); ok {
			t.Errorf("ArrivesAs(%q) reports a never-matching token", k)
		}
	}
	if KeysRejectedSince != Lang4 {
		t.Errorf("KeysRejectedSince = %d, want 4", KeysRejectedSince)
	}
}
