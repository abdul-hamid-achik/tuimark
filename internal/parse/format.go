package parse

import (
	"fmt"
	"strings"
)

// Format returns the canonical formatting of a .tui document (`tuimark
// fmt`): 2-space indentation, one element per line, attributes reordered to
// id, class, the class:NAME guards in source order (SPEC §15.1), then the
// rest in source order, self-closing leaf elements, and <text>/<button>
// (and <column> cell template) bodies collapsed to one line when they fit. It works on
// the lossless tree from ParseXML, not the validated IR, so it formats any
// well-formed document regardless of catalog or binding errors.
//
// Format only requires well-formed XML (V005-free). If src is not
// well-formed, err is the *SyntaxError describing why and the string
// result is empty.
func Format(src []byte) (string, error) {
	root, prolog, epilog, err := ParseXML(src)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, c := range prolog {
		writeComment(&b, c, 0)
	}
	writeNode(&b, root, 0)
	for _, c := range epilog {
		writeComment(&b, c, 0)
	}
	return strings.TrimRight(b.String(), "\n") + "\n", nil
}

func writeComment(b *strings.Builder, c *RawNode, depth int) {
	b.WriteString(strings.Repeat("  ", depth))
	b.WriteString("<!--")
	b.WriteString(c.Text)
	b.WriteString("-->\n")
}

// hasRealContent reports whether n has any child that is not purely
// formatting whitespace: an element, a comment, or non-blank text.
func hasRealContent(n *RawNode) bool {
	for _, c := range n.Children {
		switch c.Type {
		case RawElement, RawComment:
			return true
		case RawText, RawCData:
			if strings.TrimSpace(c.Text) != "" {
				return true
			}
		}
	}
	return false
}

// onlySimpleText reports whether n's children are only text/CDATA (no
// elements or comments), the shape <text>/<button>/<style> need to use
// their compact body formatting.
func onlySimpleText(n *RawNode) bool {
	for _, c := range n.Children {
		if c.Type == RawElement || c.Type == RawComment {
			return false
		}
	}
	return true
}

// gatherText concatenates a node's text/CDATA children, in document order,
// the same way build.go's content() does before normalizing.
func gatherText(n *RawNode) string {
	var b strings.Builder
	for _, c := range n.Children {
		if c.Type == RawText || c.Type == RawCData {
			b.WriteString(c.Text)
		}
	}
	return b.String()
}

// canonicalAttrs orders attributes id, class, the class:NAME guards in
// source order, then the rest in source order.
func canonicalAttrs(n *RawNode) []RawAttr {
	var id, class *RawAttr
	var guards []RawAttr
	rest := make([]RawAttr, 0, len(n.Attrs))
	for i := range n.Attrs {
		a := n.Attrs[i]
		switch {
		case a.Name == "id":
			id = &a
		case a.Name == "class":
			class = &a
		case strings.HasPrefix(a.Name, ClassGuardPrefix):
			guards = append(guards, a)
		default:
			rest = append(rest, a)
		}
	}
	out := make([]RawAttr, 0, len(n.Attrs))
	if id != nil {
		out = append(out, *id)
	}
	if class != nil {
		out = append(out, *class)
	}
	out = append(out, guards...)
	return append(out, rest...)
}

var textEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
var attrEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", `"`, "&quot;")

// styleEscaper escapes only what XML well-formedness requires inside a
// <style> body: '&' and '<' (SPEC §5 forbids raw '&'/'<' in character
// data), plus a literal "]]>" (forbidden anywhere in character data,
// XML 1.0 §2.4). '>' is left alone, unlike textEscaper, so CSS child
// combinators (row > box) stay byte-for-byte verbatim.
var styleEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", "]]>", "]]&gt;")

func openTag(n *RawNode, depth int) string {
	var b strings.Builder
	b.WriteString(strings.Repeat("  ", depth))
	b.WriteByte('<')
	b.WriteString(n.Name)
	for _, a := range canonicalAttrs(n) {
		fmt.Fprintf(&b, ` %s="%s"`, a.Name, attrEscaper.Replace(a.Value))
	}
	return b.String()
}

// writeNode prints n at the given indentation depth (0 = document root).
func writeNode(b *strings.Builder, n *RawNode, depth int) {
	indent := strings.Repeat("  ", depth)
	open := openTag(n, depth)

	if (n.Name == "text" || n.Name == "button" || n.Name == "column") && onlySimpleText(n) {
		writeTextBody(b, n, depth, open)
		return
	}
	if n.Name == "style" && onlySimpleText(n) {
		writeStyleBody(b, n, depth, open)
		return
	}
	if !hasRealContent(n) {
		b.WriteString(open + "/>\n")
		return
	}
	b.WriteString(open + ">\n")
	for _, c := range n.Children {
		switch c.Type {
		case RawComment:
			writeComment(b, c, depth+1)
		case RawElement:
			writeNode(b, c, depth+1)
		case RawText, RawCData:
			writeLooseText(b, c.Text, depth+1)
		}
	}
	b.WriteString(indent + "</" + n.Name + ">\n")
}

// writeTextBody formats a <text>/<button> element: a single-line body
// stays inline, a multi-line body is indented one level, one line per line
// of text, empty bodies self-close.
func writeTextBody(b *strings.Builder, n *RawNode, depth int, open string) {
	indent := strings.Repeat("  ", depth)
	norm := NormalizeText(gatherText(n))
	if norm == "" {
		b.WriteString(open + "/>\n")
		return
	}
	if !strings.Contains(norm, "\n") {
		b.WriteString(open + ">" + textEscaper.Replace(norm) + "</" + n.Name + ">\n")
		return
	}
	b.WriteString(open + ">\n")
	cindent := indent + "  "
	for _, line := range strings.Split(norm, "\n") {
		if line == "" {
			b.WriteString("\n")
			continue
		}
		b.WriteString(cindent + textEscaper.Replace(line) + "\n")
	}
	b.WriteString(indent + "</" + n.Name + ">\n")
}

// writeStyleBody re-indents an inline <style> body one level deeper,
// otherwise verbatim: it dedents by the block's common leading whitespace
// and reapplies the new indent, keeping relative (nested) indentation and
// blank lines, but does not reflow or trim individual lines the way text
// bodies do.
func writeStyleBody(b *strings.Builder, n *RawNode, depth int, open string) {
	indent := strings.Repeat("  ", depth)
	body := gatherText(n)
	if strings.TrimSpace(body) == "" {
		b.WriteString(open + "/>\n")
		return
	}
	b.WriteString(open + ">\n")
	cindent := indent + "  "
	for _, line := range reindentBlock(body) {
		if line == "" {
			b.WriteString("\n")
			continue
		}
		b.WriteString(cindent + styleEscaper.Replace(line) + "\n")
	}
	b.WriteString(indent + "</" + n.Name + ">\n")
}

// reindentBlock trims leading/trailing blank lines and dedents by the
// minimum leading-whitespace count among the remaining non-blank lines.
func reindentBlock(body string) []string {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return nil
	}
	min := -1
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		n := len(l) - len(strings.TrimLeft(l, " \t"))
		if min < 0 || n < min {
			min = n
		}
	}
	if min < 0 {
		min = 0
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		if strings.TrimSpace(l) == "" {
			out[i] = ""
			continue
		}
		if len(l) >= min {
			l = l[min:]
		}
		out[i] = strings.TrimRight(l, " \t")
	}
	return out
}

// writeLooseText prints stray text found inside a non-leaf element (only
// possible in a document that is not IR-valid; V013 already flags this).
// It is rendered, not dropped, so fmt stays lossless for any well-formed
// input, one already-normalized line per output line.
func writeLooseText(b *strings.Builder, raw string, depth int) {
	norm := NormalizeText(raw)
	if norm == "" {
		return
	}
	indent := strings.Repeat("  ", depth)
	for _, line := range strings.Split(norm, "\n") {
		if line == "" {
			b.WriteString("\n")
			continue
		}
		b.WriteString(indent + textEscaper.Replace(line) + "\n")
	}
}
