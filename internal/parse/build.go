package parse

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
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
	Keys      []string
	KeysRaw   string
	Action    string
	When      string
	WhenSel   *css.Selector
	To        string // id without '#'
	Line, Col int
}

// Document is a parsed and validated .tui file.
type Document struct {
	File    string
	Spike   bool
	Version string
	Theme   string
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

// TagAttrs is the attribute catalog per tag (V002 outside it).
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

// Focusable-by-default kinds that therefore need an id (V012).
var needsID = map[string]bool{"list": true, "input": true, "button": true, "modal": true}

// Kinds that may not contain element children.
var leafKinds = map[string]bool{"text": true, "input": true, "button": true, "progress": true, "rule": true, "spacer": true, "style": true, "bind": true}

type builder struct {
	doc  *Document
	file string
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
	if !ir.IsKind(tag) {
		b.errN(n, "V001", "unknown tag <%s>", tag)
		return
	}
	ptag := ""
	if parent != nil {
		ptag = parent.Tag
	}
	switch tag {
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
		if ptag != "list" {
			b.errN(n, "V001", "<item> must be a direct child of <list>")
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
	}
	// SPEC §8.3 makes these focusable by default, but inside a list row
	// nothing is ever focused: the list is, and navigates its rows. Reject
	// them rather than render a widget that can never respond.
	if needsID[tag] {
		if it := listItem(parent); it != nil {
			b.errN(n, "V001", "<%s> inside a list <item> can never take focus: %s", tag, listHint(it))
		}
	}
}

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
	return ir.IsKind(tag)
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
	} else if allowed, known := TagAttrs[tag]; known {
		if itemFocusAttrs[name] {
			if it := listItem(n); it == n {
				b.attrErr(n, a, "V002", "attribute %q is not allowed on a list <item>: %s", name, listHint(it))
				return
			} else if it != nil && !needsID[tag] {
				// (An input/button/list/modal there is already V001 as a
				// whole in checkTag; its own attributes add nothing.)
				b.attrErr(n, a, "V002", "attribute %q is not allowed inside a list <item> (on <%s>): %s", name, tag, listHint(it))
				return
			}
		}
		if !allowed[name] {
			if strings.HasPrefix(name, "on:") && !eventNames[strings.TrimPrefix(name, "on:")] {
				b.attrErr(n, a, "V002", "unknown event %q on <%s>", name, tag)
			} else {
				b.attrErr(n, a, "V002", "attribute %q is not allowed on <%s>", name, tag)
			}
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
			n.Classes = append(n.Classes, c)
		}
	case "width", "height":
		if _, err := ir.ParseScalar(val); err != nil {
			b.attrErr(n, a, "V003", "%s: %v", name, err)
			return
		}
		n.Hints = append(n.Hints, ir.Prop{Name: name, Value: val, Line: a.Line, Col: a.Col})
	case "gap":
		if err := css.CheckDecl("gap", val); err != nil {
			b.attrErr(n, a, "V003", "%v", err)
			return
		}
		n.Hints = append(n.Hints, ir.Prop{Name: "gap", Value: val, Line: a.Line, Col: a.Col})
	case "pad":
		if err := css.CheckDecl("padding", val); err != nil {
			b.attrErr(n, a, "V003", "pad: %v", err)
			return
		}
		n.Hints = append(n.Hints, ir.Prop{Name: "padding", Value: val, Line: a.Line, Col: a.Col})
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
		n.Hints = append(n.Hints, ir.Prop{Name: "border", Value: v, Line: a.Line, Col: a.Col})
	case "wrap":
		if err := css.CheckDecl("wrap", val); err != nil {
			b.attrErr(n, a, "V003", "%v", err)
			return
		}
		n.Hints = append(n.Hints, ir.Prop{Name: "wrap", Value: val, Line: a.Line, Col: a.Col})
	case "style":
		// Recover per declaration (like a <style> rule's block), so one
		// typo does not also drop every other, valid declaration in the
		// same style="" attribute.
		decls, errs := css.ParseDeclsAll(val)
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
	case "title", "placeholder", "label":
		// XML attribute-value normalization: tabs and line breaks are spaces
		// (these values paint on one line).
		clean := b.sanitize(n, a.Line, a.Col, strings.Map(func(r rune) rune {
			if r == '\t' || r == '\n' || r == '\r' {
				return ' '
			}
			return r
		}, val))
		n.Attrs[name] = clean
		if name != "label" && !b.doc.Spike {
			if _, err := ir.ParseInterp(clean); err != nil {
				b.attrErr(n, a, "V003", "%s: %v", name, err)
			}
		} else if name == "label" && ir.HasInterp(clean) {
			b.attrErr(n, a, "V003", "label: {path} is only allowed in <text>, title, and placeholder")
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
		if val != "1" {
			b.attrErr(n, a, "V003", "version=%q: this runtime reads version=\"1\"", val)
		}
		b.doc.Version = val
	case "theme":
		if _, ok := css.Themes[val]; !ok {
			b.attrErr(n, a, "V003", "unknown theme %q (built-in: dark, light)", val)
			return
		}
		b.doc.Theme = val
	case "focus":
		if !strings.HasPrefix(val, "#") || !isIDName(val[1:]) {
			b.attrErr(n, a, "V003", "focus=%q: want #id", val)
		}
	case "keys", "action", "when", "to", "src":
		// validated with the keymap / stylesheet loader
	}
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
	case "text", "button":
		// Trim the source indentation (spaces or tabs) first, so only control
		// characters inside the text itself are V007.
		clean := b.sanitize(n, textLine, textCol, NormalizeText(body))
		n.Text = NormalizeText(clean)
		if n.Tag == "text" && !b.doc.Spike {
			if _, err := ir.ParseInterp(n.Text); err != nil {
				b.diag(ir.Error, "V003", textLine, textCol, n.Path, n.ID, "%v", err)
			}
		}
		if n.Tag == "button" && ir.HasInterp(n.Text) {
			b.errN(n, "V003", "button label: {path} is only allowed in <text>, title, and placeholder")
		}
	default:
		if !isXMLWhitespaceOnly(body) {
			b.diag(ir.Error, "V013", textLine, textCol, n.Path, n.ID, "text is not allowed directly inside <%s>; wrap it in <text>", n.Tag)
		}
	}
}

// checkStructure applies document-level rules that need the whole tree.
func (b *builder) checkStructure() {
	root := b.doc.Root
	if b.doc.Spike {
		b.walkAll(root, false, func(n *ir.Node, _ bool) { b.checkNode(n) })
		return
	}
	if _, ok := root.Attr("version"); !ok {
		b.errN(root, "V014", "<tui> needs version=\"1\"")
	}
	for _, c := range root.Children {
		if c.Tag == "screen" {
			b.doc.Screens = append(b.doc.Screens, c)
		}
		if c.Tag == "keymap" {
			for _, k := range c.Children {
				if k.Tag == "bind" {
					b.doc.Keymap = append(b.doc.Keymap, KeyBind{
						KeysRaw: k.Attrs["keys"], Action: k.Attrs["action"], When: k.Attrs["when"],
						To: k.Attrs["to"], Line: k.Line, Col: k.Col,
					})
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
	if listItem(n) != nil {
		focusable = false
	}
	if focusable && n.ID == "" {
		b.errN(n, "V012", "<%s> is focusable and needs an id", n.Tag)
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
		if prev, ok := seen[n.ID]; ok {
			b.errN(n, "V004", "duplicate id %q (first defined at %d:%d)", n.ID, prev.Line, prev.Col)
		} else {
			seen[n.ID] = n
		}
		if dropped {
			return
		}
		if _, ok := b.doc.IDs[n.ID]; !ok {
			b.doc.IDs[n.ID] = n
		}
		if listItem(n) == nil {
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
		return "is inside a list <item>, where nothing takes focus or is addressed by id: " + listHint(listItem(n))
	}
	return ""
}

// checkKeymap validates key tokens, actions, and selectors (V003/B005).
func (b *builder) checkKeymap() {
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
		if k.Action == "" {
			b.diag(ir.Error, "V003", k.Line, k.Col, path, "", "<bind> needs action=")
		} else if !ir.IsIdent(k.Action) {
			b.diag(ir.Error, "V003", k.Line, k.Col, path, "", "action=%q: an action is a plain name", k.Action)
		}
		if k.When != "" {
			sel, err := css.ParseSelector(k.When)
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
		if k.To != "" {
			if !strings.HasPrefix(k.To, "#") || !isIDName(k.To[1:]) {
				b.diag(ir.Error, "V003", k.Line, k.Col, path, "", "to=%q: want #id", k.To)
			} else {
				k.To = k.To[1:]
				if why := b.idTarget(k.To); why == idMissing {
					b.diag(ir.Error, "B005", k.Line, k.Col, path, "", "to=\"#%s\" refers to an id that does not exist", k.To)
				} else if why != "" {
					b.diag(ir.Error, "B005", k.Line, k.Col, path, "", "to=\"#%s\" %s", k.To, why)
				}
			}
		}
		if k.Action == "focus" && k.To == "" {
			b.diag(ir.Error, "V003", k.Line, k.Col, path, "", `action="focus" needs to="#id"`)
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
	}
}
