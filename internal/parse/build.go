package parse

import (
	"fmt"
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
}

// Document is a parsed and validated .tui file.
type Document struct {
	File    string
	Spike   bool
	Version string
	// V2 is true for a <tui version="2"> or <tui version="3"> document: it
	// accepts the 0.2b vocabulary (SPEC §5.1). Any other version is checked
	// as "1".
	V2 bool
	// V3 is true for a <tui version="3"> document (SPEC v0.3 §5.1): the
	// clipping warnings L008 and L009 fire, and the dump carries clipped.
	// Every version="2" rule also holds for it (V2 is true too).
	V3    bool
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

	// dropped holds the subtrees rooted at an unknown tag: validated, kept
	// out of Root (see builder.tagKnown), in document order.
	dropped []*ir.Node
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
			out = append(out, css.Decl{Prop: p.Name, Value: p.Value, Line: p.Line, Col: p.Col, File: file})
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
// scroll's stick and modal's focus sit on version="1" tags; keymap's when
// does too. (column's priority and sparkline's scale, the other ‡
// attributes, sit on version="2" tags and are added where that vocabulary
// is implemented.)
var TagAttrsV3 = map[string]map[string]bool{
	"scroll": only("stick"),
	"modal":  only("focus"),
	"keymap": only("when"),
}

// tableItemAttrs are the attributes of a table's row template <item>
// (SPEC §6.9.1, §6.15), besides class:NAME.
var tableItemAttrs = only("class")

// noClassGuard are the tags that take no class:NAME (SPEC §6.13).
var noClassGuard = map[string]bool{"tui": true, "style": true, "keymap": true, "bind": true}

// ClassGuardPrefix starts every class:NAME attribute (SPEC §6.13).
const ClassGuardPrefix = "class:"

// attrAllowed reports whether a tag takes an attribute, and the lowest
// document version that accepts it there: 1 (or 0, equivalently, for a
// version="1" tag's own attributes), 2, or 3 (SPEC §6.15). parent is the
// element's parent tag.
func attrAllowed(tag, parent, name string) (ok bool, need int) {
	if strings.HasPrefix(name, ClassGuardPrefix) {
		return !noClassGuard[tag], 2
	}
	if tag == "item" && parent == "table" {
		return tableItemAttrs[name], 2
	}
	if ir.IsKindV2(tag) {
		if TagAttrsV2[tag][name] {
			return true, 2
		}
		return TagAttrsV3[tag][name], 3
	}
	if TagAttrs[tag][name] {
		return true, 1
	}
	if TagAttrsV2[tag][name] {
		return true, 2
	}
	return TagAttrsV3[tag][name], 3
}

// Focusable-by-default kinds that therefore need an id (V012).
var needsID = map[string]bool{"list": true, "input": true, "button": true, "modal": true}

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
	// droppedIn maps a node to the unknown-tag children content() kept
	// out of its Children, in document order.
	droppedIn map[*ir.Node][]*ir.Node
	// addressable holds the ids of tree nodes outside any list <item>:
	// the ones screen focus= and keymap to=/when= can name.
	addressable map[string]bool
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

// listHint is the advice given for a focus target inside a list row.
func listHint(item *ir.Node) string {
	sel := "list"
	if item != nil && item.Parent != nil && item.Parent.ID != "" {
		sel = "#" + item.Parent.ID
	}
	return fmt.Sprintf(`list rows are navigated by their list; handle the row with the list's on:select, or a keymap <bind keys="..." action="..." when="%s:focus"/> (its event identifies the selected row)`, sel)
}

func (b *builder) diag(sev, code string, line, col int, path, id, format string, args ...any) {
	b.doc.Diags = append(b.doc.Diags, ir.Diagnostic{
		Severity: sev, Code: code, Msg: fmt.Sprintf(format, args...),
		File: b.file, Line: line, Col: col, Path: path, ID: id,
	})
}

func (b *builder) errN(n *ir.Node, code, format string, args ...any) {
	b.diag(ir.Error, code, n.Line, n.Col, n.Path, n.ID, format, args...)
}

func (b *builder) warnN(n *ir.Node, code, format string, args ...any) {
	b.diag(ir.Warning, code, n.Line, n.Col, n.Path, n.ID, format, args...)
}

// Parse parses and validates a .tui document. file is used in diagnostics
// and to resolve relative style src paths (it may be empty).
func Parse(src []byte, file string) *Document {
	doc := &Document{File: file, IDs: map[string]*ir.Node{}}
	b := &builder{doc: doc, file: filepath.Base(file)}
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
	case "tui":
		// The version decides every check below (SPEC §5.1), whatever the
		// attribute order: <tui theme="auto" version="2"> is valid. A
		// version="3" document accepts everything version="2" does (v0.3):
		// it sets v2 too, plus its own v3 flag.
		for _, a := range raw.Attrs {
			if a.Name != "version" {
				continue
			}
			switch a.Value {
			case "2":
				b.v2 = true
			case "3":
				b.v2, b.v3 = true, true
			}
		}
		doc.V2, doc.V3 = b.v2, b.v3
	default:
		b.diag(ir.Error, "V005", raw.Line, raw.Col, "/", "", "document root must be <tui> (or the phase-0 alias <app>), got <%s>", raw.Name)
		return doc
	}
	doc.Root = b.node(raw, nil)
	b.checkStructure()
	b.checkIDs(doc.Root)
	b.checkKeymap()
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
	n := &ir.Node{
		Tag: r.Name, Kind: r.Name, Parent: parent,
		Attrs: map[string]string{}, On: map[string]string{},
		Line: r.Line, Col: r.Col, Path: childPath(parent, r.Name),
	}
	if r.Name == "app" {
		n.Kind = "col"
	}
	for _, a := range r.Attrs {
		n.Attrs[a.Name] = a.Value
		n.Order = append(n.Order, a.Name)
	}
	if id, ok := n.Attrs["id"]; ok {
		n.ID = id
	}
	b.checkTag(n, parent)
	for _, a := range r.Attrs {
		b.attr(n, a)
	}
	b.content(n, r)
	return n
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
		b.errN(n, "V001", "unknown tag <%s> (tags are lowercase)", tag)
		return
	}
	if strings.Contains(tag, ":") {
		b.errN(n, "V001", "unknown tag <%s> (namespaces are not allowed)", tag)
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
				b.errN(n, "V001", "<%s> is not in the phase-0 vocabulary (app col row box text); use a <tui> root for v1 tags", tag)
			} else {
				b.errN(n, "V001", "unknown tag <%s>", tag)
			}
			return
		}
		if tag == "app" && parent != nil {
			b.errN(n, "V001", "<app> is only valid as the document root")
		}
		return
	}
	if tag == "app" {
		b.errN(n, "V001", "<app> is the phase-0 root alias; inside <tui> use <screen>")
		return
	}
	if ir.IsKindV2(tag) && !b.v2 {
		b.v2Hint(n.Line, n.Col, n, "V001", "tag <%s>", tag)
		return
	}
	if !ir.IsKindIn(tag, b.v2) {
		b.errN(n, "V001", "unknown tag <%s>", tag)
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
			b.errN(n, "V001", "<%s> inside a list <item> can never take focus: %s", tag, listHint(it))
		} else if c := b.eachContainer(n); c != nil {
			// SPEC §6.8 item 8: the template is repeated per element, so
			// nothing in it may take focus or keep per-node state.
			b.errN(n, "V001", "<%s> inside an each template (%s) would repeat for every element and can never take focus or keep its state: %s", tag, describeNode(c), templateHint)
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
	if n.ID != "" {
		return fmt.Sprintf("<%s id=%q>", n.Tag, n.ID)
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
	if b.doc.Spike {
		for _, k := range ir.SpikeKinds {
			if k == tag {
				return true
			}
		}
		return false
	}
	return ir.IsKindIn(tag, b.v2)
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

func (b *builder) attr(n *ir.Node, a RawAttr) {
	name, val := a.Name, a.Value
	tag := n.Tag
	if b.doc.Spike {
		ok := false
		for _, s := range ir.SpikeAttrs {
			if s == name {
				ok = true
			}
		}
		if !ok {
			b.attrErr(n, a, "V002", "unknown attribute %q on <%s> (phase-0 attributes: id class width height gap pad border)", name, tag)
			return
		}
	} else if b.tagKnown(tag) {
		ptag := ""
		if n.Parent != nil && (b.v2 || n.Parent.Tag != "table") {
			ptag = n.Parent.Tag
		}
		allowed, need := attrAllowed(tag, ptag, name)
		if itemFocusAttrs[name] {
			if it := listItem(n); it == n {
				b.attrErr(n, a, "V002", "attribute %q is not allowed on a list <item>: %s", name, listHint(it))
				return
			} else if it != nil && !needsID[tag] {
				// (An input/button/list/modal there is already V001 as a
				// whole in checkTag; its own attributes add nothing.)
				b.attrErr(n, a, "V002", "attribute %q is not allowed inside a list <item> (on <%s>): %s", name, tag, listHint(it))
				return
			} else if c := b.eachContainer(n); it == nil && c != nil && !needsID[tag] && !statefulV2[tag] {
				// SPEC §6.8 item 8, at any depth of a container template.
				b.attrErr(n, a, "V002", "attribute %q is not allowed inside an each template (on <%s>, in %s): %s", name, tag, describeNode(c), templateHint)
				return
			}
		}
		if name == "stick" {
			// SPEC v0.3b §6.15: stick needs an identity across frames that a
			// repeated row does not have.
			if it := listItem(n); it != nil {
				b.attrErr(n, a, "V002", "attribute %q is not allowed inside a list <item>: its state needs an identity across frames that a repeated row does not have", name)
				return
			} else if c := b.eachContainer(n); c != nil {
				b.attrErr(n, a, "V002", "attribute %q is not allowed inside an each template (in %s): its state needs an identity across frames that a repeated row does not have", name, describeNode(c))
				return
			}
		}
		if !allowed {
			if strings.HasPrefix(name, "on:") && !eventNames[strings.TrimPrefix(name, "on:")] {
				b.attrErr(n, a, "V002", "unknown event %q on <%s>", name, tag)
			} else {
				b.attrErr(n, a, "V002", "attribute %q is not allowed on <%s>", name, tag)
			}
			return
		}
		// A version="3" item is checked against version="3" first, whether
		// the document is version="1" or version="2" (both get the same
		// hint); only an attribute that needs exactly version="2" falls to
		// the version="2" hint (SPEC v0.3b §5.1).
		if need >= 3 && !b.v3 {
			b.v3Hint(a.Line, a.Col, n, "V002", "attribute %q on <%s>", name, tag)
			return
		}
		if need >= 2 && !b.v2 {
			b.v2Hint(a.Line, a.Col, n, "V002", "attribute %q on <%s>", name, tag)
			return
		}
	} else {
		return // unknown tag already reported
	}
	if hasUpper(name) {
		b.attrErr(n, a, "V002", "unknown attribute %q (attribute names are lowercase)", name)
		return
	}
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
		if err := css.CheckDecl("wrap", val); err != nil {
			b.attrErr(n, a, "V003", "%v", err)
			return
		}
		n.Hints = append(n.Hints, ir.Prop{Name: "wrap", Value: val, Line: a.Line, Col: a.Col, Attr: attrText(a)})
	case "style":
		// Recover per declaration (like a <style> rule's block), so one
		// typo does not also drop every other, valid declaration in the
		// same style="" attribute.
		decls, errs := css.ParseDeclsAllIn(val, b.v2)
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
			if b.v2 {
				b.attrErr(n, a, "V003", "unknown theme %q (built-in: dark, light; or auto)", val)
			} else {
				b.attrErr(n, a, "V003", "unknown theme %q (built-in: dark, light)", val)
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
			if n.Tag == "text" {
				b.diag(ir.Error, "V013", c.Line, c.Col, n.Path, n.ID, "<text> cannot contain elements (found <%s>)", c.Name)
				continue
			}
			// SPEC §6.9.1: a column's body is its cell template (text
			// only); a table's <item> is the row template, with no
			// children.
			if b.v2 && n.Tag == "column" {
				b.diag(ir.Error, "V016", c.Line, c.Col, n.Path, n.ID, "<column> holds its cell template as text, with {path} for the row's values; it cannot contain elements (found <%s>)", c.Name)
				continue
			}
			if b.tableItem(n) {
				b.diag(ir.Error, "V016", c.Line, c.Col, n.Path, n.ID, "a table's <item> is the row template: it takes class and class:NAME only, no children (found <%s>); the cells come from the <column> elements", c.Name)
				continue
			}
			if b.v2 && (n.Tag == "sparkline" || n.Tag == "hints") {
				// SPEC §6.11, §6.12: no children (a hints' items come
				// from the keymap).
				b.diag(ir.Error, "V016", c.Line, c.Col, n.Path, n.ID, "<%s> takes no children (found <%s>)", n.Tag, c.Name)
				continue
			}
			if leafKinds[n.Tag] {
				b.diag(ir.Error, "V013", c.Line, c.Col, n.Path, n.ID, "<%s> cannot contain elements (found <%s>)", n.Tag, c.Name)
				continue
			}
			child := b.node(c, n)
			// An unknown kind still gets its subtree validated (V001 plus
			// whatever its own children report), but it is dropped here so
			// it never reaches layout, paint, dump nodes, or `tuimark ir`.
			// The tree-level checks (V004, V012, V013, B006) still visit
			// it through walkAll.
			if b.tagKnown(c.Name) {
				n.Children = append(n.Children, child)
			} else {
				b.doc.dropped = append(b.doc.dropped, child)
				if b.droppedIn == nil {
					b.droppedIn = map[*ir.Node][]*ir.Node{}
				}
				b.droppedIn[n] = append(b.droppedIn[n], child)
			}
		}
	}
	body := text.String()
	switch n.Tag {
	case "style":
		n.Text = body
		b.doc.Styles = append(b.doc.Styles, StyleRef{Src: n.Attrs["src"], Body: body, Line: n.Line, Col: n.Col, BodyLine: textLine, BodyCol: textCol})
		if n.Attrs["src"] != "" && !isXMLWhitespaceOnly(body) {
			b.errN(n, "V013", "<style> takes either src or an inline body, not both")
		}
	case "text", "button", "column":
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
		if n.Tag == "button" && ir.HasInterp(n.Text) {
			b.errN(n, "V003", "button label: {path} is only allowed in <text>, title, and placeholder")
		}
	default:
		if isXMLWhitespaceOnly(body) {
			break
		}
		switch {
		case b.v2 && n.Tag == "table":
			b.diag(ir.Error, "V016", textLine, textCol, n.Path, n.ID, "text is not allowed directly inside <table>; a column's cell template is the body of its <column>")
		case b.v2 && n.Tag == "tabs":
			b.diag(ir.Error, "V016", textLine, textCol, n.Path, n.ID, "text is not allowed directly inside <tabs>; put it inside a <tab>, and name the tab with label=")
		case b.v2 && (n.Tag == "sparkline" || n.Tag == "hints"):
			b.diag(ir.Error, "V016", textLine, textCol, n.Path, n.ID, "<%s> takes no text", n.Tag)
		case b.tableItem(n):
			b.diag(ir.Error, "V016", textLine, textCol, n.Path, n.ID, "a table's <item> is the row template and takes no text; the cells come from the <column> elements")
		default:
			b.diag(ir.Error, "V013", textLine, textCol, n.Path, n.ID, "text is not allowed directly inside <%s>; wrap it in <text>", n.Tag)
		}
	}
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
	if _, ok := root.Attr("version"); !ok {
		b.errN(root, "V014", "<tui> needs version=\"1\" (or version=\"2\")")
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
						To: k.Attrs["to"], Line: k.Line, Col: k.Col,
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
	if b.tagKnown(n.Tag) {
		fn(n, dropped)
	}
	kids := n.Children
	if extra := b.droppedIn[n]; len(extra) > 0 {
		kids = make([]*ir.Node, 0, len(n.Children)+len(extra))
		i, j := 0, 0
		for i < len(n.Children) || j < len(extra) {
			if j == len(extra) || (i < len(n.Children) && before(n.Children[i], extra[j])) {
				kids = append(kids, n.Children[i])
				i++
			} else {
				kids = append(kids, extra[j])
				j++
			}
		}
	}
	for _, c := range kids {
		b.walkAll(c, dropped || !b.tagKnown(c.Tag), fn)
	}
}

// before reports whether a starts before b in the source.
func before(a, b *ir.Node) bool {
	if a.Line != b.Line {
		return a.Line < b.Line
	}
	return a.Col < b.Col
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
	if focusable && n.ID == "" && !(b.v2 && (n.Tag == "table" || n.Tag == "tabs")) {
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
		case "sparkline":
			// SPEC §6.11: bind is required.
			if _, ok := n.Attr("bind"); !ok {
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
			if _, ok := n.Attr("key"); !ok {
				b.warnN(n, "B006", "<list each=%q> has no key=; selection follows the index", n.Each)
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
	if n.ID == "" && !nested {
		b.errN(n, "V012", "<table> needs an id: its cursor and scroll offset are kept by it, and to=\"#id\" names it")
	}
	eachAttr, hasEach := n.Attr("each")
	if !hasEach {
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
	if _, ok := n.Attr("key"); n.Each != "" && !ok && !nested {
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
	if n.ID == "" && !nested {
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
			if why := b.idTarget(id); why == idMissing {
				b.errN(n, "B005", "focus=%q refers to an id that does not exist", f)
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
	if n.ID == "" {
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
			if why := b.idTarget(id); why == idMissing {
				b.errN(n, "B005", "focus=%q refers to an id that does not exist", f)
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
	e, err := ir.ParseEach(n.Each)
	if err != nil {
		return
	}
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Each == "" {
			continue
		}
		if pe, err := ir.ParseEach(p.Each); err == nil && pe.Alias == e.Alias {
			b.errN(n, "V011", "each=%q: the alias %q repeats the alias of the enclosing each=%q of %s; pick another name", n.Each, e.Alias, p.Each, describeNode(p))
			return
		}
	}
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
	_, hasChecked := n.Attr("checked")
	if hasChecked && n.Tag != "tabs" {
		_, hasEach := n.Attr("each")
		_, hasKey := n.Attr("key")
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
	b.walkAll(root, false, func(n *ir.Node, dropped bool) {
		if n.ID == "" {
			return
		}
		inTemplate := b.eachContainer(n) != nil
		if prev, ok := seen[n.ID]; ok {
			b.errN(n, "V004", "duplicate id %q (first defined at %d:%d)", n.ID, prev.Line, prev.Col)
		} else {
			seen[n.ID] = n
			if inTemplate {
				// SPEC §6.8 item 8: the template is inflated once per
				// element, so its id would repeat.
				b.errN(n, "V004", "id %q inside an each template (%s) would repeat for every element: %s", n.ID, describeNode(b.eachContainer(n)), templateHint)
			}
		}
		if dropped {
			return
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
func (b *builder) idTarget(id string) string {
	n, ok := b.doc.IDs[id]
	if !ok {
		return idMissing
	}
	if !b.addressable[id] {
		if it := listItem(n); it != nil {
			return "is inside a list <item>, where nothing takes focus or is addressed by id: " + listHint(it)
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
		b.diag(ir.Error, "V003", k.Line, k.Col, path, "", `action="switch-to" needs to="#tab" or to="#screen"`)
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
	if why := BuiltinIncompatible(k.Action, n.Kind, hasAttr(n, "checked")); why != "" {
		b.diag(ir.Error, "B007", k.Line, k.Col, path, "", "action=%q: its target #%s (from %s) is a <%s>, %s", k.Action, target, how, n.Tag, why)
	}
}

func hasAttr(n *ir.Node, name string) bool {
	_, ok := n.Attr(name)
	return ok
}

// BuiltinIncompatible returns why a node of the given kind can never be
// the target of the hyphenated built-in action (SPEC §8.4 "Static
// checks"), or "" when data or state may make it compatible. checked
// tells whether the node has a checked= attribute.
func BuiltinIncompatible(action, kind string, checked bool) string {
	switch action {
	case "move-next", "move-prev", "move-first", "move-last", "move-page-down", "move-page-up":
		switch kind {
		case "text", "rule", "spacer", "input", "button", "progress", "sparkline", "hints", "column", "tab", "item":
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
			b.diag(ir.Error, "V003", k.Line, k.Col, path, "", "<bind> needs keys=")
		}
		k.Keys = ir.SplitKeys(k.KeysRaw)
		for _, tok := range k.Keys {
			if !ir.ValidKey(tok) {
				b.diag(ir.Error, "V003", k.Line, k.Col, path, "", "unknown key %q (see the key token list: a-z 0-9 enter esc tab backspace space arrows home end pgup pgdn ctrl+x shift+tab)", tok)
			}
		}
		switch {
		case k.Action == "":
			b.diag(ir.Error, "V003", k.Line, k.Col, path, "", "<bind> needs action=")
		case ir.IsBuiltinActionV2(k.Action):
			// SPEC §8.4: the hyphenated built-ins are version="2" keymap
			// actions (ADR 0006).
			if !b.v2 {
				b.diag(ir.Error, "V003", k.Line, k.Col, path, "", "action=%q: built-in action%s", k.Action, ir.VersionHint)
			}
		case !ir.IsIdent(k.Action):
			b.diag(ir.Error, "V003", k.Line, k.Col, path, "", "action=%q: an action is a plain name", k.Action)
		}
		switch {
		case k.HasOwnWhen:
			if k.When != "" {
				sel, err := css.ParseSelectorIn(k.When, b.v2)
				if err == nil && sel.Root {
					err = fmt.Errorf(":root is not a node selector")
				}
				if err != nil {
					b.diag(ir.Error, "V003", k.Line, k.Col, path, "", "when=%q: %v", k.When, err)
				} else {
					k.WhenSel = &sel
					for _, part := range sel.Parts {
						if part.ID != "" {
							if why := b.idTarget(part.ID); why != "" {
								b.diag(ir.Error, "B005", k.Line, k.Col, path, "", "when=%q refers to #%s, which %s", k.When, part.ID, why)
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
				b.diag(ir.Error, "V003", k.Line, k.Col, path, "", "to=%q: want #id", k.To)
			} else {
				k.To = k.To[1:]
				if why := b.idTarget(k.To); why == idMissing {
					b.diag(ir.Error, "B005", k.Line, k.Col, path, "", "to=\"#%s\" refers to an id that does not exist", k.To)
				} else if why != "" {
					b.diag(ir.Error, "B005", k.Line, k.Col, path, "", "to=\"#%s\" %s", k.To, why)
				} else {
					toOK = true
				}
			}
		}
		if k.Action == "focus" && k.To == "" {
			b.diag(ir.Error, "V003", k.Line, k.Col, path, "", `action="focus" needs to="#id"`)
		}
		if b.v2 && ir.IsBuiltinActionV2(k.Action) {
			b.checkBuiltinRow(k, path, toOK)
		}
	}
	for _, s := range b.doc.Screens {
		if f, ok := s.Attr("focus"); ok && strings.HasPrefix(f, "#") {
			if why := b.idTarget(f[1:]); why == idMissing {
				b.errN(s, "B005", "focus=%q refers to an id that does not exist", f)
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
// parsed selector of the keymaps whose when validated clean (nil, so a
// map lookup for any other keymap, or one with no when=, correctly gives
// nil: no selector).
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
		sel, err := css.ParseSelectorIn(w, b.v2)
		if err == nil && sel.Root {
			err = fmt.Errorf(":root is not a node selector")
		}
		if err != nil {
			b.errN(c, "V003", "when=%q: %v", w, err)
			continue
		}
		clean := true
		for _, part := range sel.Parts {
			if part.ID != "" {
				if why := b.idTarget(part.ID); why != "" {
					b.errN(c, "B005", "when=%q refers to #%s, which %s", w, part.ID, why)
					clean = false
				}
			}
		}
		if clean {
			out[c] = &sel
		}
	}
	return out
}
