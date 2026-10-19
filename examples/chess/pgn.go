package main

import (
	"fmt"
	"strings"
)

// The Seven Tag Roster, in the order PGN wants them.
var pgnRoster = []string{"Event", "Site", "Date", "Round", "White", "Black", "Result"}

// PGN writes the game in portable game notation: the Seven Tag Roster (and
// SetUp and FEN when the game did not start from the standard position),
// then the moves, wrapped at 80 columns, and the result.
func (g *Game) PGN() string { return g.pgnText(80) }

// pgnText writes the PGN with the movetext wrapped at width columns, or on
// one line when width is 0 (the dialog wraps it to its own width).
func (g *Game) pgnText(width int) string {
	tags := map[string]string{
		"Event": "Casual game", "Site": "tuimark chess", "Date": "????.??.??",
		"Round": "-", "White": "?", "Black": "?",
	}
	for k, v := range g.Tags {
		tags[k] = v
	}
	tags["Result"] = g.Result
	standard := g.Start.FEN() == startFEN
	if !standard {
		tags["SetUp"] = "1"
		tags["FEN"] = g.Start.FEN()
	}
	var b strings.Builder
	order := append([]string(nil), pgnRoster...)
	if !standard {
		order = append(order, "SetUp", "FEN")
	}
	seen := map[string]bool{}
	for _, k := range order {
		seen[k] = true
		fmt.Fprintf(&b, "[%s \"%s\"]\n", k, pgnEscape(tags[k]))
	}
	extra := make([]string, 0, len(tags))
	for k := range tags {
		if !seen[k] {
			extra = append(extra, k)
		}
	}
	sortStrings(extra)
	for _, k := range extra {
		fmt.Fprintf(&b, "[%s \"%s\"]\n", k, pgnEscape(tags[k]))
	}
	b.WriteByte('\n')

	var toks []string
	num := g.Start.Full
	black := g.Start.Turn == Black
	for i, san := range g.SAN {
		switch {
		case !black:
			toks = append(toks, fmt.Sprintf("%d.", num), san)
		case i == 0:
			toks = append(toks, fmt.Sprintf("%d...", num), san)
		default:
			toks = append(toks, san)
		}
		if black {
			num++
		}
		black = !black
	}
	toks = append(toks, g.Result)
	line := 0
	for _, t := range toks {
		if width > 0 && line > 0 && line+1+len(t) > width {
			b.WriteByte('\n')
			line = 0
		}
		if line > 0 {
			b.WriteByte(' ')
			line++
		}
		b.WriteString(t)
		line += len(t)
	}
	b.WriteByte('\n')
	return b.String()
}

func pgnEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `"`, `\"`)
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// ParsePGN reads the first game of a PGN text. Comments ({…} and ;…),
// numeric annotation glyphs ($n), and variations ((…), nested too) are
// skipped; the FEN tag sets the starting position. A move that is not legal
// is an error naming it and its move number.
func ParsePGN(text string) (*Game, error) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	tags := map[string]string{}
	var order []string
	i := 0
	for ; i < len(lines); i++ {
		l := strings.TrimSpace(lines[i])
		if l == "" || strings.HasPrefix(l, "%") {
			continue
		}
		if !strings.HasPrefix(l, "[") {
			break
		}
		k, v, ok := parseTag(l)
		if !ok {
			return nil, fmt.Errorf("PGN line %d: bad tag %q", i+1, l)
		}
		tags[k] = v
		order = append(order, k)
	}
	// The movetext runs to a result token, or to the next game's tags.
	var mt strings.Builder
	for ; i < len(lines); i++ {
		l := lines[i]
		if strings.HasPrefix(strings.TrimSpace(l), "[") && strings.TrimSpace(mt.String()) != "" {
			break
		}
		if strings.HasPrefix(l, "%") {
			continue
		}
		mt.WriteString(l)
		mt.WriteByte('\n')
	}
	start := StartPosition()
	if fen, ok := tags["FEN"]; ok {
		p, err := ParseFEN(fen)
		if err != nil {
			return nil, fmt.Errorf("PGN FEN tag: %w", err)
		}
		start = p
	}
	g := NewGame(start)
	for _, k := range order {
		if k != "FEN" && k != "SetUp" && k != "Result" {
			g.Tags[k] = tags[k]
		}
	}
	toks, err := pgnTokens(mt.String())
	if err != nil {
		return nil, err
	}
	result := ""
	n := 0
	for _, t := range toks {
		switch t {
		case "1-0", "0-1", "1/2-1/2", "*":
			result = t
		}
		if result != "" {
			break
		}
		n++
		m, err := ParseMove(g.Cur(), t)
		if err != nil {
			return nil, fmt.Errorf("PGN move %d (%s): %w", n, t, err)
		}
		g.push(m, [2]int64{})
	}
	if result == "" {
		result = tags["Result"]
	}
	if result == "1-0" || result == "0-1" || result == "1/2-1/2" {
		if !g.Over() {
			g.Result, g.Reason = result, "per the PGN"
		}
	}
	return g, nil
}

// parseTag reads [Name "value"].
func parseTag(l string) (string, string, bool) {
	if !strings.HasSuffix(l, "]") {
		return "", "", false
	}
	body := strings.TrimSpace(l[1 : len(l)-1])
	sp := strings.IndexAny(body, " \t")
	if sp < 0 {
		return "", "", false
	}
	name := body[:sp]
	val := strings.TrimSpace(body[sp:])
	if len(val) < 2 || val[0] != '"' || val[len(val)-1] != '"' {
		return "", "", false
	}
	val = val[1 : len(val)-1]
	val = strings.ReplaceAll(val, `\"`, `"`)
	val = strings.ReplaceAll(val, `\\`, `\`)
	return name, val, true
}

// pgnTokens splits movetext into move tokens and the result, dropping
// comments, variations, NAGs, and move numbers.
func pgnTokens(s string) ([]string, error) {
	var toks []string
	var cur strings.Builder
	flush := func() {
		t := cur.String()
		cur.Reset()
		if t == "" {
			return
		}
		if strings.HasPrefix(t, "$") || strings.Trim(t, "!?") == "" || t == "e.p." {
			return
		}
		t = strings.TrimSuffix(t, "e.p.")
		// "12." and "12..." are move numbers, "12.e4" a number glued to a move.
		j := 0
		for j < len(t) && t[j] >= '0' && t[j] <= '9' {
			j++
		}
		if j > 0 && j < len(t) && t[j] == '.' {
			for j < len(t) && t[j] == '.' {
				j++
			}
			t = t[j:]
			if t == "" {
				return
			}
		} else if j == len(t) {
			return
		}
		toks = append(toks, t)
	}
	depth := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case depth > 0:
			switch c {
			case '(':
				depth++
			case ')':
				depth--
			case '{':
				end := strings.IndexByte(s[i:], '}')
				if end < 0 {
					return nil, fmt.Errorf("PGN: a comment is never closed")
				}
				i += end
			case ';':
				if end := strings.IndexByte(s[i:], '\n'); end < 0 {
					i = len(s)
				} else {
					i += end
				}
			}
		case c == '(':
			flush()
			depth++
		case c == '{':
			flush()
			end := strings.IndexByte(s[i:], '}')
			if end < 0 {
				return nil, fmt.Errorf("PGN: a comment is never closed")
			}
			i += end
		case c == ';':
			flush()
			end := strings.IndexByte(s[i:], '\n')
			if end < 0 {
				i = len(s)
			} else {
				i += end
			}
		case c == ' ' || c == '\n' || c == '\t' || c == '\r':
			flush()
		default:
			cur.WriteByte(c)
		}
	}
	if depth > 0 {
		return nil, fmt.Errorf("PGN: a variation is never closed")
	}
	flush()
	return toks, nil
}
