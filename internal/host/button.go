package host

import (
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// buttonText is the text a button shows (SPEC v0.4b §6.4.2): its label,
// even an empty one, else its body. In the current language either is a
// template resolved every frame in the button's scope, exactly like a
// <text> body (§7): a missing path is B003 and resolves to the empty
// string, and each LF that a resolved value brings becomes a space, since
// a button is one row. A body that is not shown is never resolved, so its
// paths report no B003. Line breaks written in a body keep their 0.4.1
// behavior. In a document that writes version the text is literal, as in
// 0.4.1.
func (fb *builder) buttonText(n *ir.Node, sc *scope) string {
	label, hasLabel := n.Attr("label")
	text := n.Text
	if hasLabel {
		text = label
	}
	if !fb.a.doc.Lang.V4() || fb.a.doc.Spike || text == "" {
		return text
	}
	segs, ok := fb.a.segsOf(text)
	if !ok {
		return text
	}
	var b strings.Builder
	for _, sg := range segs {
		if sg.Path == "" {
			b.WriteString(sg.Lit)
			continue
		}
		v, found := sc.resolve(fb.a.store, sg.Path)
		if !found {
			fb.missingBind(n, sg.Path, sc)
			continue
		}
		b.WriteString(oneLineLabel(cleanText(Format(v))))
	}
	return b.String()
}
