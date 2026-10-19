// Package catalog builds the output of `tuimark catalog` (SPEC v0.4a
// §15.10): the closed vocabulary as one JSON-ready value, generated from
// the runtime's own tables so an editor, a language client, or an agent
// reads the catalog instead of scraping AGENTS.md.
//
// The sources are parse.TagAttrs (with its version="2" and version="3"
// additions) and the tag-kind catalog, the keyword table of the enumerated
// attributes, internal/css's property, pseudo-class, and media tables, the
// key-token catalog, the built-in actions, and agentsdoc's diagnostic
// table. The generated part of AGENTS.md comes from the same tables.
package catalog

import (
	"sort"

	"github.com/abdul-hamid-achik/tuimark/internal/agentsdoc"
	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/host"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// Format is the version of the catalog object (the `catalog` member).
// Removing or renaming a member needs a new Format; adding members, items,
// or document versions does not (SPEC v0.4a §15.10).
const Format = 1

// Versions lists the document versions this build accepts as version
// values.
var Versions = []int{1, 2, 3}

// Current is the number of the current language, a <tui> without version
// (SPEC v0.4b §5.1, §15.10): not a writable value, and the since of every
// item only such a document accepts.
const Current = int(ir.LangCurrent)

// Item is a named entry with the lowest document version that accepts it.
type Item struct {
	Name  string `json:"name"`
	Since int    `json:"since"`
}

// Attr is one attribute of a tag. Keywords lists the literal values of an
// enumerated attribute, and is empty for one that takes paths, text,
// numbers, sizes, or an action name. Pattern is set on the class:NAME
// family, whose NAME is open.
type Attr struct {
	Name     string `json:"name"`
	Since    int    `json:"since"`
	Pattern  string `json:"pattern,omitempty"`
	Keywords []Item `json:"keywords"`
}

// Tag is one element of the vocabulary and every attribute it accepts in
// any context.
type Tag struct {
	Name  string `json:"name"`
	Since int    `json:"since"`
	Attrs []Attr `json:"attrs"`
}

// Property is one TCSS property. Syntax is its value syntax as AGENTS.md
// writes it.
type Property struct {
	Name     string `json:"name"`
	Since    int    `json:"since"`
	Syntax   string `json:"syntax"`
	Keywords []Item `json:"keywords"`
}

// Media is one @media feature.
type Media struct {
	Name     string `json:"name"`
	Since    int    `json:"since"`
	Keywords []Item `json:"keywords"`
}

// Keys describes the key tokens of SPEC §8.1.
type Keys struct {
	Named      []string `json:"named"`
	Ctrl       string   `json:"ctrl"`
	Printable  bool     `json:"printable"`
	NeverMatch []string `json:"never_match"`
	// RejectedSince is the first language whose keymaps reject NeverMatch
	// with V003 (SPEC v0.4b §8.1, §15.10).
	RejectedSince int `json:"rejected_since"`
}

// Action is a built-in action. KeymapOnly marks the hyphenated ones, which
// are allowed in keymap rows and are V003 in an on:* attribute.
type Action struct {
	Name       string `json:"name"`
	Since      int    `json:"since"`
	Builtin    bool   `json:"builtin"`
	NeedsTo    bool   `json:"needs_to"`
	KeymapOnly bool   `json:"keymap_only"`
}

// Diagnostic is one row of the diagnostic table (SPEC §14).
type Diagnostic struct {
	Code string `json:"code"`
	Pass string `json:"pass"`
	When string `json:"when"`
}

// Catalog is the whole object.
type Catalog struct {
	Catalog       int          `json:"catalog"`
	Tuimark       string       `json:"tuimark"`
	Versions      []int        `json:"versions"`
	Current       int          `json:"current"`
	Tags          []Tag        `json:"tags"`
	Properties    []Property   `json:"properties"`
	PseudoClasses []Item       `json:"pseudo_classes"`
	Media         []Media      `json:"media"`
	Keys          Keys         `json:"keys"`
	Actions       []Action     `json:"actions"`
	Diagnostics   []Diagnostic `json:"diagnostics"`
}

// ClassGuardAttr is the name and pattern of the class:NAME family.
const (
	ClassGuardAttr    = "class:NAME"
	ClassGuardPattern = "class:[a-z_][a-z0-9_-]*"
)

// needsTo names the built-in actions whose row needs a `to` (V003
// otherwise, SPEC §8.4): focus names the node to focus, switch-to the tab
// or screen. A test checks it against the parser.
var needsTo = map[string]bool{"focus": true, "switch-to": true}

// Build returns the catalog of this build; tuimark is the bare runtime
// version (`tuimark version` without its metadata).
func Build(tuimark string) Catalog {
	return Catalog{
		Catalog:       Format,
		Tuimark:       tuimark,
		Versions:      append([]int(nil), Versions...),
		Current:       Current,
		Tags:          tags(),
		Properties:    properties(),
		PseudoClasses: pseudoClasses(),
		Media:         media(),
		Keys: Keys{
			Named:         append([]string(nil), ir.NamedKeys...),
			Ctrl:          "a-z",
			Printable:     true,
			NeverMatch:    append([]string(nil), agentsdoc.NeverMatchKeys...),
			RejectedSince: int(ir.KeysRejectedSince),
		},
		Actions:     actions(),
		Diagnostics: diagnostics(),
	}
}

func tags() []Tag {
	since := map[string]int{}
	for _, k := range ir.Kinds {
		since[k] = 1
	}
	for _, k := range ir.KindsV2 {
		since[k] = 2
	}
	for _, k := range ir.KindsCurrent {
		since[k] = Current
	}
	names := make([]string, 0, len(since))
	for k := range since {
		names = append(names, k)
	}
	sort.Strings(names)
	out := make([]Tag, 0, len(names))
	for _, name := range names {
		out = append(out, Tag{Name: name, Since: since[name], Attrs: attrs(name)})
	}
	return out
}

// attrs lists a tag's attributes alphabetically: those of the version="1"
// table, then what version="2", version="3", and the current language add,
// each with the first version whose table has it (Current for the current
// language's table, SPEC v0.4b §15.10). An attribute of a tag that is
// itself version="2" is at least 2 (the tag's rows live in the version="2"
// table).
func attrs(tag string) []Attr {
	since := map[string]int{}
	for name := range parse.TagAttrs[tag] {
		since[name] = 1
	}
	for name := range parse.TagAttrsV2[tag] {
		if _, ok := since[name]; !ok {
			since[name] = 2
		}
	}
	for name := range parse.TagAttrsV3[tag] {
		if _, ok := since[name]; !ok {
			since[name] = 3
		}
	}
	for name := range parse.TagAttrsCurrent[tag] {
		if _, ok := since[name]; !ok {
			since[name] = Current
		}
	}
	names := make([]string, 0, len(since)+1)
	for name := range since {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]Attr, 0, len(names)+1)
	for _, name := range names {
		out = append(out, Attr{Name: name, Since: since[name], Keywords: attrKeywords(tag, name, since[name])})
	}
	if parse.TakesClassGuard(tag) {
		// class:NAME sorts after "class" and before "disabled" ('c' < 'd'),
		// so it goes in order.
		out = append(out, Attr{Name: ClassGuardAttr, Since: 2, Pattern: ClassGuardPattern, Keywords: []Item{}})
		sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	}
	return out
}

// attrKeywords lists the attribute's keywords on the tag. The table says
// which version accepts a keyword wherever its attribute exists; on a tag
// that is itself newer (border on a table, say) the attribute's own since
// is the floor, so a keyword is never listed lower than its attribute.
func attrKeywords(tag, attr string, attrSince int) []Item {
	out := []Item{}
	for _, k := range parse.AttrKeywords(tag, attr) {
		out = append(out, Item{k.Name, max(k.Since, attrSince)})
	}
	return out
}

func keywords(list []css.Keyword) []Item {
	out := []Item{}
	for _, k := range list {
		out = append(out, Item{k.Name, k.Since})
	}
	return out
}

func properties() []Property {
	names := css.PropertyNames()
	sort.Strings(names)
	out := make([]Property, 0, len(names))
	for _, name := range names {
		out = append(out, Property{
			Name:     name,
			Since:    css.PropertySince(name),
			Syntax:   css.PropertyValues(name),
			Keywords: keywords(css.PropertyKeywords(name)),
		})
	}
	return out
}

func pseudoClasses() []Item {
	out := []Item{}
	for _, k := range css.PseudoClasses() {
		out = append(out, Item{k.Name, k.Since})
	}
	return out
}

func media() []Media {
	out := []Media{}
	for _, f := range css.MediaFeatures() {
		out = append(out, Media{Name: f.Name, Since: f.Since, Keywords: keywords(f.Keywords)})
	}
	return out
}

// actions lists the built-ins as AGENTS.md does: the version="1" ones
// alphabetically (focus, quit), then the hyphenated ones in the table's
// order.
func actions() []Action {
	var base []string
	for name := range host.Builtins {
		base = append(base, name)
	}
	sort.Strings(base)
	out := make([]Action, 0, len(base)+len(ir.BuiltinActionsV2))
	for _, name := range base {
		out = append(out, Action{Name: name, Since: 1, Builtin: true, NeedsTo: needsTo[name]})
	}
	for _, name := range ir.BuiltinActionsV2 {
		out = append(out, Action{Name: name, Since: 2, Builtin: true, NeedsTo: needsTo[name], KeymapOnly: true})
	}
	return out
}

func diagnostics() []Diagnostic {
	out := []Diagnostic{}
	for _, d := range agentsdoc.DiagCodes() {
		out = append(out, Diagnostic{d.Code, d.Pass, d.When})
	}
	return out
}
