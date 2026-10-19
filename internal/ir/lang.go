package ir

// Lang is the language a document is checked and rendered in (SPEC v0.4b
// §5.1). A document that writes version="1", "2", or "3" is pinned to
// that version's tuimark 0.4.1 meaning and gets Lang1, Lang2, or Lang3; a
// <tui> without version is the current language, LangCurrent. There is no
// version="4": the number only exists for the tools (the catalog's since
// and current, IR 0.4, the language server's vocabulary filter). The
// spike root <app>, an invalid version (version="4" included), and a
// document that fails before its root is read are Lang1.
type Lang int

const (
	Lang1 Lang = 1
	Lang2 Lang = 2
	Lang3 Lang = 3
	// Lang4 is the language 0.4b extends: a <tui> without version in
	// tuimark 0.4.2. Rules that 0.4b introduces are gated on it (l >=
	// Lang4), never on LangCurrent, so a later cut that moves the current
	// language keeps them, and keeps their catalog since, at 4.
	Lang4 Lang = 4
	// LangCurrent is the language of a <tui> without version (SPEC v0.4b
	// §5.1): the number the catalog's current member, IR 0.4, and the
	// language server use for it.
	LangCurrent = Lang4
)

// V2 reports whether the language accepts the version="2" vocabulary
// (SPEC §5.1): version="2", version="3", and the current language.
func (l Lang) V2() bool { return l >= Lang2 }

// V3 reports whether the language holds the version="3" rules (SPEC v0.3
// §5.1): version="3" and the current language.
func (l Lang) V3() bool { return l >= Lang3 }

// V4 reports whether the language holds what 0.4b adds and changes (SPEC
// v0.4b §5.1): only a <tui> without version, never a pinned document.
func (l Lang) V4() bool { return l >= Lang4 }

// VersionV4 is the source IR version `tuimark ir` emits for a <tui>
// without version (SPEC v0.4b §5.1, §13.1, §23.6).
const VersionV4 = "0.4"

// VersionHintCurrent ends the message of every V001, V002, and V003 that
// an item only the current language accepts gets in a document that
// writes version="1", "2", or "3" (SPEC v0.4b §5.1). The ctrl+i, ctrl+j,
// and ctrl+m V003 is a restriction, not vocabulary, and never carries it
// (decision 83).
const VersionHintCurrent = ` (requires a <tui> without version)`

// KindsCurrent are the tags only the current language accepts (SPEC
// v0.4b §5.1, §6): in a pinned document each is V001 with
// VersionHintCurrent. Each 0.4b phase adds its tags here when it lays
// them out and paints them; none is declared ahead of its phase.
var KindsCurrent = []string{}

// IsKindCurrent reports whether tag is one of the current-language tags.
func IsKindCurrent(tag string) bool {
	for _, k := range KindsCurrent {
		if k == tag {
			return true
		}
	}
	return false
}

// IsKindLang reports whether tag is in the catalog of a document of the
// given language: Kinds, plus KindsV2 from Lang2 on, plus KindsCurrent
// in Lang4.
func IsKindLang(tag string, l Lang) bool {
	return IsKindIn(tag, l.V2()) || (l.V4() && IsKindCurrent(tag))
}
