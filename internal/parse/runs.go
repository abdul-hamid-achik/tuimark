package parse

import (
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// Text runs (SPEC v0.4d §6.20): <text runs="path"/> paints an array of
// runs the host built. The attribute and the rules of its body belong to
// the current language only: in a document that writes version, runs is
// V002 with ir.VersionHintCurrent (attrGate), and the text is a plain text.

// msgRunsBody is the V013 of body text on a <text runs> (decision 207).
const msgRunsBody = "<text runs> takes no body text: its text comes from the runs"

// runsAttr checks runs= on a <text> (SPEC v0.4d §6.20.1): a path, resolved
// every frame in the text's scope as bind= is. A value that is not a path,
// a {path} or a {$name} included, is V003 and the text paints nothing from
// it (a {$name} is reported by props.go before it gets here).
func (b *builder) runsAttr(n *ir.Node, a RawAttr) {
	if !ir.IsPath(a.Value) {
		b.attrErr(n, a, "V003", "runs=%q is not a path", a.Value)
		return
	}
	n.Runs = a.Value
}

// wroteRuns reports whether n is a <text> of the current language that
// writes runs=, valid or not: its body is not its text then.
func (b *builder) wroteRuns(n *ir.Node) bool {
	if n.Tag != "text" || !b.lang.V4() {
		return false
	}
	_, ok := n.Attrs["runs"]
	return ok
}

// runsBody handles the body of n when n is a <text runs> (SPEC v0.4d
// §6.20.1, decision 207) and reports whether it did: character data other
// than whitespace is V013 at the <text>, and the body is ignored either
// way, so n.Text stays empty. An element child keeps its own V013
// (noElements) and is not part of body.
func (b *builder) runsBody(n *ir.Node, body string) bool {
	if !b.wroteRuns(n) {
		return false
	}
	if !isXMLWhitespaceOnly(body) {
		b.errSelf(n, "V013", msgRunsBody)
	}
	n.Text = ""
	return true
}
