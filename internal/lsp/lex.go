package lsp

import "strings"

// The lexers below read the text before the cursor and say what the cursor
// is in. They are tolerant on purpose: a buffer being typed is usually not
// well-formed XML (V005) or CSS, and completion must still work in it, so
// they never need the parsers' trees.

// tuiKind is what a cursor in a .tui is inside.
type tuiKind int

const (
	tuiNone      tuiKind = iota // a comment, a closing tag, a malformed spot
	tuiText                     // element content
	tuiTagName                  // after "<", in the tag name
	tuiAttrName                 // in a tag, where an attribute name goes
	tuiAttrValue                // inside the quotes of an attribute value
)

// openTag is an element still open at the cursor: its name and its
// attributes as written.
type openTag struct {
	name  string
	attrs map[string]string
}

// tuiCtx is the lexer's answer for a cursor.
type tuiCtx struct {
	kind tuiKind
	// tag and attr name the tag the cursor is in and, for a value, its
	// attribute.
	tag, attr string
	// prefix is what is typed so far: the tag or attribute name, or the
	// whole value text before the cursor.
	prefix string
	// start is the byte offset where prefix starts.
	start int
	// present are the attributes the tag already has.
	present map[string]bool
	// stack are the open ancestors of the cursor, outermost first.
	stack []openTag
}

func isNameByte(c byte) bool {
	return c == '_' || c == '-' || c == ':' || c == '.' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}

func isSpaceByte(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

// lexTUI reads text up to off.
func lexTUI(text string, off int) tuiCtx {
	src := text[:off]
	var stack []openTag
	i := 0   // scan position
	seg := 0 // start of the current text segment
	for i < len(src) {
		j := strings.IndexByte(src[i:], '<')
		if j < 0 {
			break
		}
		i += j
		rest := src[i:]
		skip := func(open, end string) bool { // a comment, CDATA, or PI
			if !strings.HasPrefix(rest, open) {
				return false
			}
			k := strings.Index(rest[len(open):], end)
			if k < 0 {
				i = len(src)
				seg = -1 // the cursor is inside it
			} else {
				i += len(open) + k + len(end)
				seg = i
			}
			return true
		}
		switch {
		case skip("<!--", "-->"), skip("<![CDATA[", "]]>"), skip("<?", "?>"):
			if seg < 0 {
				return tuiCtx{kind: tuiNone, stack: stack}
			}
			continue
		case strings.HasPrefix(rest, "<!"), strings.HasPrefix(rest, "</"):
			k := strings.IndexByte(rest, '>')
			if k < 0 {
				return tuiCtx{kind: tuiNone, stack: stack}
			}
			if rest[1] == '/' && len(stack) > 0 {
				name := strings.TrimSpace(rest[2:k])
				for n := len(stack) - 1; n >= 0; n-- {
					if stack[n].name == name {
						stack = stack[:n]
						break
					}
				}
			}
			i += k + 1
			seg = i
			continue
		}
		// An opening tag.
		k := i + 1
		for k < len(src) && isNameByte(src[k]) {
			k++
		}
		if k == len(src) {
			return tuiCtx{kind: tuiTagName, prefix: src[i+1 : k], start: i + 1, stack: stack}
		}
		tag := openTag{name: src[i+1 : k], attrs: map[string]string{}}
		present := map[string]bool{}
		p := k
		closed, abandoned := false, false
		for !closed && !abandoned {
			for p < len(src) && isSpaceByte(src[p]) {
				p++
			}
			if p >= len(src) {
				return tuiCtx{kind: tuiAttrName, tag: tag.name, start: p, present: present, stack: stack}
			}
			switch src[p] {
			case '<': // a tag left unfinished: the next one starts here
				abandoned = true
				continue
			case '>':
				stack = append(stack, tag)
				closed = true
				p++
				continue
			case '/':
				if p+1 < len(src) && src[p+1] == '>' {
					closed = true
					p += 2
				} else if p+1 >= len(src) {
					return tuiCtx{kind: tuiNone, stack: stack}
				} else {
					p++
				}
				continue
			}
			q := p
			for q < len(src) && isNameByte(src[q]) {
				q++
			}
			if q == p { // a stray character: skip it
				p++
				continue
			}
			if q == len(src) {
				return tuiCtx{kind: tuiAttrName, tag: tag.name, prefix: src[p:q], start: p, present: present, stack: stack}
			}
			name := src[p:q]
			present[name] = true
			r := q
			for r < len(src) && isSpaceByte(src[r]) {
				r++
			}
			if r >= len(src) || src[r] != '=' {
				p = r
				continue
			}
			r++
			for r < len(src) && isSpaceByte(src[r]) {
				r++
			}
			if r >= len(src) {
				return tuiCtx{kind: tuiNone, stack: stack}
			}
			if src[r] != '"' && src[r] != '\'' { // unquoted: not XML, skip it
				for r < len(src) && !isSpaceByte(src[r]) && src[r] != '>' && src[r] != '/' {
					r++
				}
				p = r
				continue
			}
			e := strings.IndexByte(src[r+1:], src[r])
			if e < 0 {
				return tuiCtx{kind: tuiAttrValue, tag: tag.name, attr: name, prefix: src[r+1:], start: r + 1, present: present, stack: stack}
			}
			tag.attrs[name] = src[r+1 : r+1+e]
			p = r + 1 + e + 1
		}
		i, seg = p, p
	}
	return tuiCtx{kind: tuiText, prefix: src[seg:], start: seg, stack: stack}
}

// cssKind is what a cursor in a .tcss is inside.
type cssKind int

const (
	cssNone     cssKind = iota // a comment, a string
	cssSelector                // before a rule's "{": a selector (or @media's)
	cssProperty                // in a rule, where a property name goes
	cssValue                   // in a rule, after a property's ":"
	cssMedia                   // inside the parentheses of an @media condition
)

// cssCtx is the lexer's answer for a cursor in a stylesheet.
type cssCtx struct {
	kind cssKind
	// prop is the property of a value; for cssMedia, the feature once its
	// ":" is typed.
	prop string
	// prefix is the statement text before the cursor: a selector, a
	// property name, or a value.
	prefix string
	// root is true inside a :root rule.
	root bool
}

// lexTCSS reads text up to off.
func lexTCSS(text string, off int) cssCtx {
	src := text[:off]
	type block struct{ kind, head string }
	var stack []block
	stmt := 0 // start of the current statement
	for i := 0; i < len(src); i++ {
		switch c := src[i]; c {
		case '/':
			if i+1 < len(src) && src[i+1] == '*' {
				k := strings.Index(src[i+2:], "*/")
				if k < 0 {
					return cssCtx{kind: cssNone}
				}
				i += 2 + k + 1
			}
		case '"', '\'':
			k := strings.IndexByte(src[i+1:], c)
			if k < 0 {
				return cssCtx{kind: cssNone}
			}
			i += 1 + k
		case '{':
			head := strings.TrimSpace(src[stmt:i])
			kind := "rule"
			if strings.HasPrefix(head, "@") {
				kind = "at"
			}
			stack = append(stack, block{kind, head})
			stmt = i + 1
		case '}':
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			stmt = i + 1
		case ';':
			stmt = i + 1
		}
	}
	cur := src[stmt:]
	if n := len(stack); n > 0 && stack[n-1].kind == "rule" {
		ctx := cssCtx{root: strings.Contains(stack[n-1].head, ":root")}
		if k := strings.IndexByte(cur, ':'); k >= 0 {
			ctx.kind, ctx.prop, ctx.prefix = cssValue, strings.TrimSpace(cur[:k]), cur[k+1:]
		} else {
			ctx.kind, ctx.prefix = cssProperty, strings.TrimSpace(cur)
		}
		return ctx
	}
	head := strings.TrimSpace(cur)
	if strings.HasPrefix(head, "@media") {
		if open := strings.LastIndexByte(cur, '('); open >= 0 && !strings.Contains(cur[open:], ")") {
			ctx := cssCtx{kind: cssMedia, prefix: strings.TrimSpace(cur[open+1:])}
			if k := strings.IndexByte(cur[open:], ':'); k >= 0 {
				ctx.prop = strings.TrimSpace(cur[open+1 : open+k])
				ctx.prefix = strings.TrimSpace(cur[open+k+1:])
			}
			return ctx
		}
	}
	return cssCtx{kind: cssSelector, prefix: cur}
}

// wordAt is the run of word characters around byte offset off in text, and
// where it starts and ends.
func wordAt(text string, off int) (word string, start, end int) {
	start, end = off, off
	for start > 0 && isWordByte(text[start-1]) {
		start--
	}
	for end < len(text) && isWordByte(text[end]) {
		end++
	}
	return text[start:end], start, end
}

// isWordByte is isWordRune on a byte: a non-ASCII byte is part of a word.
func isWordByte(c byte) bool {
	return c >= 0x80 || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || strings.IndexByte("_-:.#$@%", c) >= 0
}
