package parse

import (
	"encoding/binary"
	"fmt"
	"hash/maphash"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/uniwidth"
)

// StyleRef is one <style> element: an external src or an inline body.
type StyleRef struct {
	Src       string
	Body      string
	Line, Col int // position of the element
	BodyLine  int // position of the inline body (for diagnostics)
	BodyCol   int
	// File is the display name of the file the <style> is written in: the
	// document's own name, or a fragment's (SPEC v0.4b §6.18.6).
	File string
	// From is the loader path of that file when it is a fragment (""
	// for the document): a relative src resolves against its directory
	// (SPEC v0.4b §6.18.2).
	From string
	// Org is where the <style> is written when an include placed it (nil
	// otherwise): its diagnostics carry the placement suffix.
	Org *ir.Origin
	// Rel is the stylesheet's path relative to the document's directory
	// when the <style> is written in a fragment ("" otherwise): the name
	// its diagnostics, `tuimark ir`, and `inspect` use for it, so that
	// two kits' theme.tcss stay apart (SPEC v0.4b §6.18.6).
	Rel string
}

// KeyBind is one <bind> row of the keymap.
type KeyBind struct {
	Keys    []string
	KeysRaw string
	Action  string
	// When is this row's own when=, as written ("" when absent).
	When string
	// HasOwnWhen is whether the row itself wrote a when= attribute, even
	// ""="" (SPEC v0.3b §8.1): it then overrides the enclosing <keymap>'s
	// own when instead of inheriting it.
	HasOwnWhen bool
	// KeymapNode is the enclosing <keymap> element, whose own when= this
	// row inherits when it wrote none itself (SPEC v0.3b §8.1).
	KeymapNode *ir.Node
	// EffectiveWhen is When when HasOwnWhen, else the enclosing <keymap>'s
	// own when= ("" when it has none either): what `tuimark ir`'s
	// top-level keymap array carries as this row's when (SPEC v0.3b §8.1,
	// §13.1).
	EffectiveWhen string
	// WhenSel is EffectiveWhen parsed, nil when it is empty or invalid
	// (checkKeymap): the selector dispatch, <hints>, and B007 use.
	WhenSel   *css.Selector
	To        string // id without '#'
	Label     string // version="2": the hint label (SPEC §6.12); "" when absent
	Keycap    string // version="2": the key text a hint shows instead of the key
	HasLabel  bool
	HasKeycap bool
	Line, Col int
	// Org is where the row is written when that is not the document
	// itself (SPEC v0.4b §6.18.6): nil for a row of the document.
	Org *ir.Origin
	// Node is the <bind> element of the row: what `tuimark inspect` names
	// as the source of the hint items the row gives (SPEC v0.4b §15.7).
	Node *ir.Node
}

// Document is a parsed and validated .tui file.
type Document struct {
	File    string
	Spike   bool
	Version string
	// V2 is true for a <tui version="2"> or <tui version="3"> document, and
	// for a <tui> without version (v0.4.1): it accepts the 0.2b vocabulary
	// (SPEC §5.1). Any other version is checked as "1".
	V2 bool
	// V3 is true for a <tui version="3"> document and a <tui> without
	// version (SPEC v0.3 §5.1, v0.4.1): the clipping warnings L008 and L009
	// fire, and the dump carries clipped. Every version="2" rule also holds
	// for it (V2 is true too).
	V3 bool
	// Lang is the language the document is checked and rendered in (SPEC
	// v0.4b §5.1): 1, 2, or 3 for a <tui> that writes that version (pinned
	// to its tuimark 0.4.1 meaning), ir.LangCurrent for a <tui> without
	// version, and 1 for the spike root <app>, an invalid version, and a
	// document that fails before its root is read. V2 and V3 are
	// Lang.V2() and Lang.V3().
	Lang  ir.Lang
	Theme string // the theme attribute: dark, light, auto (version="2"), or "" when absent
	// Mouse is the mouse attribute as written (version="2", SPEC §8.5).
	Mouse   string
	Root    *ir.Node   // <tui> or <app>
	Screens []*ir.Node // v1 only
	Styles  []StyleRef
	Keymap  []KeyBind
	IDs     map[string]*ir.Node
	Diags   ir.Diags

	Raw            *RawNode
	Prolog, Epilog []*RawNode

	// FragmentRoot is true when the file's root is <fragment>: not a
	// document (V005), but a file `tuimark validate` checks on its own
	// (FragmentDiags; SPEC v0.4b §6.18.2, §15.1).
	FragmentRoot bool
	// Fragments are the files the document's <include> elements name, at
	// any depth, in the order they were first resolved, each once (SPEC
	// v0.4b §6.18.2): what run and preview --watch watch (§15.9). A file
	// that could not be read is listed too, so creating it reloads, as a
	// missing <style src> does; one outside the file system, in a cycle,
	// or ending in .tcss is never read and is not.
	Fragments []FragmentRef
	// Composed is set when an <include> or a <use> was expanded: one
	// written element may then be placed more than once, and diagnostics
	// equal in severity, code, message, file, line, and column are one,
	// with the path and id of the first placement (SPEC v0.4b §6.18.6,
	// decision 16). Every pinned document leaves it false.
	Composed bool

	// dropped holds the subtrees rooted at an unknown tag: validated, kept
	// out of Root (see builder.tagKnown), in document order.
	dropped []*ir.Node
}

// FragmentRef is a file an <include> names: its path in the loader's file
// system and the name diagnostics give it.
type FragmentRef struct {
	Path string
	Name string
}

// InlineDecls returns the style="" declarations of every node of the
// document, whatever the data and whichever screen is active: other
// screens, closed modals, false if= branches, hidden nodes, list item
// templates (not expanded), and the subtrees of unknown tags. Each carries
// the file name and position the cascade reports for it at render time, so
// a static check over them (css.CheckTokens) and the render-time
// diagnostic for the same declaration are identical and dedupe.
func (d *Document) InlineDecls() []css.Decl {
	file := ""
	if d.File != "" {
		file = filepath.Base(d.File)
	}
	var out []css.Decl
	var walk func(n *ir.Node)
	walk = func(n *ir.Node) {
		for _, p := range n.Inline {
			out = append(out, css.Decl{Prop: p.Name, Value: p.Value, Line: p.Line, Col: p.Col, File: n.FileName(file)})
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	if d.Root != nil {
		walk(d.Root)
	}
	for _, n := range d.dropped {
		walk(n)
	}
	return out
}

// Events allowed as on:<name>.
var eventNames = map[string]bool{
	"click": true, "select": true, "submit": true, "change": true,
	"escape": true, "focus": true, "open": true, "close": true,
}

var commonAttrs = []string{
	"id", "class", "title", "hidden", "disabled", "focusable", "if", "style",
	"width", "height", "gap", "pad", "border", "on:click", "on:focus",
}

func with(extra ...string) map[string]bool {
	m := map[string]bool{}
	for _, a := range commonAttrs {
		m[a] = true
	}
	for _, a := range extra {
		m[a] = true
	}
	return m
}

func only(names ...string) map[string]bool {
	m := map[string]bool{}
	for _, a := range names {
		m[a] = true
	}
	return m
}

func without(m map[string]bool, names ...string) map[string]bool {
	for _, a := range names {
		delete(m, a)
	}
	return m
}

// itemFocusAttrs make a node a focus target or give it a click/focus
// handler. A list row never takes focus (its list does, and navigates the
// rows), so these are V002 on an <item> and anywhere inside one.
var itemFocusAttrs = map[string]bool{"focusable": true, "on:click": true, "on:focus": true}

// TagAttrs is the attribute catalog per tag of a version="1" document
// (V002 outside it). TagAttrsV2 adds what version="2" accepts.
var TagAttrs = map[string]map[string]bool{
	"tui":      only("version", "theme"),
	"style":    only("src"),
	"keymap":   only(),
	"bind":     only("keys", "action", "when", "to"),
	"screen":   only("id", "class", "focus", "title", "style"),
	"col":      with(),
	"row":      with(),
	"box":      with(),
	"item":     without(with(), "focusable", "on:click", "on:focus"),
	"spacer":   with(),
	"scroll":   with("axis"),
	"text":     with("wrap"),
	"rule":     with("axis"),
	"list":     with("each", "key", "bind", "on:select"),
	"input":    with("bind", "placeholder", "secret", "on:submit", "on:change"),
	"button":   with("label"),
	"progress": with("bind", "value"),
	"modal":    with("open", "bind", "on:escape", "on:open", "on:close"),
}

// TagAttrsV2 is what a version="2" document accepts on top of TagAttrs:
// the attributes marked † in SPEC §6.15, and the rows of the six new tags.
// Every tag except tui, style, keymap, and bind also takes class:NAME
// (noClassGuard), and a table's <item> takes only class and class:NAME.
var TagAttrsV2 = map[string]map[string]bool{
	"tui":       only("mouse"),
	"bind":      only("label", "keycap"),
	"col":       only("each", "key"),
	"row":       only("each", "key"),
	"box":       only("each", "key"),
	"list":      only("checked", "mark", "on:change"),
	"table":     with("each", "key", "bind", "checked", "mark", "placeholder", "on:select", "on:change"),
	"column":    only("id", "class", "title", "width", "style", "on:click"),
	"tabs":      with("bind", "mark", "on:select"),
	"tab":       only("id", "class", "title", "hidden", "disabled", "if", "style", "gap", "pad", "border", "label", "short", "focus"),
	"sparkline": with("bind", "min", "max"),
	"hints":     with("scope"),
}

// TagAttrsV3 is what a version="3" document additionally accepts on top of
// TagAttrs and TagAttrsV2: the attributes marked ‡ in SPEC §6.15 (v0.3b).
// scroll's stick, modal's focus, and keymap's when sit on version="1"
// tags; column's priority (§6.9.3) and sparkline's scale (§6.11) on
// version="2" tags.
var TagAttrsV3 = map[string]map[string]bool{
	"scroll":    only("stick"),
	"modal":     only("focus"),
	"keymap":    only("when"),
	"column":    only("priority"),
	"sparkline": only("scale"),
}

// TagAttrsCurrent is what only the current language, a <tui> without
// version, accepts on top of TagAttrs, TagAttrsV2, and TagAttrsV3 (SPEC
// v0.4b §5.1, §6.15): in a pinned document each is V002 with
// ir.VersionHintCurrent. Each 0.4b phase adds its attributes here, and
// the rows of its tags, when it implements them; none is declared ahead
// of its phase.
var TagAttrsCurrent = map[string]map[string]bool{
	"include":  only("src"),
	"fragment": only(),
	// SPEC v0.4b §6.18.3; a <use>'s attributes are decision 9's, and it
	// also takes the props its template declares (v0.4c §6.18.9, which
	// adds props).
	"template": only("id", "props"),
	"use":      only("id", "template", "with", "if", "hidden", "class"),
	"slot":     only(), // SPEC v0.4c §6.18.10: no attribute, no content (slot.go)

	"progress": only("indeterminate"), // SPEC v0.4b §6.4.1: the spinner
	"checkbox": checkboxAttrs(),       // SPEC v0.4b §6.16 (forms.go)
	"select":   selectAttrs(),         // SPEC v0.4b §6.17 (forms.go)
	"list":     only("single"),        // SPEC v0.4b §6.14.1
	"table":    only("single"),        // SPEC v0.4b §6.14.1
}

// tableItemAttrs are the attributes of a table's row template <item>
// (SPEC §6.9.1, §6.15), besides class:NAME.
var tableItemAttrs = only("class")

// noClassGuard are the tags that take no class:NAME (SPEC §6.13).
var noClassGuard = map[string]bool{"tui": true, "style": true, "keymap": true, "bind": true, "include": true, "fragment": true, "template": true, "use": true, "slot": true}

// ClassGuardPrefix starts every class:NAME attribute (SPEC §6.13).
const ClassGuardPrefix = "class:"

// attrAllowed reports whether a tag takes an attribute, and the lowest
// language that accepts it there: 1 (or 0, equivalently, for a
// version="1" tag's own attributes), 2, 3 (SPEC §6.15), or 4, the current
// language (SPEC v0.4b §5.1). parent is the element's parent tag.
func attrAllowed(tag, parent, name string) (ok bool, need int) {
	if strings.HasPrefix(name, ClassGuardPrefix) {
		return !noClassGuard[tag], 2
	}
	if tag == "item" && parent == "table" {
		return tableItemAttrs[name], 2
	}
	if ir.IsKindCurrent(tag) {
		return TagAttrsCurrent[tag][name], int(ir.Lang4)
	}
	if ir.IsKindV2(tag) {
		if TagAttrsV2[tag][name] {
			return true, 2
		}
		if TagAttrsV3[tag][name] {
			return true, 3
		}
		return TagAttrsCurrent[tag][name], int(ir.Lang4)
	}
	if TagAttrs[tag][name] {
		return true, 1
	}
	if TagAttrsV2[tag][name] {
		return true, 2
	}
	if TagAttrsV3[tag][name] {
		return true, 3
	}
	return TagAttrsCurrent[tag][name], int(ir.Lang4)
}

// Focusable-by-default kinds that therefore need an id (V012).
var needsID = map[string]bool{"list": true, "input": true, "button": true, "modal": true, "checkbox": true, "select": true}

// Kinds that may not contain element children.
var leafKinds = map[string]bool{"text": true, "input": true, "button": true, "progress": true, "rule": true, "spacer": true, "style": true, "bind": true}

type builder struct {
	doc  *Document
	file string
	// v2 is the document's version="2" or version="3" (SPEC §5.1), known
	// before any element is built: the root's version decides how every
	// tag, attribute, value, selector, and action is checked.
	v2 bool
	// v3 is the document's version="3" (SPEC v0.3 §5.1): the clipping
	// warnings L008 and L009 fire (internal/layout), and the dump carries
	// clipped. v2 is always true when v3 is.
	v3 bool
	// lang is the document's language (SPEC v0.4b §5.1), known with v2 and
	// v3 before any element is built; v2 and v3 are lang.V2() and
	// lang.V3().
	lang ir.Lang
	// droppedIn maps a node to the unknown-tag children content() kept
	// out of its Children, in document order, each with the number of
	// Children that precede it (walkAll merges by that index: a position
	// means nothing across the files an include reads).
	droppedIn map[*ir.Node][]droppedAt
	// addressable holds the ids of tree nodes outside any list <item>:
	// the ones screen focus= and keymap to=/when= can name.
	addressable map[string]bool
	// org is the origin of the elements being built: nil while the
	// document's own text is read, and the include that placed a fragment's
	// elements while one is (SPEC v0.4b §6.18.6). Every diagnostic made
	// while building an element names its file.
	org *ir.Origin
	// depth is the depth of the element being built in the tree being
	// built, the root's 1 (checkDepth bounds what an expansion nests).
	depth int
	// attrNames and attrCands cache what attrSuggestion looks up: the
	// names every tag takes, and the sorted attributes of each tag.
	// styleSeen holds the places of the <style> elements recorded in
	// doc.Styles (styleKey), so one placed again is not added again.
	styleSeen map[string]bool
	attrNames map[string]bool
	attrCands map[string][]string
	// comp holds the state of composition (<include>); nil in a document
	// that is not the current language.
	comp *composer
	// reported holds the diagnostics reported since an expansion made
	// dedupeDiags apply, by what decides their text (diagKey), each with
	// its arguments: a repeat, which dedupeDiags would drop at the end, is
	// dropped before its message is formatted (the F2 final check), so one
	// written element that many copies place costs one message. cutDiags
	// clears it wherever doc.Diags loses entries.
	reported map[diagKey][]any
	seed     maphash.Seed
	// at and mute are set while one attribute, or one body, that holds a
	// {$name} is checked (props.go): at names the use a diagnostic there
	// names in its placement suffix (an element of a copy), mute drops
	// every diagnostic there (outside a template, after its V003).
	at   propAt
	mute propAt
	// pbody is what the element being built holds in its body, when its
	// written body held a {$name} (node, content).
	pbody *pattr
}

// diagKey is what decides a diagnostic's identity for dedupeDiags before
// its message is formatted: with equal arguments, two equal keys give
// equal severity, code, message, file, line, and column. placed is the
// element whose placement suffix the message carries, nil for none; args
// is the hash of the arguments (argsHash).
type diagKey struct {
	sev, code, file string
	line, col       int
	placed          *ir.Node
	format          string
	args            uint64
}

// argsHash hashes a diagnostic's arguments, each a string or an int,
// copying nothing; ok is false for any other argument (its diagnostic is
// formatted, and left to dedupeDiags).
func argsHash(seed maphash.Seed, args []any) (uint64, bool) {
	var h maphash.Hash
	h.SetSeed(seed)
	var buf [9]byte
	for _, a := range args {
		switch v := a.(type) {
		case string:
			binary.LittleEndian.PutUint64(buf[1:], uint64(len(v)))
			buf[0] = 's'
			h.Write(buf[:])
			h.WriteString(v)
		case int:
			binary.LittleEndian.PutUint64(buf[1:], uint64(v))
			buf[0] = 'i'
			h.Write(buf[:])
		default:
			return 0, false
		}
	}
	return h.Sum64(), true
}

// earlyDedupe turns the dedupe before formatting on (builder.reported); a
// test turns it off to check that it keeps what dedupeDiags keeps.
var earlyDedupe = true

// sameArgs reports whether two argument lists argsHash took are equal.
func sameArgs(a, b []any) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// cutDiags drops the diagnostics from index n on (those of an expansion
// that is discarded, or set aside), and forgets what reported held.
func (b *builder) cutDiags(n int) {
	b.doc.Diags = b.doc.Diags[:n]
	b.reported = nil
}

// droppedAt is an unknown-tag child content() kept out of its parent's
// Children, with the number of Children before it. A ghost entry is a
// <template> or a <use> instead (expand.go): never a node, but where it
// stands is its place in the id space.
type droppedAt struct {
	at    int
	node  *ir.Node
	ghost ghostKind
}

// listItem returns the list <item> that n is or is inside (the nearest
// <item> whose parent is a <list>), or nil. Such a node is inflated as part
// of a list row: it is never focused or addressed by id at run time, the
// list is (host.collectFocusables, host inflate).
func listItem(n *ir.Node) *ir.Node {
	for p := n; p != nil; p = p.Parent {
		if p.Tag == "item" && p.Parent != nil && p.Parent.Tag == "list" {
			return p
		}
	}
	return nil
}

// v2Hint reports a version="2" item in a version="1" document: the
// message ends with ` (requires version="2")` (SPEC §5.1).
func (b *builder) v2Hint(line, col int, n *ir.Node, code, format string, args ...any) {
	b.diag(ir.Error, code, line, col, n.Path, n.ID, format+"%s", append(args, ir.VersionHint)...)
}

// v3Hint reports a version="3" item in a version="1" or version="2"
// document: the message ends with ` (requires version="3")` (SPEC v0.3b
// §5.1).
func (b *builder) v3Hint(line, col int, n *ir.Node, code, format string, args ...any) {
	b.diag(ir.Error, code, line, col, n.Path, n.ID, format+"%s", append(args, ir.VersionHintV3)...)
}

// currentHint reports an item only the current language accepts in a
// document that writes version="1", "2", or "3": the message ends with
// ` (requires a <tui> without version)` (SPEC v0.4b §5.1).
func (b *builder) currentHint(line, col int, n *ir.Node, code, format string, args ...any) {
	b.diag(ir.Error, code, line, col, n.Path, n.ID, format+"%s", append(args, ir.VersionHintCurrent)...)
}

// listHint is the advice given for a focus target inside a list row.
func listHint(item *ir.Node) string {
	return listHintAt(nil, item)
}

// listHintAt is listHint in a diagnostic about the node at: the list's id
// as ir.ShownIDAt shows it there.
func listHintAt(at, item *ir.Node) string {
	sel := "list"
	if item != nil && item.Parent != nil && item.Parent.ID != "" {
		sel = "#" + ir.ShownIDAt(at, item.Parent.ID)
	}
	return fmt.Sprintf(`list rows are navigated by their list; handle the row with the list's on:select, or a keymap <bind keys="..." action="..." when="%s:focus"/> (its event identifies the selected row)`, sel)
}

// diag reports a diagnostic in the file being built (b.org).
func (b *builder) diag(sev, code string, line, col int, path, id, format string, args ...any) {
	b.diagOrg(b.org, false, sev, code, line, col, path, id, format, args...)
}

// diagOrg reports a diagnostic in the file org says (the document's when
// nil). place appends the placement suffix of an element an include placed
// (SPEC v0.4b §6.18.6): a diagnostic that depends on where the element
// stands gets it, a self-check (what the element's own attributes and text
// say) does not, since it holds wherever the element is placed.
func (b *builder) diagOrg(org *ir.Origin, place bool, sev, code string, line, col int, path, id, format string, args ...any) {
	var placed *ir.Node
	if org != nil && place {
		placed = org.Placed
	}
	b.diagBy(org, placed, sev, code, line, col, path, id, format, args...)
}

// diagBy is diagOrg with the include or use the placement suffix names,
// placed (nil for none). A diagnostic at the attribute or the body of an
// element of a copy that held a {$name} names the use b.at says, whatever
// its code (SPEC v0.4c §6.18.6, decision 194), and one about a value
// outside a template that holds a {$name} is muted (b.mute: the V003 of
// §6.18.9 is reported instead of any other about that value).
func (b *builder) diagBy(org *ir.Origin, placed *ir.Node, sev, code string, line, col int, path, id, format string, args ...any) {
	file := b.file
	if org != nil {
		file = org.File
	}
	if b.mute.on && line == b.mute.line && col == b.mute.col && file == b.mute.file {
		return
	}
	if b.at.on && line == b.at.line && col == b.at.col && file == b.at.file {
		placed = b.at.by
	}
	if b.comp != nil && b.comp.used && earlyDedupe {
		// dedupeDiags applies (ParseWith): a repeat is dropped unformatted.
		if b.reported == nil {
			b.reported = map[diagKey][]any{}
			b.seed = maphash.MakeSeed()
		}
		if h, ok := argsHash(b.seed, args); ok {
			k := diagKey{sev, code, file, line, col, placed, format, h}
			if first, seen := b.reported[k]; seen && sameArgs(first, args) {
				return
			} else if !seen {
				b.reported[k] = args
			}
		}
	}
	var whole string
	if placed != nil && org.InCopy() {
		// One per copy, each told apart by its placement suffix: what it
		// quotes with %q is cut, as the runtime's are (ir.At), and kept
		// whole beside it, so the repeat dropped above, and the dedupes
		// after (dedupeDiags, the host's), compare it whole. A self-check
		// (no suffix) is the same in every copy and in the template's own
		// body, and is reported once, whole.
		args, whole = ir.CutArgs(format, args)
	}
	msg := fmt.Sprintf(format, ir.UnmarkArgs(args)...)
	if placed != nil {
		msg += ir.SuffixOf(placed)
	}
	b.doc.Diags = append(b.doc.Diags, ir.WithWhole(ir.Diagnostic{
		Severity: sev, Code: code, Msg: msg,
		File: file, Line: line, Col: col, Path: path, ID: id,
	}, whole))
}

// diagPlaced is diag for a check that depends on where the element stands:
// it carries the placement suffix of an element an include placed.
func (b *builder) diagPlaced(sev, code string, line, col int, path, id, format string, args ...any) {
	b.diagOrg(b.org, true, sev, code, line, col, path, id, format, args...)
}

// selfCheck are the codes an element's own attributes and text give: they
// hold wherever the element is placed, so they carry no placement suffix
// (SPEC v0.4b §6.18.6). Every other code about an element depends on its
// place, its ids, or the data.
var selfCheck = map[string]bool{"V002": true, "V003": true, "V007": true, "V013": true, "V015": true, "V018": true}

// errN reports an error about n, in the file n is written in.
func (b *builder) errN(n *ir.Node, code, format string, args ...any) {
	b.diagBy(n.Org, nodePlacer(n, code), ir.Error, code, n.Line, n.Col, n.Path, n.ID, format, args...)
}

// nodePlacer is the include or use that the placement suffix of a
// diagnostic with code at n names, nil for none: n's innermost placer for
// a check that depends on where n stands, none for a self-check, and, at
// an element of a copy whose attributes or body held a {$name}, the use
// that n.PropBy names, whatever the code (SPEC v0.4c §6.18.6).
func nodePlacer(n *ir.Node, code string) *ir.Node {
	if n.PropBy != nil {
		return n.PropBy
	}
	if selfCheck[code] || n.Org == nil {
		return nil
	}
	return n.Org.Placed
}

// errSelf reports an error about n that holds wherever n is placed (an
// unknown tag): no placement suffix.
func (b *builder) errSelf(n *ir.Node, code, format string, args ...any) {
	b.diagOrg(n.Org, false, ir.Error, code, n.Line, n.Col, n.Path, n.ID, format, args...)
}

// errKB reports an error about a keymap row, in the file the row is
// written in.
func (b *builder) errKB(k *KeyBind, code, path, format string, args ...any) {
	b.diagOrg(k.Org, !selfCheck[code], ir.Error, code, k.Line, k.Col, path, "", format, args...)
}

func (b *builder) warnN(n *ir.Node, code, format string, args ...any) {
	b.diagBy(n.Org, nodePlacer(n, code), ir.Warning, code, n.Line, n.Col, n.Path, n.ID, format, args...)
}

// Parse parses and validates a .tui document. file is used in diagnostics
// and to resolve relative style src paths (it may be empty). It reads no
// file: an <include> of the current language is V006 "cannot be read"
// (ParseWith gives it a file system).
func Parse(src []byte, file string) *Document {
	return ParseWith(src, file, nil)
}

// ParseWith is Parse for a document that may include other files (SPEC
// v0.4b §6.18.2): the loader resolves and reads the files its <include>
// elements name, and the document is expanded while it is built. A nil
// loader reads nothing.
func ParseWith(src []byte, file string, ld *Loader) *Document {
	doc := &Document{File: file, IDs: map[string]*ir.Node{}, Lang: ir.Lang1}
	b := &builder{doc: doc, file: filepath.Base(file), lang: ir.Lang1}
	if file == "" {
		b.file = ""
	}
	raw, prolog, epilog, err := ParseXML(src)
	if err != nil {
		b.diag(ir.Error, "V005", err.Line, err.Col, "", "", "%s", err.Msg)
		return doc
	}
	doc.Raw, doc.Prolog, doc.Epilog = raw, prolog, epilog
	switch raw.Name {
	case "app":
		doc.Spike = true
	case "fragment":
		// SPEC v0.4b §6.18.2: a fragment is not a document. The tools that
		// check a fragment on their own read doc.Raw (FragmentDiags).
		doc.FragmentRoot = true
		b.diag(ir.Error, "V005", raw.Line, raw.Col, "/", "", "a <fragment> is not a document: check it through the document that includes it")
		return doc
	case "tui":
		// The version decides every check below (SPEC §5.1), whatever the
		// attribute order: <tui theme="auto" version="2"> is valid. A
		// version="3" document accepts everything version="2" does (v0.3):
		// it sets v2 too, plus its own v3 flag. Without the attribute the
		// document is the current language (SPEC v0.4b §5.1), which holds
		// every version="3" rule plus what 0.4b adds; a written version
		// pins the document to its tuimark 0.4.1 meaning, and a value
		// other than 1, 2, or 3 (version="4" included) is V003 and read as
		// version="1".
		b.lang = ir.LangCurrent
		for _, a := range raw.Attrs {
			if a.Name != "version" {
				continue
			}
			switch a.Value {
			case "2":
				b.lang = ir.Lang2
			case "3":
				b.lang = ir.Lang3
			default:
				b.lang = ir.Lang1
			}
		}
		b.v2, b.v3 = b.lang.V2(), b.lang.V3()
		doc.V2, doc.V3, doc.Lang = b.v2, b.v3, b.lang
		if b.lang.V4() {
			// Only the current language has composition; a pinned document
			// never reads the file an <include> names (decision 23).
			self := ""
			if ld != nil {
				self = ld.Self
			}
			selfPath := ""
			if ld != nil {
				selfPath = ld.SelfPath
			}
			b.comp = &composer{ld: ld, chain: []chainFile{{canon: self, name: b.docName(), real: selfPath}}}
			b.comp.charge = b.chargeBytes
		}
	default:
		b.diag(ir.Error, "V005", raw.Line, raw.Col, "/", "", "document root must be <tui> (or the phase-0 alias <app>), got <%s>", raw.Name)
		return doc
	}
	doc.Root = b.node(raw, nil)
	b.expandUses() // SPEC v0.4b §6.18.1 steps 2 and 3 (expand.go)
	b.checkStructure()
	b.checkIDs(doc.Root)
	b.checkKeymap()
	if b.comp != nil && b.comp.used {
		// One written element placed more than once reports once (SPEC
		// v0.4b §6.18.6), at render time too (Composed).
		doc.Diags = dedupeDiags(doc.Diags)
		doc.Composed = true
	}
	sort.SliceStable(doc.Diags, func(i, j int) bool {
		if doc.Diags[i].Line != doc.Diags[j].Line {
			return doc.Diags[i].Line < doc.Diags[j].Line
		}
		return doc.Diags[i].Col < doc.Diags[j].Col
	})
	return doc
}

func childPath(parent *ir.Node, tag string) string {
	if parent == nil {
		return "/"
	}
	if parent.Path == "/" {
		return "/" + tag
	}
	return parent.Path + "/" + tag
}

// node converts a raw element (recursively) into an IR node.
func (b *builder) node(r *RawNode, parent *ir.Node) *ir.Node {
	b.countElement()
	b.depth++
	b.checkDepth(b.depth)
	n := &ir.Node{
		Tag: r.Name, Kind: r.Name, Parent: parent,
		Attrs: map[string]string{}, On: map[string]string{},
		Line: r.Line, Col: r.Col, Path: childPath(parent, r.Name),
		Org: b.org,
	}
	if r.Name == "app" {
		n.Kind = "col"
	}
	// The {$name} of its attributes and body (SPEC v0.4c §6.18.9,
	// props.go): nil when none holds one, the case of every element of a
	// document that does not use props.
	var pn *propNode
	if b.comp != nil {
		pn = b.propNode(n, r)
	}
	for i, a := range r.Attrs {
		if pn != nil && !pn.inAttrs(n, i, a) {
			continue
		}
		n.Attrs[a.Name] = a.Value
		n.Order = append(n.Order, a.Name)
	}
	b.chargePlaced()
	if id, ok := n.Attrs["id"]; ok {
		n.ID = id
		if b.comp != nil && b.comp.prefix != "" {
			n.ID = b.comp.prefix + id // scoped by its use (SPEC v0.4b §6.18.4)
		}
	}
	b.checkTag(n, parent)
	for i, a := range r.Attrs {
		if pn != nil {
			var ok bool
			if a, ok = pn.check(b, i, a); !ok {
				continue
			}
		}
		b.attr(n, a)
		b.at.on, b.mute.on = false, false
	}
	b.pbody = nil
	if pn != nil && pn.hasBody {
		b.pbody = &pn.body
	}
	if b.comp != nil && len(b.comp.copies) > 0 {
		b.copyContent(n, r) // an element of a copy (expand.go)
	} else {
		b.content(n, r)
	}
	if pn != nil {
		pn.restore(b, n, r)
	}
	b.depth--
	return n
}

// htmlTagHints are common HTML/frontend names that are not in the
// vocabulary, each with the Tuimark way to say it. An unknown tag found
// here gets the hint appended to its V001 instead of a did-you-mean.
var htmlTagHints = map[string]string{
	"div":       "use <box>, <col>, or <row>",
	"span":      "use <text>",
	"p":         "use <text>",
	"label":     "use <text>",
	"h1":        "use <text>",
	"h2":        "use <text>",
	"h3":        "use <text>",
	"h4":        "use <text>",
	"h5":        "use <text>",
	"h6":        "use <text>",
	"ul":        "use <list>",
	"ol":        "use <list>",
	"li":        "use <item> inside a <list>",
	"dialog":    "use <modal>",
	"option":    "a <list> with on:select picks one row",
	"radio":     "a <list> with on:select picks one row",
	"textarea":  "not in the vocabulary yet; use <input>",
	"component": "Tuimark has no components or includes; only <style src> loads another file",
	"import":    "Tuimark has no components or includes; only <style src> loads another file",
	"img":       "images are not supported",
	"image":     "images are not supported",
}

// didYouMean formats the "; did you mean X?" suffix of a V001/V002/V003
// message, or "" when there is no suggestion. shown is how X is written
// (a tag in angle brackets, a name in quotes); best is Suggest's result.
func didYouMean(shown, best string) string {
	if best == "" {
		return ""
	}
	return "; did you mean " + shown + "?"
}

// spikeTagSuggestion is tagSuggestion for an <app> document: the closest of
// the five phase-0 tags.
func spikeTagSuggestion(tag string) string {
	best := ir.Suggest(tag, ir.SpikeKinds)
	return didYouMean("<"+best+">", best)
}

// tagSuggestion is the suffix of an unknown tag's V001: a fixed hint for a
// common HTML name, else the closest tag of the vocabulary (a
// version="2" tag in a version="1" document says what it requires). The
// current-language tags are candidates only in the current language: a
// pinned document keeps 0.4.1's suggestion, or its lack of one, for a
// name it misspells (SPEC v0.4b §5.1).
func (b *builder) tagSuggestion(tag string) string {
	if h, ok := b.htmlHint(tag); ok {
		return "; " + h
	}
	cands := make([]string, 0, len(ir.Kinds)+len(ir.KindsV2)+len(ir.KindsCurrent))
	for _, k := range ir.Kinds {
		if k != "tui" { // only valid as the root
			cands = append(cands, k)
		}
	}
	cands = append(cands, ir.KindsV2...)
	if b.lang.V4() {
		cands = append(cands, ir.KindsCurrent...)
	}
	best := ir.Suggest(tag, cands)
	if best == "" {
		return ""
	}
	if ir.IsKindV2(best) && !b.v2 {
		return "; did you mean <" + best + "> (requires version=\"2\")?"
	}
	return didYouMean("<"+best+">", best)
}

// attrExists reports whether any tag of any version takes an attribute
// called name. The current-language attributes count only in the current
// language, so a pinned document keeps 0.4.1's suggestion for a name a
// 0.4b attribute shares (SPEC v0.4b §5.1).
func attrExists(name string, l ir.Lang) bool {
	return attrNameSet(l)[name]
}

// attrNameSet is every attribute name any tag takes in language l: one set
// to look a name up in, built from the tables each call (they are tables
// tests replace), so a caller that asks many times keeps the result.
func attrNameSet(l ir.Lang) map[string]bool {
	cats := []map[string]map[string]bool{TagAttrs, TagAttrsV2, TagAttrsV3}
	if l.V4() {
		cats = append(cats, TagAttrsCurrent)
	}
	out := map[string]bool{}
	for _, cat := range cats {
		for _, m := range cat {
			for a, ok := range m {
				if ok {
					out[a] = true
				}
			}
		}
	}
	for a, ok := range tableItemAttrs {
		if ok {
			out[a] = true
		}
	}
	return out
}

// attrSuggestion is the suffix of an unknown attribute's V002: the closest
// attribute the tag takes in this document's version, searched in sorted
// order for a deterministic tie-break.
func (b *builder) attrSuggestion(tag, parent, name string) string {
	// An attribute some other tag (or version) takes is misplaced, not
	// misspelled: pointing it at a lookalike (bind -> id) would mislead.
	if b.attrNames == nil {
		b.attrNames = attrNameSet(b.lang)
	}
	if b.attrNames[name] {
		return ""
	}
	key := tag
	if tag == "item" && parent == "table" {
		key = "item|table"
	}
	cands, ok := b.attrCands[key]
	if !ok {
		cands = b.attrCandidates(tag, parent)
		if b.attrCands == nil {
			b.attrCands = map[string][]string{}
		}
		b.attrCands[key] = cands
	}
	best := ir.Suggest(name, cands)
	return didYouMean(fmt.Sprintf("%q", best), best)
}

// attrCandidates are the attributes the tag takes in this document's
// version, sorted, for attrSuggestion (kept per tag: a document with many
// unknown attributes asks again and again).
func (b *builder) attrCandidates(tag, parent string) []string {
	var set []map[string]bool
	if tag == "item" && parent == "table" {
		set = append(set, tableItemAttrs)
	} else {
		set = append(set, TagAttrs[tag])
		if b.v2 {
			set = append(set, TagAttrsV2[tag])
		}
		if b.v3 {
			set = append(set, TagAttrsV3[tag])
		}
		if b.lang.V4() {
			set = append(set, TagAttrsCurrent[tag])
		}
	}
	seen := map[string]bool{}
	var cands []string
	for _, m := range set {
		for a := range m {
			if !seen[a] {
				seen[a] = true
				cands = append(cands, a)
			}
		}
	}
	sort.Strings(cands)
	return cands
}

func hasUpper(s string) bool {
	for _, c := range s {
		if unicode.IsUpper(c) {
			return true
		}
	}
	return false
}

func (b *builder) checkTag(n *ir.Node, parent *ir.Node) {
	tag := n.Tag
	if hasUpper(tag) {
		b.errSelf(n, "V001", "unknown tag <%s> (tags are lowercase)", tag)
		return
	}
	if strings.Contains(tag, ":") {
		b.errSelf(n, "V001", "unknown tag <%s> (namespaces are not allowed)", tag)
		return
	}
	if b.doc.Spike {
		ok := false
		for _, k := range ir.SpikeKinds {
			if k == tag {
				ok = true
			}
		}
		if !ok {
			if ir.IsKind(tag) {
				b.errSelf(n, "V001", "<%s> is not in the phase-0 vocabulary (app col row box text); use a <tui> root for v1 tags", tag)
			} else {
				b.errSelf(n, "V001", "unknown tag <%s>%s", tag, spikeTagSuggestion(tag))
			}
			return
		}
		if tag == "app" && parent != nil {
			b.errSelf(n, "V001", "<app> is only valid as the document root")
		}
		return
	}
	if tag == "app" {
		b.errSelf(n, "V001", "<app> is the phase-0 root alias; inside <tui> use <screen>")
		return
	}
	if ir.IsKindV2(tag) && !b.v2 {
		b.v2Hint(n.Line, n.Col, n, "V001", "tag <%s>", tag)
		return
	}
	if ir.IsKindCurrent(tag) && !b.lang.V4() {
		// SPEC v0.4b §5.1: a 0.4b tag in a pinned document is checked no
		// further, and has no effect.
		b.currentHint(n.Line, n.Col, n, "V001", "tag <%s>", tag)
		return
	}
	if tag == "fragment" {
		b.errSelf(n, "V001", "<fragment> is only valid as the root of an included file")
		return
	}
	if !ir.IsKindLang(tag, b.lang) {
		b.errSelf(n, "V001", "unknown tag <%s>%s", tag, b.tagSuggestion(tag))
		return
	}
	if b.comp != nil && (b.comp.selfOnly || b.comp.selfCheck) {
		// A fragment checked on its own, or a template's body as written:
		// where an element stands is not known, so only the checks an
		// element gets from itself (SPEC v0.4b §6.18.3, "An unused
		// template").
		return
	}
	ptag := ""
	if parent != nil {
		ptag = parent.Tag
	}
	if b.v2 && ptag == "tabs" && tag != "tab" {
		// SPEC §6.10.1, V016: a tabs holds tab elements only.
		b.errN(n, "V016", "<tabs> children must be <tab> elements, got <%s>", tag)
		return
	}
	switch tag {
	case "tab":
		// SPEC §6.10.1, V016 (only a version="2" document gets here).
		if ptag != "tabs" {
			b.errN(n, "V016", "<tab> must be a direct child of <tabs>")
		}
	case "tui":
		if parent != nil {
			b.errN(n, "V001", "<tui> is only valid as the document root")
		}
	case "style", "keymap", "screen":
		if ptag != "tui" {
			b.errN(n, "V001", "<%s> must be a direct child of <tui>", tag)
		}
	case "bind":
		if ptag != "keymap" {
			b.errN(n, "V001", "<bind> must be inside <keymap>")
		}
	case "item":
		if ptag != "list" && !(b.v2 && ptag == "table") {
			if b.v2 {
				b.errN(n, "V001", "<item> must be a direct child of <list> or <table>")
			} else {
				b.errN(n, "V001", "<item> must be a direct child of <list>")
			}
		}
	case "column":
		// SPEC §6.9.1, V016 (only a version="2" document gets here).
		if ptag != "table" {
			b.errN(n, "V016", "<column> must be a direct child of <table>")
		}
	case "modal":
		if ptag != "screen" {
			b.errN(n, "L004", "<modal> must be a direct child of <screen> (and come last)")
		}
	default:
		if ptag == "tui" {
			b.errN(n, "V001", "<%s> must be inside a <screen>", tag)
		}
		if ptag == "keymap" {
			b.errN(n, "V001", "<keymap> children must be <bind>, got <%s>", tag)
		}
		if ptag == "list" {
			b.errN(n, "V001", "<list> children must be <item>, got <%s>", tag)
		}
		if ptag == "table" && b.v2 {
			b.errN(n, "V016", "<table> children must be <column> elements and at most one <item> (the row template), got <%s>", tag)
		}
	}
	// SPEC §8.3 makes these focusable by default, but inside a list row
	// nothing is ever focused: the list is, and navigates its rows. Reject
	// them rather than render a widget that can never respond.
	if needsID[tag] || statefulV2[tag] {
		if it := listItem(parent); it != nil {
			b.errN(n, "V001", "<%s> inside a list <item> can never take focus: %s", tag, listHintAt(n, it))
		} else if c := b.eachContainer(n); c != nil {
			// SPEC §6.8 item 8: the template is repeated per element, so
			// nothing in it may take focus or keep per-node state.
			b.errN(n, "V001", "<%s> inside an each template (%s) would repeat for every element and can never take focus or keep its state: %s", tag, describeNodeAt(n, c), templateHint)
		}
	}
}

// templateHint is the advice given for a focus target or an id inside a
// container each template (SPEC §6.8 item 8).
const templateHint = `an each template on <col>, <row>, or <box> holds only what is shown per element; put a widget that takes focus outside it, or use a <list> (its rows are navigated by the list)`

// eachContainer returns the col, row, or box with an each= attribute
// whose template n is part of, at any depth (n itself excluded), or nil.
// Only a version="2" document has container templates (SPEC §6.8); in a
// version="1" document each= on a container is V002 and nothing is a
// template.
func (b *builder) eachContainer(n *ir.Node) *ir.Node {
	if !b.v2 || n == nil {
		return nil
	}
	for p := n.Parent; p != nil; p = p.Parent {
		if isEachContainer(p) {
			return p
		}
	}
	return nil
}

// isEachContainer reports whether n is a col, row, or box with an each=
// attribute (valid or not: a malformed each still makes its children a
// template for the checks).
func isEachContainer(n *ir.Node) bool {
	switch n.Tag {
	case "col", "row", "box":
		_, ok := n.Attrs["each"]
		return ok
	}
	return false
}

// describeNode names a node in a diagnostic: <tag id="..."> or <tag> at
// line:col.
func describeNode(n *ir.Node) string {
	return describeNodeAt(nil, n)
}

// describeNodeAt names n in a diagnostic about the node at: n's id as
// ir.ShownIDAt shows it there, so an element of a copy, reported once per
// copy, quotes a long id of an enclosing element shortened (the F2 final
// check), and every other element keeps describeNode's 0.4.1 text.
func describeNodeAt(at, n *ir.Node) string {
	if n.ID != "" {
		return fmt.Sprintf("<%s id=%q>", n.Tag, ir.ShownIDAt(at, n.ID))
	}
	return fmt.Sprintf("<%s> at %d:%d", n.Tag, n.Line, n.Col)
}

// statefulV2 are the version="2" tags that keep per-node state by id and
// so cannot sit inside a list <item> either (SPEC §6.8 item 8).
var statefulV2 = map[string]bool{"table": true, "tabs": true, "tab": true}

// tagKnown reports whether tag is in the closed vocabulary at all (SPEC §6,
// §6.7), independent of whether it appears in a valid context: false for
// uppercase or namespaced names, for spike-mode tags outside
// ir.SpikeKinds, and for v1 tags outside ir.Kinds (which also excludes the
// phase-0 <app> alias from a v1 document). checkTag reports V001 for these
// plus, separately, for tags that ARE known but misplaced (a <col> that is
// a direct child of <tui>, an <item> outside <list>, ...); those stay
// known, since their kind is still a valid one to lay out. content() uses
// this to keep a truly unknown kind out of the IR tree (SPEC §1, §13.1:
// "unknown kind fails before layout"), even though its own subtree is
// still built and validated so its diagnostics are reported.
func (b *builder) tagKnown(tag string) bool {
	if hasUpper(tag) || strings.Contains(tag, ":") {
		return false
	}
	if tag == "fragment" {
		// Only ever the root of an included file (SPEC v0.4b §6.18.2):
		// never a node, where it stands it is V001 and dropped.
		return false
	}
	if b.doc.Spike {
		for _, k := range ir.SpikeKinds {
			if k == tag {
				return true
			}
		}
		return false
	}
	return ir.IsKindLang(tag, b.lang)
}

func (b *builder) attrErr(n *ir.Node, a RawAttr, code, format string, args ...any) {
	b.diag(ir.Error, code, a.Line, a.Col, n.Path, n.ID, format, args...)
}

// sanitize strips control characters and ANSI escapes, reporting V007.
func (b *builder) sanitize(n *ir.Node, line, col int, s string) string {
	clean, bad := StripControl(s, true)
	if bad {
		b.diag(ir.Error, "V007", line, col, n.Path, n.ID, "control character or ANSI escape in text (stripped)")
	}
	return clean
}

// StripControl removes C0/C1 control characters (keeping \n when keepNL)
// and whole ANSI escape sequences. It reports whether anything was removed.
func StripControl(s string, keepNL bool) (string, bool) {
	var out strings.Builder
	bad := false
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == 0x1b {
			bad = true
			i += size
			// CSI: ESC [ params final ; OSC: ESC ] ... BEL/ST ; else one char.
			if i < len(s) && s[i] == '[' {
				i++
				for i < len(s) && !(s[i] >= 0x40 && s[i] <= 0x7e) {
					i++
				}
				if i < len(s) {
					i++
				}
			} else if i < len(s) && s[i] == ']' {
				for i < len(s) && s[i] != 0x07 && !(s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\') {
					i++
				}
				if i < len(s) && s[i] == 0x07 {
					i++
				} else if i < len(s) {
					i += 2
				}
			} else if i < len(s) {
				_, sz := utf8.DecodeRuneInString(s[i:])
				i += sz
			}
			continue
		}
		if (r < 0x20 && !(keepNL && r == '\n')) || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			bad = true
			i += size
			continue
		}
		out.WriteRune(r)
		i += size
	}
	return out.String(), bad
}

// attrGate reports whether n takes the attribute a, where it stands
// (SPEC §6.15): when it does not, the attribute is V002, reported when
// report is set, and is checked no further, its value included (SPEC
// v0.4c §6.18.9, decision 157: props.go asks without reporting).
func (b *builder) attrGate(n *ir.Node, a RawAttr, report bool) bool {
	name := a.Name
	tag := n.Tag
	err := func(format string, args ...any) bool {
		if report {
			b.attrErr(n, a, "V002", format, args...)
		}
		return false
	}
	if b.doc.Spike {
		ok := false
		for _, s := range ir.SpikeAttrs {
			if s == name {
				ok = true
			}
		}
		if !ok {
			return err("unknown attribute %q on <%s> (phase-0 attributes: id class width height gap pad border)", name, tag)
		}
	} else if b.tagKnown(tag) {
		ptag := ""
		if n.Parent != nil && (b.v2 || n.Parent.Tag != "table") {
			ptag = n.Parent.Tag
		}
		allowed, need := attrAllowed(tag, ptag, name)
		if itemFocusAttrs[name] {
			if it := listItem(n); it == n {
				return err("attribute %q is not allowed on a list <item>: %s", name, listHintAt(n, it))
			} else if it != nil && !needsID[tag] {
				// (An input/button/list/modal there is already V001 as a
				// whole in checkTag; its own attributes add nothing.)
				return err("attribute %q is not allowed inside a list <item> (on <%s>): %s", name, tag, listHintAt(n, it))
			} else if c := b.eachContainer(n); it == nil && c != nil && !needsID[tag] && !statefulV2[tag] {
				// SPEC §6.8 item 8, at any depth of a container template.
				return err("attribute %q is not allowed inside an each template (on <%s>, in %s): %s", name, tag, describeNodeAt(n, c), templateHint)
			}
		}
		if !allowed {
			if !report {
				return false
			}
			if strings.HasPrefix(name, "on:") && !eventNames[strings.TrimPrefix(name, "on:")] {
				return err("unknown event %q on <%s>%s", name, tag, b.attrSuggestion(tag, ptag, name))
			}
			return err("attribute %q is not allowed on <%s>%s", name, tag, b.attrSuggestion(tag, ptag, name))
		}
		// The newest gate first: a current-language item gets its own
		// hint in every pinned document (SPEC v0.4b §5.1); a version="3"
		// item is checked against version="3" next, whether the document
		// is version="1" or version="2" (both get the same hint); only an
		// attribute that needs exactly version="2" falls to the version="2"
		// hint (SPEC v0.3b §5.1).
		if need >= int(ir.Lang4) && !b.lang.V4() {
			if report {
				b.currentHint(a.Line, a.Col, n, "V002", "attribute %q on <%s>", name, tag)
			}
			return false
		}
		if need >= 3 && !b.v3 {
			if report {
				b.v3Hint(a.Line, a.Col, n, "V002", "attribute %q on <%s>", name, tag)
			}
			return false
		}
		if need >= 2 && !b.v2 {
			if report {
				b.v2Hint(a.Line, a.Col, n, "V002", "attribute %q on <%s>", name, tag)
			}
			return false
		}
		if name == "stick" {
			// SPEC v0.3b §6.15: stick needs an identity across frames that a
			// repeated row does not have. Only a version="3" document gets
			// here (the version gate above comes first, §5.1), and only on
			// a tag that takes stick.
			if it := listItem(n); it != nil {
				return err("attribute %q is not allowed inside a list <item>: its state needs an identity across frames that a repeated row does not have", name)
			} else if c := b.eachContainer(n); c != nil {
				return err("attribute %q is not allowed inside an each template (in %s): its state needs an identity across frames that a repeated row does not have", name, describeNodeAt(n, c))
			}
		}
	} else {
		return false // unknown tag already reported
	}
	if hasUpper(name) {
		return err("unknown attribute %q (attribute names are lowercase)", name)
	}
	return true
}

func (b *builder) attr(n *ir.Node, a RawAttr) {
	if !b.attrGate(n, a, true) {
		return
	}
	name, val := a.Name, a.Value
	tag := n.Tag
	if strings.HasPrefix(name, "on:") {
		if !ir.IsIdent(val) {
			b.attrErr(n, a, "V003", "%s=%q: an action is a plain name", name, val)
			return
		}
		n.On[strings.TrimPrefix(name, "on:")] = val
		return
	}
	if strings.HasPrefix(name, ClassGuardPrefix) {
		b.classGuard(n, a)
		return
	}
	switch name {
	case "id":
		if !isIDName(val) {
			b.attrErr(n, a, "V003", "bad id %q (letters, digits, '-' and '_'; must not start with a digit)", val)
		}
	case "class":
		for _, c := range strings.Fields(val) {
			if !isIDName(c) {
				b.attrErr(n, a, "V003", "bad class name %q", c)
				continue
			}
			// In version="2" an element's classes have no repeats, the
			// first occurrence wins (SPEC v0.2b §6.13): the dump's
			// classes, selectors, and inspect all read this list.
			// version="1" keeps the names as written (its IR and dumps
			// stay byte-identical to 0.2a).
			if b.v2 && slices.Contains(n.Classes, c) {
				continue
			}
			n.Classes = append(n.Classes, c)
		}
	case "width", "height":
		if _, err := ir.ParseScalar(val); err != nil {
			b.attrErr(n, a, "V003", "%s: %v", name, err)
			return
		}
		n.Hints = append(n.Hints, ir.Prop{Name: name, Value: val, Line: a.Line, Col: a.Col, Attr: attrText(a)})
	case "gap":
		if err := css.CheckDecl("gap", val); err != nil {
			b.attrErr(n, a, "V003", "%v", err)
			return
		}
		n.Hints = append(n.Hints, ir.Prop{Name: "gap", Value: val, Line: a.Line, Col: a.Col, Attr: attrText(a)})
	case "pad":
		if err := css.CheckDecl("padding", val); err != nil {
			b.attrErr(n, a, "V003", "pad: %v", err)
			return
		}
		n.Hints = append(n.Hints, ir.Prop{Name: "padding", Value: val, Line: a.Line, Col: a.Col, Attr: attrText(a)})
	case "border":
		v := val
		switch val {
		case "1":
			v = "single"
		case "0":
			v = "none"
		}
		if err := css.CheckDecl("border", v); err != nil {
			b.attrErr(n, a, "V003", "border=%q: want 1, 0, none, single, double, rounded, or thick", val)
			return
		}
		n.Hints = append(n.Hints, ir.Prop{Name: "border", Value: v, Line: a.Line, Col: a.Col, Attr: attrText(a)})
	case "wrap":
		if err := css.CheckDeclLang("wrap", val, b.lang); err != nil {
			b.attrErr(n, a, "V003", "%v", err)
			return
		}
		n.Hints = append(n.Hints, ir.Prop{Name: "wrap", Value: val, Line: a.Line, Col: a.Col, Attr: attrText(a)})
	case "style":
		// Recover per declaration (like a <style> rule's block), so one
		// typo does not also drop every other, valid declaration in the
		// same style="" attribute.
		decls, errs := css.ParseDeclsAllLang(val, b.lang)
		for _, err := range errs {
			b.attrErr(n, a, "V003", "%v", err)
		}
		for _, d := range decls {
			n.Inline = append(n.Inline, ir.Prop{Name: d.Prop, Value: d.Value, Line: a.Line, Col: a.Col})
		}
	case "if":
		if _, err := ir.ParseGuard(val); err != nil {
			b.attrErr(n, a, "V011", "if: %v", err)
			return
		}
		n.If = val
	case "each":
		if _, err := ir.ParseEach(val); err != nil {
			b.attrErr(n, a, "V011", "%v", err)
			return
		}
		n.Each = val
	case "hidden", "disabled":
		if val != "true" && val != "false" {
			if _, err := ir.ParseGuard(val); err != nil {
				b.attrErr(n, a, "V003", "%s=%q: want true, false, a path, or !path", name, val)
			}
		}
	case "focusable", "secret":
		if val != "true" && val != "false" {
			b.attrErr(n, a, "V003", "%s=%q: want true or false", name, val)
		}
	case "single":
		b.singleAttr(n, a)
	case "indeterminate":
		// SPEC v0.4b §6.4.1 (only the current language gets here, the
		// version gate above comes first): literal true or false. true
		// with bind or value is V003 and the progress stays a bar; the
		// test is on the attributes as written, so an invalid bind path
		// counts, whatever the order of the attributes.
		switch val {
		case "false":
		case "true":
			_, hasBind := n.Attrs["bind"]
			_, hasValue := n.Attrs["value"]
			if hasBind || hasValue {
				b.attrErr(n, a, "V003", "indeterminate=%q cannot be combined with bind or value", val)
				return
			}
			n.Spinner = true
		default:
			b.attrErr(n, a, "V003", "indeterminate=%q: want true or false", val)
		}
	case "bind":
		if !ir.IsPath(val) {
			b.attrErr(n, a, "V003", "bind=%q is not a path", val)
			return
		}
		n.Bind = val
	case "open":
		if val != "true" && val != "false" {
			if _, err := ir.ParseGuard(val); err != nil {
				b.attrErr(n, a, "V003", "open=%q: want true, false, a path, or !path", val)
			}
		}
	case "key":
		if !ir.IsPath(val) {
			b.attrErr(n, a, "V003", "key=%q is not a path", val)
		}
	case "title", "placeholder", "label", "keycap", "short", "mark":
		// XML attribute-value normalization: tabs and line breaks are spaces
		// (these values paint on one line).
		clean := b.sanitize(n, a.Line, a.Col, strings.Map(func(r rune) rune {
			if r == '\t' || r == '\n' || r == '\r' {
				return ' '
			}
			return r
		}, val))
		n.Attrs[name] = clean
		// A tab's label and short are templates in version="3" (SPEC v0.3b
		// §6.10.1, §7): {path} is checked like <text>'s body there. In
		// every other case (title, placeholder always; label, keycap,
		// short, and mark everywhere else, SPEC §6.10.1, §6.12) they are
		// literal text.
		tabTemplate := tag == "tab" && (name == "label" || name == "short")
		switch {
		case name == "title" || name == "placeholder":
			if !b.doc.Spike {
				if _, err := ir.ParseInterp(clean); err != nil {
					b.attrErr(n, a, "V003", "%s: %v", name, err)
				}
			}
		case tabTemplate && b.v3:
			if _, err := ir.ParseInterp(clean); err != nil {
				b.attrErr(n, a, "V003", "%s: %v", name, err)
			}
		case tabTemplate && ir.HasInterp(clean):
			b.v3Hint(a.Line, a.Col, n, "V003", "%s: {path} is only allowed in <text>, title, and placeholder", name)
		case (tag == "checkbox" || tag == "select") && name == "label":
			// A checkbox's label, and a select's, are templates (SPEC
			// v0.4b §6.16.1, §6.17.1); the tags exist only in the current
			// language.
			b.checkboxLabel(n, a, clean)
		case tag == "button" && name == "label" && b.lang.V4():
			// A button's label is a template in the current language (SPEC
			// v0.4b §6.4.2, §7): checked like a <text> body.
			if _, err := ir.ParseInterp(clean); err != nil {
				b.attrErr(n, a, "V003", "%s: %v", name, err)
			}
		case tag == "button" && name == "label" && ir.HasInterp(clean):
			// Pinned: V003 at the attribute, as in 0.4.1, with the hint of
			// §5.1 in place of the 0.4.1 wording (§6.4.2).
			b.currentHint(a.Line, a.Col, n, "V003", "{path} in a <button> label")
		case ir.HasInterp(clean):
			b.attrErr(n, a, "V003", "%s: {path} is only allowed in <text>, title, and placeholder", name)
		}
	case "axis":
		ok := val == "x" || val == "y" || (tag == "scroll" && val == "both")
		if !ok {
			want := "x or y"
			if tag == "scroll" {
				want = "x, y, or both"
			}
			b.attrErr(n, a, "V003", "axis=%q: want %s", val, want)
		}
	case "value":
		if !isPercent(val) {
			b.attrErr(n, a, "V003", "value=%q: want a number 0-100", val)
		}
	case "version":
		// SPEC v0.3 §5.1: exactly "1", "2", or "3"; any other value is
		// V003 and the document is checked and rendered as version="1".
		if val != "1" && val != "2" && val != "3" {
			b.attrErr(n, a, "V003", "version=%q: want \"1\", \"2\", or \"3\" (the document is read as version=\"1\")", val)
		}
		b.doc.Version = val
	case "theme":
		if val == "auto" {
			if !b.v2 {
				b.v2Hint(a.Line, a.Col, n, "V003", "theme=%q", val)
				return
			}
			b.doc.Theme = val
			return
		}
		if _, ok := css.Themes[val]; !ok {
			cands := []string{"dark", "light"}
			if b.v2 {
				cands = append(cands, "auto")
			}
			best := ir.Suggest(val, cands)
			if b.v2 {
				b.attrErr(n, a, "V003", "unknown theme %q (built-in: dark, light; or auto)%s", val, didYouMean(fmt.Sprintf("%q", best), best))
			} else {
				b.attrErr(n, a, "V003", "unknown theme %q (built-in: dark, light)%s", val, didYouMean(fmt.Sprintf("%q", best), best))
			}
			return
		}
		b.doc.Theme = val
	case "mouse":
		// SPEC §8.5: a flag, evaluated every frame.
		if !ir.IsFlag(val) {
			b.attrErr(n, a, "V003", "mouse=%q: want true, false, a path, or !path", val)
			return
		}
		b.doc.Mouse = val
	case "checked":
		if !ir.IsPath(val) {
			b.attrErr(n, a, "V003", "checked=%q is not a path", val)
		}
	case "scope":
		if val != "active" && val != "all" {
			b.attrErr(n, a, "V003", "scope=%q: want active or all", val)
		}
	case "min", "max":
		if !isNumber(val) {
			b.attrErr(n, a, "V003", "%s=%q: want a number such as 0, -5, or 0.25", name, val)
		}
	case "priority":
		// SPEC v0.3b §6.9.3: a non-negative integer; the smallest hides
		// first when the table's columns do not fit.
		if !isNonNegInt(val) {
			b.attrErr(n, a, "V003", "priority=%q: want a non-negative integer", val)
		}
	case "scale":
		// SPEC v0.3b §6.11: the sparklines of a frame sharing this name
		// share their range; NAME follows the ident grammar of §7.
		if !ir.IsIdent(val) {
			b.attrErr(n, a, "V003", "scale=%q: a scale name is a plain identifier ([A-Za-z_][A-Za-z0-9_]*)", val)
		}
	case "focus":
		if !strings.HasPrefix(val, "#") || !isIDName(val[1:]) {
			b.attrErr(n, a, "V003", "focus=%q: want #id", val)
		}
	case "stick":
		// SPEC v0.3b §6.2, §11.4: the only value is "bottom"; the axis and
		// id requirements are structural (checkNode).
		if val != "bottom" {
			b.attrErr(n, a, "V003", "stick=%q: want bottom", val)
		}
	case "keys", "action", "when", "to", "src":
		// validated with the keymap / stylesheet loader
	}
}

// classGuard records a class:NAME="guard" attribute of a version="2"
// element (SPEC §6.13): NAME must be a guardname and the value path or
// !path, else V015. An uppercase letter in NAME is already V002 (attribute
// names are lowercase, SPEC §5), and a repeated NAME is a repeated
// attribute, V005.
func (b *builder) classGuard(n *ir.Node, a RawAttr) {
	name := strings.TrimPrefix(a.Name, ClassGuardPrefix)
	if !ir.IsGuardName(name) {
		b.attrErr(n, a, "V015", "%s: %q is not a class name for a guard ([a-z_][a-z0-9_-]*)", a.Name, name)
		return
	}
	if _, err := ir.ParseGuard(a.Value); err != nil {
		b.attrErr(n, a, "V015", "%s=%q: a class guard is path or !path", a.Name, a.Value)
		return
	}
	n.ClassGuards = append(n.ClassGuards, ir.ClassGuard{Name: name, Guard: a.Value})
}

// attrText is an attribute as written, `name="value"` (`tuimark inspect`
// names a presentational attribute that way, SPEC §15.7).
func attrText(a RawAttr) string { return a.Name + `="` + a.Value + `"` }

// isNumber matches the number production of SPEC §7 (sparkline min and
// max): an optional '-', digits, and optionally '.' and digits.
func isNumber(s string) bool {
	s = strings.TrimPrefix(s, "-")
	_, _, ok := ir.ParseDecimalLit(s)
	return ok
}

// isPercent reports whether s is a plain decimal in 0-100, compared on the
// literal (100.00000000000000001 is over 100 even though its float64 is not).
func isPercent(s string) bool {
	lit, _, ok := ir.ParseDecimalLit(s)
	return ok && ir.DecimalCmp(lit, 100) <= 0
}

// isNonNegInt reports whether s is a plain non-negative integer literal:
// one or more digits, no sign, no fraction (SPEC v0.3b §6.9.3, `priority`
// on a column).
func isNonNegInt(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func isIDName(s string) bool {
	if s == "" || (s[0] >= '0' && s[0] <= '9') || s[0] == '-' {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c == '-' || c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

// xmlSpaceCutset is the XML S production (space, tab, CR; \n is the line
// separator NormalizeText itself splits on and is never part of a line).
// Trimming only these — not every Unicode space — means a control
// character such as VT or NEL that a naive TrimSpace would silently eat
// instead reaches sanitize/StripControl and is reported as V007, and a
// non-ASCII space such as NBSP or U+3000 (the only way to indent <text>,
// since numeric character references are rejected) survives as content.
const xmlSpaceCutset = " \t\r"

// isXMLWhitespaceOnly reports whether s contains only XML S characters
// (space, tab, CR, LF), the same "ignorable whitespace" §5 describes for
// non-text elements. Unlike unicode.IsSpace/strings.TrimSpace, it does not
// treat VT, FF, NEL, NBSP, or U+3000 as whitespace, so text made only of
// those is not silently ignored.
func isXMLWhitespaceOnly(s string) bool {
	return strings.Trim(s, xmlSpaceCutset+"\n") == ""
}

// NormalizeText trims each line and drops leading/trailing blank lines, so
// indentation in the .tui source does not leak into the grid.
func NormalizeText(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.Trim(l, xmlSpaceCutset)
	}
	for len(lines) > 0 && lines[0] == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

func (b *builder) content(n *ir.Node, r *RawNode) {
	pb := b.pbody // what node found in the body, if it held a {$name}
	b.pbody = nil
	var text strings.Builder
	textLine, textCol := 0, 0
	for _, c := range r.Children {
		switch c.Type {
		case RawComment:
			continue
		case RawText, RawCData:
			if textLine == 0 {
				textLine, textCol = c.Line, c.Col
			}
			text.WriteString(c.Text)
		case RawElement:
			if c.Name == "slot" && b.comp != nil && b.slotIn(n, c) {
				// SPEC v0.4c §6.18.10 (slot.go): its rules come before
				// those of an element that takes no element children.
				continue
			}
			if n.Tag == "text" {
				b.diag(ir.Error, "V013", c.Line, c.Col, n.Path, n.ID, "<text> cannot contain elements (found <%s>)", c.Name)
				continue
			}
			// SPEC §6.9.1: a column's body is its cell template (text
			// only); a table's <item> is the row template, with no
			// children.
			if b.v2 && n.Tag == "column" {
				b.diagPlaced(ir.Error, "V016", c.Line, c.Col, n.Path, n.ID, "<column> holds its cell template as text, with {path} for the row's values; it cannot contain elements (found <%s>)", c.Name)
				continue
			}
			if b.tableItem(n) {
				b.diagPlaced(ir.Error, "V016", c.Line, c.Col, n.Path, n.ID, "a table's <item> is the row template: it takes class and class:NAME only, no children (found <%s>); the cells come from the <column> elements", c.Name)
				continue
			}
			if (b.v2 && (n.Tag == "sparkline" || n.Tag == "hints")) || b.formNoContent(n) {
				// SPEC §6.11, §6.12: no children (a hints' items come
				// from the keymap); a form control neither (v0.4b §6.16.1).
				b.diagPlaced(ir.Error, "V016", c.Line, c.Col, n.Path, n.ID, "<%s> takes no children (found <%s>)", n.Tag, c.Name)
				continue
			}
			if leafKinds[n.Tag] {
				b.diag(ir.Error, "V013", c.Line, c.Col, n.Path, n.ID, "<%s> cannot contain elements (found <%s>)", n.Tag, c.Name)
				continue
			}
			if b.comp != nil && b.compose(n, c, nil) {
				// SPEC v0.4b §6.18.1: the elements of a fragment, or of a
				// template's copy, take the include's or the use's place,
				// built here as if written here (expand.go). This comes
				// after the rules above: in an element that takes no
				// element children, an include or a use is reported by
				// that rule and not expanded (§6.18.2, §6.18.3).
				continue
			}
			// An unknown kind still gets its subtree validated (V001 plus
			// whatever its own children report), but it is dropped here so
			// it never reaches layout, paint, dump nodes, or `tuimark ir`.
			// The tree-level checks (V004, V012, V013, B006) still visit
			// it through walkAll.
			b.attach(n, nil, b.node(c, n))
		}
	}
	body := text.String()
	switch n.Tag {
	case "style":
		n.Text = body
		b.addStyle(n, body, textLine, textCol)
		if n.Attrs["src"] != "" && !isXMLWhitespaceOnly(body) {
			b.errN(n, "V013", "<style> takes either src or an inline body, not both")
		}
	case "text", "button", "column":
		if pb != nil {
			// A body that held a {$name} (SPEC v0.4c §6.18.9, props.go).
			var check bool
			if body, check = pb.body(b, n, body, textLine, textCol); !check {
				break
			}
			defer func() { b.at.on, b.mute.on = false, false }()
		}
		// Trim the source indentation (spaces or tabs) first, so only control
		// characters inside the text itself are V007. A version="2"
		// <column>'s body is its cell template, a template like a <text>
		// body (SPEC §6.9.1, §7).
		clean := b.sanitize(n, textLine, textCol, NormalizeText(body))
		n.Text = NormalizeText(clean)
		if (n.Tag == "text" || n.Tag == "column") && !b.doc.Spike {
			if _, err := ir.ParseInterp(n.Text); err != nil {
				b.diag(ir.Error, "V003", textLine, textCol, n.Path, n.ID, "%v", err)
			}
		}
		if n.Tag == "button" {
			if b.lang.V4() {
				// The body is a template like a <text> body in the current
				// language (SPEC v0.4b §6.4.2).
				if _, err := ir.ParseInterp(n.Text); err != nil {
					b.diag(ir.Error, "V003", textLine, textCol, n.Path, n.ID, "%v", err)
				}
			} else if ir.HasInterp(n.Text) {
				// Pinned: V003 once per body, at the <button>, as in 0.4.1,
				// with the hint of §5.1 (§6.4.2).
				b.currentHint(n.Line, n.Col, n, "V003", "{path} in a <button> label")
			}
		}
	default:
		if isXMLWhitespaceOnly(body) {
			break
		}
		switch {
		case b.v2 && n.Tag == "table":
			b.diagPlaced(ir.Error, "V016", textLine, textCol, n.Path, n.ID, "text is not allowed directly inside <table>; a column's cell template is the body of its <column>")
		case b.v2 && n.Tag == "tabs":
			b.diagPlaced(ir.Error, "V016", textLine, textCol, n.Path, n.ID, "text is not allowed directly inside <tabs>; put it inside a <tab>, and name the tab with label=")
		case (b.v2 && (n.Tag == "sparkline" || n.Tag == "hints")) || b.formNoContent(n):
			b.diagPlaced(ir.Error, "V016", textLine, textCol, n.Path, n.ID, "<%s> takes no text", n.Tag)
		case b.tableItem(n):
			b.diagPlaced(ir.Error, "V016", textLine, textCol, n.Path, n.ID, "a table's <item> is the row template and takes no text; the cells come from the <column> elements")
		case !b.tagKnown(n.Tag):
			// V001 already names this element; "wrap it in <text>" would
			// only be noise next to it.
		default:
			b.diag(ir.Error, "V013", textLine, textCol, n.Path, n.ID, "text is not allowed directly inside <%s>; wrap it in <text>", n.Tag)
		}
	}
}

// styleKey names the place a <style> is written: its file, line, and column.
func styleKey(file string, line, col int) string {
	return fmt.Sprintf("%s\x00%d\x00%d", file, line, col)
}

// addStyle records a <style> element. A <style> placed more than once (a
// fragment included twice) is applied once, at its first place in document
// order (SPEC v0.4b §6.18.2): the elements written at the same place of the
// same file are one.
func (b *builder) addStyle(n *ir.Node, body string, textLine, textCol int) {
	file := n.FileName(b.file)
	key := styleKey(file, n.Line, n.Col)
	if b.styleSeen[key] {
		return
	}
	if b.styleSeen == nil {
		b.styleSeen = map[string]bool{}
	}
	b.styleSeen[key] = true
	if b.comp != nil && b.comp.noLoad[n] {
		// src holds a {$name} outside a template: loads nothing, with no
		// V006 (SPEC v0.4c §6.18.9, decision 184).
		return
	}
	ref := StyleRef{Src: n.Attrs["src"], Body: body, Line: n.Line, Col: n.Col, BodyLine: textLine, BodyCol: textCol, File: file}
	if n.Org != nil && n.Org.Placed != nil {
		ref.Org = n.Org
	}
	if n.Org != nil && b.comp != nil && len(b.comp.chain) > 1 {
		// Written in a fragment: a relative src resolves against that
		// file's directory, and the sheet is named by its path relative to
		// the document's directory (SPEC v0.4b §6.18.6).
		cur := b.comp.chain[len(b.comp.chain)-1]
		ref.From = cur.path
		if ref.Src != "" {
			ref.Rel = relFrom(cur.name, ref.Src)
		}
	}
	b.doc.Styles = append(b.doc.Styles, ref)
}

// relFrom joins src, written in the file called name (a path relative to
// the document's directory), to that file's directory: the name of a
// stylesheet a fragment loads. An absolute src stays as it is.
func relFrom(name, src string) string {
	if strings.HasPrefix(src, "/") || filepath.IsAbs(src) {
		return src
	}
	d := name
	if i := strings.LastIndexByte(d, '/'); i >= 0 {
		d = d[:i]
	} else {
		d = "."
	}
	return path.Join(d, src)
}

// tableItem reports whether n is the row template <item> of a table (a
// version="2" document; SPEC §6.9.1).
func (b *builder) tableItem(n *ir.Node) bool {
	return b.v2 && n.Tag == "item" && n.Parent != nil && n.Parent.Tag == "table"
}

// checkStructure applies document-level rules that need the whole tree.
func (b *builder) checkStructure() {
	root := b.doc.Root
	if b.doc.Spike {
		b.walkAll(root, false, func(n *ir.Node, _ bool) { b.checkNode(n) })
		return
	}
	for _, c := range root.Children {
		if c.Tag == "screen" {
			b.doc.Screens = append(b.doc.Screens, c)
		}
		if c.Tag == "keymap" {
			// <keymap when="SEL"> (version="3", SPEC v0.3b §8.1) gives each
			// of its rows that default when, unless the row writes its own
			// (which replaces it, even when="").
			kmWhen := ""
			if b.v3 {
				kmWhen, _ = c.Attr("when")
			}
			for _, k := range c.Children {
				if k.Tag == "bind" {
					rowWhen, hasOwnWhen := k.Attr("when")
					kb := KeyBind{
						KeysRaw: k.Attrs["keys"], Action: k.Attrs["action"], When: rowWhen,
						HasOwnWhen: hasOwnWhen, KeymapNode: c, EffectiveWhen: rowWhen,
						To: k.Attrs["to"], Line: k.Line, Col: k.Col, Org: k.Org, Node: k,
					}
					if !hasOwnWhen {
						kb.EffectiveWhen = kmWhen
					}
					if b.v2 {
						kb.Label, kb.HasLabel = k.Attr("label")
						kb.Keycap, kb.HasKeycap = k.Attr("keycap")
					}
					b.doc.Keymap = append(b.doc.Keymap, kb)
				}
			}
		}
	}
	if len(b.doc.Screens) == 0 {
		b.errN(root, "V005", "document has no <screen>")
	}
	for _, s := range b.doc.Screens {
		if s.ID == "" && len(b.doc.Screens) > 1 {
			b.errN(s, "V012", "each <screen> needs an id when there are several")
		}
		// Modals form a trailing group: nothing but modals after the first one.
		seenModal := false
		for _, c := range s.Children {
			if c.Tag == "modal" {
				seenModal = true
			} else if seenModal {
				b.errN(c, "L004", "<modal> must be the last child of <screen> (found <%s> after it)", c.Tag)
			}
		}
	}
	b.walkAll(root, false, func(n *ir.Node, _ bool) { b.checkNode(n) })
}

// walkAll walks the whole source in document order: the tree, plus the
// subtrees of unknown tags that content() keeps out of Children.
// dropped tells fn whether n is in such a subtree; an unknown-tag node
// itself is never passed to fn (its attributes, id included, are not
// validated: V001 is all it gets).
func (b *builder) walkAll(n *ir.Node, dropped bool, fn func(n *ir.Node, dropped bool)) {
	b.walkSeq(n, dropped, false, fn)
}

// walkSeq is walkAll that, with ghosts, also passes fn the <template> and
// <use> elements where they stand (expand.go), without walking into them:
// the id space holds their ids, in document order (SPEC v0.4b §6.18.4).
func (b *builder) walkSeq(n *ir.Node, dropped, ghosts bool, fn func(n *ir.Node, dropped bool)) {
	if b.tagKnown(n.Tag) {
		fn(n, dropped)
	}
	extra := b.droppedIn[n]
	if len(extra) == 0 {
		for _, c := range n.Children {
			b.walkSeq(c, dropped || !b.tagKnown(c.Tag), ghosts, fn)
		}
		return
	}
	// Merge by the index content() recorded: a source position says
	// nothing about the order of nodes read from different files.
	j := 0
	for i := 0; i <= len(n.Children); i++ {
		for j < len(extra) && extra[j].at == i {
			switch e := extra[j]; {
			case e.ghost == 0:
				b.walkSeq(e.node, true, ghosts, fn)
			case ghosts:
				fn(e.node, dropped)
			}
			j++
		}
		if i < len(n.Children) {
			c := n.Children[i]
			b.walkSeq(c, dropped || !b.tagKnown(c.Tag), ghosts, fn)
		}
	}
}

func (b *builder) checkNode(n *ir.Node) {
	// SPEC §8.3: "Default focusable: input, list, button, modal. Others
	// only if focusable=\"true\"." followed by "Focusable widgets without
	// id -> V012" — the general rule, not just the four default kinds.
	// A list <item> and everything inside it are exempt: the list
	// navigates between its items, which are never focused themselves
	// (host.collectFocusables skips them), an each= template item cannot
	// have a unique id of its own, and a focus target inside a row is
	// already V001/V002 (checkTag, attr), which an id would not fix.
	focusable := needsID[n.Tag]
	if v, ok := n.Attrs["focusable"]; ok && v == "true" {
		focusable = true
	}
	if listItem(n) != nil || b.eachContainer(n) != nil {
		// (A focus target inside a container each template is already
		// V001/V002, and any id there V004: SPEC §6.8 item 8.)
		focusable = false
	}
	if focusable && b.noID(n) && !(b.v2 && (n.Tag == "table" || n.Tag == "tabs")) {
		// (A table needs an id whether or not it is focusable: checkTable.)
		b.errN(n, "V012", "<%s> is focusable and needs an id", n.Tag)
	}
	if b.v2 {
		b.checkAliases(n)
		b.checkMultiSelect(n)
		switch n.Tag {
		case "table":
			b.checkTable(n)
		case "tabs", "tab":
			b.checkTabs(n)
		case "select":
			b.checkSelect(n)
		case "sparkline":
			// SPEC §6.11: bind is required.
			if !b.wrote(n, "bind") {
				b.errN(n, "V003", "<sparkline> needs bind=\"path\": the numeric array it draws")
			}
		}
	}
	if b.v3 && n.Tag == "scroll" {
		b.checkStick(n)
	}
	if n.Tag == "list" {
		items := 0
		for _, c := range n.Children {
			if c.Tag == "item" {
				items++
			}
		}
		if n.Each != "" {
			if items != 1 {
				b.errN(n, "V013", "<list each=...> needs exactly one <item> template (found %d)", items)
			}
			if !b.wrote(n, "key") {
				// The each as written, as V011 quotes it (writtenEach).
				b.warnN(n, "B006", "<list each=%q> has no key=; selection follows the index", writtenEach(n))
			}
		}
	}
	if n.Tag == "rule" || n.Tag == "spacer" {
		if n.Text != "" {
			b.errN(n, "V013", "<%s> takes no content", n.Tag)
		}
	}
}

// checkTable applies the structure rules of SPEC §6.9.1 to a table: it
// needs an id (V012: its cursor and offset are kept by it), each= (V017),
// and at least one <column> child (V017); a second <item> is V016; without
// key= it reports B006 (a warning), as a list does. Inside a list <item>
// or a container each template a table is already V001 as a whole, so it
// gets neither V012 (an id there would be V004) nor B006.
func (b *builder) checkTable(n *ir.Node) {
	nested := listItem(n.Parent) != nil || b.eachContainer(n) != nil
	if b.noID(n) && !nested {
		b.errN(n, "V012", "<table> needs an id: its cursor and scroll offset are kept by it, and to=\"#id\" names it")
	}
	eachAttr, _ := n.Attr("each")
	if !b.wrote(n, "each") {
		b.errN(n, "V017", "<table> needs each=\"path as alias\": its rows are the elements of an array")
	}
	cols, items := 0, 0
	for _, c := range n.Children {
		switch c.Tag {
		case "column":
			cols++
		case "item":
			items++
			if items == 2 {
				b.errN(c, "V016", "a <table> has at most one <item>, its row template")
			}
		}
	}
	if cols == 0 {
		b.errN(n, "V017", "<table> needs at least one <column>")
	}
	if n.Each != "" && !b.wrote(n, "key") && !nested {
		if n.Written != nil {
			// As written: an element of a copy holds its paths after the
			// with aliases (Node.Written), as V011 says.
			eachAttr = n.Written.Attrs["each"]
		}
		b.warnN(n, "B006", "<table each=%q> has no key=; the cursor follows the row index", eachAttr)
	}
}

// checkTabs applies the structure rules of SPEC §6.10.1 to a tabs or a
// tab: both need an id (V012: the tab ids are the bound values, the
// switch-to targets, and the label keys; the tabs id keeps the remembered
// tab); a tabs needs at least one tab (V017); a tab needs a non-empty
// label (V003). Inside a list <item> or a container each template they
// are already V001 as a whole, so they get no V012 (an id there would be
// V004).
func (b *builder) checkTabs(n *ir.Node) {
	nested := listItem(n.Parent) != nil || b.eachContainer(n) != nil
	if b.noID(n) && !nested {
		if n.Tag == "tabs" {
			b.errN(n, "V012", "<tabs> needs an id: it keeps the remembered tab, and move-* and to=\"#id\" name it")
		} else {
			b.errN(n, "V012", "<tab> needs an id: it is the value of the tabs' bind, the target of switch-to, and its label's key")
		}
	}
	if n.Tag == "tabs" {
		tabs := 0
		for _, c := range n.Children {
			if c.Tag == "tab" {
				tabs++
			}
		}
		if tabs == 0 {
			b.errN(n, "V017", "<tabs> needs at least one <tab>")
		}
		return
	}
	if l, ok := n.Attr("label"); !ok || l == "" {
		b.errN(n, "V003", "<tab> needs a non-empty label=: it is the text of its label in the strip")
	}
}

// checkTabFocus reports B005 for a tab focus="#id" that names no node, a
// node inside a list <item>, or a node outside that tab (SPEC §6.10.1).
func (b *builder) checkTabFocus(n *ir.Node) {
	if n.Tag == "tab" {
		if f, ok := n.Attr("focus"); ok && strings.HasPrefix(f, "#") && isIDName(f[1:]) {
			id := f[1:]
			if why := b.idTargetAt(n, id); strings.HasPrefix(why, idMissing) {
				b.errN(n, "B005", "focus=%q refers to an id that %s", f, why)
			} else if why != "" {
				b.errN(n, "B005", "focus=%q %s", f, why)
			} else if !inside(b.doc.IDs[id], n) {
				b.errN(n, "B005", "focus=%q names a node outside this tab; a tab's focus= names a node inside it", f)
			}
		}
	}
	for _, c := range n.Children {
		b.checkTabFocus(c)
	}
}

// checkStick applies the structural rules of SPEC v0.3b §11.4 to a
// scroll's stick="bottom": it needs an id (V012: the offset and the
// previous maximum are kept across frames by it) and a y scroll axis
// (V003 on axis="x"). Inside a list <item> or a container each template
// stick is already V002 (attr), reported nothing further here.
func (b *builder) checkStick(n *ir.Node) {
	if _, ok := n.Attr("stick"); !ok {
		return
	}
	if listItem(n) != nil || b.eachContainer(n) != nil {
		return
	}
	if b.noID(n) {
		b.errN(n, "V012", `<scroll stick="bottom"> needs an id: its offset and the previous maximum are kept across frames by it`)
	}
	if ax, _ := n.Attr("axis"); ax == "x" {
		b.errN(n, "V003", `stick="bottom" needs a y scroll axis; axis="x" cannot stick`)
	}
}

// checkModalFocus reports B005 for a modal focus="#id" that names no
// node, a node inside a list <item> or an each template, or a node
// outside the modal (SPEC v0.3b §8.3, mirroring a tab's focus=, §6.10.1).
func (b *builder) checkModalFocus(n *ir.Node) {
	if n.Tag == "modal" {
		if f, ok := n.Attr("focus"); ok && strings.HasPrefix(f, "#") && isIDName(f[1:]) {
			id := f[1:]
			if why := b.idTargetAt(n, id); strings.HasPrefix(why, idMissing) {
				b.errN(n, "B005", "focus=%q refers to an id that %s", f, why)
			} else if why != "" {
				b.errN(n, "B005", "focus=%q %s", f, why)
			} else if !inside(b.doc.IDs[id], n) {
				b.errN(n, "B005", "focus=%q names a node outside this modal; a modal's focus= names a node inside it", f)
			}
		}
	}
	for _, c := range n.Children {
		b.checkModalFocus(c)
	}
}

// inside reports whether n is anc or one of its descendants.
func inside(n, anc *ir.Node) bool {
	for p := n; p != nil; p = p.Parent {
		if p == anc {
			return true
		}
	}
	return false
}

// checkAliases reports V011 for an each alias equal to the alias of an
// enclosing each, on a container, a list, or a table (SPEC §6.8 item 7,
// §7). Only version="2" documents can nest an each inside another.
func (b *builder) checkAliases(n *ir.Node) {
	if n.Each == "" {
		return
	}
	e, err := ir.ParseEachField(n.Each)
	if err != nil {
		return
	}
	// An alias the parser renamed in a copy (ir.MarkAlias) compares as
	// written: renaming keeps a with path from being captured, it does not
	// change what §6.8 says about two aliases.
	alias := ir.Unmark(e.Alias)
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Each == "" {
			continue
		}
		if pe, err := ir.ParseEachField(p.Each); err == nil && ir.Unmark(pe.Alias) == alias {
			// The message quotes each each as written: an element of a
			// copy holds its paths after the with aliases (Node.Written).
			b.errN(n, "V011", "each=%q: the alias %q repeats the alias of the enclosing each=%q of %s; pick another name", writtenEach(n), alias, ir.ShownIDAt(n, writtenEach(p)), describeNodeAt(n, p))
			return
		}
	}
}

// writtenEach is n's each as the document wrote it: what an element of a
// copy wrote before its paths went through the with aliases in scope.
func writtenEach(n *ir.Node) string {
	if n.Written != nil {
		return n.Written.Each
	}
	return n.Each
}

// checkMultiSelect applies the V018 rules of SPEC §6.14 and §6.10.1:
// checked needs each= and key= on the same list or table (a list of
// static items cannot have it); mark is 1 or 2 columns wide and, on a list
// or table, needs checked; on:change on a list needs checked.
func (b *builder) checkMultiSelect(n *ir.Node) {
	switch n.Tag {
	case "list", "table", "tabs":
	default:
		return
	}
	hasChecked := b.wrote(n, "checked")
	b.checkSingle(n, hasChecked)
	if hasChecked && n.Tag != "tabs" {
		hasEach, hasKey := b.wrote(n, "each"), b.wrote(n, "key")
		if !hasEach || !hasKey {
			what := "each= and key="
			switch {
			case hasEach:
				what = "key="
			case hasKey:
				what = "each="
			}
			b.errN(n, "V018", "checked on <%s> needs %s on the same element: the checked array holds row keys (a list of static items cannot have it)", n.Tag, what)
		}
	}
	if mark, ok := n.Attr("mark"); ok {
		// (A mark holding {path} is already V003: it is literal text.)
		if w := uniwidth.Width(mark); (w < 1 || w > 2) && !ir.HasInterp(mark) {
			b.errN(n, "V018", "mark=%q is %d columns wide; a mark is 1 or 2 columns wide", mark, w)
		}
		if !hasChecked && n.Tag != "tabs" {
			b.errN(n, "V018", "mark on <%s> needs checked=: the mark shows the checked rows", n.Tag)
		}
	}
	if _, ok := n.On["change"]; ok && n.Tag == "list" && !hasChecked {
		b.errN(n, "V018", "on:change on <list> needs checked=: it fires when the checked array changes")
	}
}

// checkIDs reports duplicate ids (V004) over the whole source, in document
// order, the subtrees of unknown tags included. Only ids in the tree enter
// doc.IDs (the first of each), so a dropped subtree can collide with the
// tree but never becomes addressable.
func (b *builder) checkIDs(root *ir.Node) {
	seen := map[string]*ir.Node{}
	// The <template> and <use> elements are in the id space too (SPEC
	// v0.4b §6.18.4): walkSeq passes them where they stand.
	b.walkSeq(root, false, b.comp != nil, func(n *ir.Node, dropped bool) {
		if n.ID == "" {
			return
		}
		inTemplate := b.eachContainer(n) != nil
		if prev, ok := seen[n.ID]; ok {
			if !sameIDCopies(n, prev) {
				b.dupID(n, prev)
			}
		} else {
			seen[n.ID] = n
			if inTemplate {
				// SPEC §6.8 item 8: the template is inflated once per
				// element, so its id would repeat.
				b.errN(n, "V004", "id %q inside an each template (%s) would repeat for every element: %s", ir.ShownIDAt(n, n.ID), describeNodeAt(n, b.eachContainer(n)), templateHint)
			}
		}
		if dropped {
			return
		}
		if b.ghostID(n) {
			return // not a node (expand.go)
		}
		if _, ok := b.doc.IDs[n.ID]; !ok {
			b.doc.IDs[n.ID] = n
		}
		if listItem(n) == nil && !inTemplate {
			if b.addressable == nil {
				b.addressable = map[string]bool{}
			}
			b.addressable[n.ID] = true
		}
	})
}

const idMissing = "does not exist"

// idTarget reports why #id cannot be a focus/keymap target: it does not
// exist, or it exists only inside a list <item>, where nothing is focused
// or addressed by id at run time (the host inflates rows without an id
// index). It returns "" when the id is a valid target.
//
// A missing id's reason starts with idMissing and may go on: in a document
// with templates, an id that is not a node may be a <use>'s or a
// <template>'s, or one an element of a template writes (expand.go).
func (b *builder) idTarget(id string) string {
	return b.idTargetAt(nil, id)
}

// idTargetAt is idTarget for a diagnostic about the node at: the list a
// reason names is shown as ir.ShownIDAt shows it there (a tab or a modal
// of a copy is reported once per copy).
func (b *builder) idTargetAt(at *ir.Node, id string) string {
	n, ok := b.doc.IDs[id]
	if !ok {
		if b.comp != nil {
			return b.notANode(id)
		}
		return idMissing
	}
	if !b.addressable[id] {
		if it := listItem(n); it != nil {
			return "is inside a list <item>, where nothing takes focus or is addressed by id: " + listHintAt(at, it)
		}
		return "is inside an each template, where nothing takes focus or is addressed by id: " + templateHint
	}
	return ""
}

// checkBuiltinRow applies the static checks of SPEC §8.4 to a keymap row
// naming a hyphenated built-in: switch-to needs to= (V003), and B007 (an
// error) reports a target known without data to be incompatible. The
// target is known when to= names a node, or, without to=, when when= is
// exactly one compound of an #id and :focus (such as #procs:focus), which
// fixes the focused node. toOK reports that to= names an addressable node.
func (b *builder) checkBuiltinRow(k *KeyBind, path string, toOK bool) {
	if k.Action == "switch-to" && k.To == "" {
		b.errKB(k, "V003", path, `action="switch-to" needs to="#tab" or to="#screen"`)
		return
	}
	target, how := "", ""
	switch {
	case toOK:
		target, how = k.To, fmt.Sprintf("to=\"#%s\"", k.To)
	case k.To == "" && k.WhenSel != nil && len(k.WhenSel.Parts) == 1:
		c := k.WhenSel.Parts[0]
		if c.ID != "" && c.Tag == "" && len(c.Classes) == 0 && len(c.Pseudos) == 1 && c.Pseudos[0] == "focus" {
			if b.idTarget(c.ID) == "" {
				target, how = c.ID, fmt.Sprintf("when=%q", k.EffectiveWhen)
			}
		}
	}
	if target == "" {
		return
	}
	n := b.doc.IDs[target]
	if n == nil {
		return
	}
	if why := BuiltinIncompatible(k.Action, n.Kind, hasAttr(n, "checked"), n.Single); why != "" {
		b.errKB(k, "B007", path, "action=%q: its target #%s (from %s) is a <%s>, %s", k.Action, target, how, n.Tag, why)
	}
}

func hasAttr(n *ir.Node, name string) bool {
	_, ok := n.Attr(name)
	return ok
}

// BuiltinIncompatible returns why a node of the given kind can never be
// the target of the hyphenated built-in action (SPEC §8.4 "Static
// checks"), or "" when data or state may make it compatible. checked
// tells whether the node has a checked= attribute, and single whether it
// is a single-choice widget (single="true", SPEC v0.4b §6.14.1).
func BuiltinIncompatible(action, kind string, checked, single bool) string {
	switch action {
	case "move-next", "move-prev", "move-first", "move-last", "move-page-down", "move-page-up":
		switch kind {
		case "text", "rule", "spacer", "input", "button", "progress", "sparkline", "hints", "column", "tab", "item", "checkbox", "select":
			return "which has no cursor, no tabs, and no scroll offset to move (want a list, a table, a tabs, or a viewport)"
		case "tabs":
			if action == "move-page-down" || action == "move-page-up" {
				return "whose strip does not page (use move-next or move-prev)"
			}
		}
	case "check-toggle", "check-all", "check-none":
		if (kind != "list" && kind != "table") || !checked {
			return "not a list or table with checked= (the array of checked row keys)"
		}
		if action == "check-all" && single {
			// SPEC v0.4b §6.14.1, §8.4.
			return `which has single="true": it holds one checked row, so check-all never applies`
		}

	case "switch-to":
		if kind != "tab" && kind != "screen" {
			return "not a tab or a screen"
		}
	}
	return ""
}

// checkKeymap validates key tokens, actions, and selectors (V003/B005). A
// <keymap when="SEL"> (version="3", SPEC v0.3b §8.1) is checked once, at
// the <keymap> element itself (keymapWhens); a row without its own when=
// inherits that already-validated selector instead of being checked
// again.
func (b *builder) checkKeymap() {
	kmSel := b.keymapWhens()
	for i := range b.doc.Keymap {
		k := &b.doc.Keymap[i]
		path := "/keymap/bind"
		if strings.TrimSpace(k.KeysRaw) == "" {
			b.errKB(k, "V003", path, "<bind> needs keys=")
		}
		k.Keys = ir.SplitKeys(k.KeysRaw)
		for _, tok := range k.Keys {
			if !ir.ValidKey(tok) {
				b.errKB(k, "V003", path, "unknown key %q (see the key token list: a-z 0-9 enter esc tab backspace space arrows home end pgup pgdn ctrl+x shift+tab)", tok)
				continue
			}
			// SPEC v0.4b §8.1: in the current language a token that never
			// reaches a keymap is V003, at the <bind>, once per token, with
			// no version hint (decision 83). The row and its other tokens
			// stay. A pinned document accepts it, never matching, as in
			// 0.4.1.
			if as, never := ir.ArrivesAs(tok); never && b.lang >= ir.KeysRejectedSince {
				b.errKB(k, "V003", path, "key %q never reaches a keymap: it arrives as %s (write %s)", tok, as, as)
			}
		}
		switch {
		case k.Action == "":
			b.errKB(k, "V003", path, "<bind> needs action=")
		case ir.IsBuiltinActionV2(k.Action):
			// SPEC §8.4: the hyphenated built-ins are version="2" keymap
			// actions (ADR 0006).
			if !b.v2 {
				b.errKB(k, "V003", path, "action=%q: built-in action%s", k.Action, ir.VersionHint)
			}
		case !ir.IsIdent(k.Action):
			b.errKB(k, "V003", path, "action=%q: an action is a plain name", k.Action)
		}
		switch {
		case k.HasOwnWhen:
			if k.When != "" {
				sel, err := css.ParseSelectorLang(k.When, b.lang)
				if err == nil && sel.Root {
					err = fmt.Errorf(":root is not a node selector")
				}
				if err != nil {
					b.errKB(k, "V003", path, "when=%q: %v", k.When, err)
				} else {
					k.WhenSel = &sel
					for _, part := range sel.Parts {
						if part.ID != "" {
							if why := b.idTarget(part.ID); why != "" {
								b.errKB(k, "B005", path, "when=%q refers to #%s, which %s", k.When, part.ID, why)
							}
						}
					}
				}
			}
		case k.KeymapNode != nil:
			k.WhenSel = kmSel[k.KeymapNode]
		}
		toOK := false
		if k.To != "" {
			if !strings.HasPrefix(k.To, "#") || !isIDName(k.To[1:]) {
				b.errKB(k, "V003", path, "to=%q: want #id", k.To)
			} else {
				k.To = k.To[1:]
				if why := b.idTarget(k.To); strings.HasPrefix(why, idMissing) {
					b.errKB(k, "B005", path, "to=\"#%s\" refers to an id that %s", k.To, why)
				} else if why != "" {
					b.errKB(k, "B005", path, "to=\"#%s\" %s", k.To, why)
				} else {
					toOK = true
				}
			}
		}
		if k.Action == "focus" && k.To == "" {
			b.errKB(k, "V003", path, `action="focus" needs to="#id"`)
		}
		if b.v2 && ir.IsBuiltinActionV2(k.Action) {
			b.checkBuiltinRow(k, path, toOK)
		}
	}
	for _, s := range b.doc.Screens {
		if f, ok := s.Attr("focus"); ok && strings.HasPrefix(f, "#") {
			if why := b.idTarget(f[1:]); strings.HasPrefix(why, idMissing) {
				b.errN(s, "B005", "focus=%q refers to an id that %s", f, why)
			} else if why != "" {
				b.errN(s, "B005", "focus=%q %s", f, why)
			}
		}
		if b.v2 {
			b.checkTabFocus(s)
		}
		if b.v3 {
			b.checkModalFocus(s)
		}
	}
}

// keymapWhens validates the own when= of every <keymap> element once
// (version="3", SPEC v0.3b §8.1): a bad selector is one V003, and an id
// that names no node, or a node inside a list item or an each template,
// is one B005, both at the <keymap>'s own position. It returns the
// parsed selector of every keymap whose when parsed, B005 or not, as the
// row-level check keeps a row's own when: an id that names no node is a
// selector that matches nothing, never "no when", so dispatch, <hints>,
// and B007 see the effective when alike wherever it was written. A
// keymap whose when is V003, or that has no when=, is absent, so a map
// lookup gives nil (no selector), again as for a row's own V003 when.
func (b *builder) keymapWhens() map[*ir.Node]*css.Selector {
	out := map[*ir.Node]*css.Selector{}
	if !b.v3 || b.doc.Root == nil {
		return out
	}
	for _, c := range b.doc.Root.Children {
		if c.Tag != "keymap" {
			continue
		}
		w, ok := c.Attr("when")
		if !ok || w == "" {
			continue
		}
		sel, err := css.ParseSelectorLang(w, b.lang)
		if err == nil && sel.Root {
			err = fmt.Errorf(":root is not a node selector")
		}
		if err != nil {
			b.errN(c, "V003", "when=%q: %v", w, err)
			continue
		}
		for _, part := range sel.Parts {
			if part.ID != "" {
				if why := b.idTarget(part.ID); why != "" {
					b.errN(c, "B005", "when=%q refers to #%s, which %s", w, part.ID, why)
				}
			}
		}
		out[c] = &sel
	}
	return out
}
