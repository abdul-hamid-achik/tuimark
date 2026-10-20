package parse

import (
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// The form controls of SPEC v0.4b: single="true" on a list or a table
// (§6.14.1) and the <checkbox> (§6.16) (phase F8), and the <select>
// (§6.17) (phase F9). All are current-language vocabulary only: in a
// document that writes version they are V001 and V002 with
// ir.VersionHintCurrent (checkTag, attr), checked no further.

// checkboxAttrs is the attribute row of <checkbox> (SPEC v0.4b §6.15):
// the common set without on:click, plus bind, label, and on:change. A
// checkbox fires on:change, never on:click (decision 45): §8.6 step 2
// gives enter and space to any focused node with on:click, which would
// make space both toggle and fire, and bring enter back.
func checkboxAttrs() map[string]bool {
	return without(with("bind", "label", "on:change"), "on:click")
}

// selectAttrs is the attribute row of <select> (SPEC v0.4b §6.15): the
// common set without on:click, plus bind, each, key, label, placeholder,
// and on:change. A focused select takes enter and space to open its
// option list (§8.6 step 2), so an on:click would never fire from the
// keyboard.
func selectAttrs() map[string]bool {
	return without(with("bind", "each", "key", "label", "placeholder", "on:change"), "on:click")
}

// htmlTagHintsCurrent are the HTML-name hints of the current language
// (SPEC v0.4b decision 67): they replace htmlTagHints' entry for the same
// name in a <tui> without version. A document that writes version keeps
// 0.4.1's hint. <checkbox> and <select> are tags of the current language,
// and the §5.1 hint names them in a pinned document, so they have no HTML
// hint any more. <component>, <slot>, and <import> name the composition
// of SPEC v0.4b §6.18 (the F2 review).
var htmlTagHintsCurrent = map[string]string{
	"radio":     `a radio group is a <list> with checked= and single="true"`,
	"option":    `use <select each=… key=… label=…>`,
	"component": `write it once in a <template id=…> and place it with <use template=…>`,
	"slot":      `a <use> takes no content: write the markup in a <template id=…> and place it with <use template=…>`,
	"import":    `use <include src=…> for markup, <style src=…> for a stylesheet`,
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
	return b.lang.V4() && (n.Tag == "checkbox" || n.Tag == "select")
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

// checkboxLabel checks a checkbox's label, or a select's, a template like
// a <text> body (SPEC v0.4b §6.16.1, §6.17.1, §7): a {…} pair that does
// not hold exactly a path is V003 at the attribute.
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

// checkSelect applies the structure rules of SPEC v0.4b §6.17.1 to a
// select: it needs each= and key= (V017 for each one that is absent; a
// malformed each is V011 at the attribute, not absent). A select has no
// B006: its bind holds the chosen option's key, which an index would not
// keep when the array changes (decision 50). Inside a list <item> or a
// container each template it is already V001 as a whole, so it gets no
// V017 there (nor V012, checkNode).
func (b *builder) checkSelect(n *ir.Node) {
	if listItem(n.Parent) != nil || b.eachContainer(n) != nil {
		return
	}
	if _, ok := n.Attr("each"); !ok {
		b.errN(n, "V017", "<select> needs each=\"path as alias\": its options are the elements of an array")
	}
	if _, ok := n.Attr("key"); !ok {
		b.errN(n, "V017", "<select> needs key=\"path\": bind holds the chosen option's key, which an index would not keep when the array changes")
	}
}
