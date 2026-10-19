package parse

import (
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// The form controls of SPEC v0.4b (phase F8): single="true" on a list or
// a table (§6.14.1) and the <checkbox> (§6.16). Both are current-language
// vocabulary only: in a document that writes version they are V001 and
// V002 with ir.VersionHintCurrent (checkTag, attr), checked no further.

// checkboxAttrs is the attribute row of <checkbox> (SPEC v0.4b §6.15):
// the common set without on:click, plus bind, label, and on:change. A
// checkbox fires on:change, never on:click (decision 45): §8.6 step 2
// gives enter and space to any focused node with on:click, which would
// make space both toggle and fire, and bring enter back.
func checkboxAttrs() map[string]bool {
	return without(with("bind", "label", "on:change"), "on:click")
}

// htmlTagHintsCurrent are the HTML-name hints of the current language
// (SPEC v0.4b decision 67): they replace htmlTagHints' entry for the same
// name in a <tui> without version. A document that writes version keeps
// 0.4.1's hint. <checkbox> is a tag of the current language, and the §5.1
// hint names it in a pinned document, so it has no HTML hint any more.
var htmlTagHintsCurrent = map[string]string{
	"radio": `a radio group is a <list> with checked= and single="true"`,
}

// htmlHint returns the HTML-name hint of tag in this document's language,
// and whether there is one.
func (b *builder) htmlHint(tag string) (string, bool) {
	if b.lang.V4() {
		if h, ok := htmlTagHintsCurrent[tag]; ok {
			return h, true
		}
	}
	h, ok := htmlTagHints[tag]
	return h, ok
}

// formNoContent reports whether n is a form control of the current
// language, which takes no element children and no text (SPEC v0.4b
// §6.16.1, decision 90): those are V016, as for the 0.2b leaves sparkline
// and hints, not V013. In a document that writes version the tag is
// unknown (V001) and checked no further, so its content is read as in
// 0.4.1.
func (b *builder) formNoContent(n *ir.Node) bool {
	return b.lang.V4() && n.Tag == "checkbox"
}

// singleAttr checks single= on a list or a table (SPEC v0.4b §6.14.1):
// the literal true or false (not a path: it decides the type of the
// stored value). Only the current language gets here; the version gate of
// attr comes first.
func (b *builder) singleAttr(n *ir.Node, a RawAttr) {
	switch a.Value {
	case "true":
		n.Single = true
	case "false":
	default:
		b.attrErr(n, a, "V003", "single=%q: want true or false", a.Value)
	}
}

// checkboxLabel checks a checkbox's label, a template like a <text> body
// (SPEC v0.4b §6.16.1, §7): a {…} pair that does not hold exactly a path
// is V003 at the attribute.
func (b *builder) checkboxLabel(n *ir.Node, a RawAttr, clean string) {
	if _, err := ir.ParseInterp(clean); err != nil {
		b.attrErr(n, a, "V003", "%s: %v", a.Name, err)
	}
}

// checkSingle reports single="true" without checked= on the same list or
// table (SPEC v0.4b §6.14.1, V018). single="false" is the same as no
// single, so it needs nothing.
func (b *builder) checkSingle(n *ir.Node, hasChecked bool) {
	if n.Single && !hasChecked {
		b.errN(n, "V018", "single on <%s> needs checked=: it makes checked hold one row key instead of an array", n.Tag)
	}
}
