package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// This file is the board's data model: plain Go values and pure functions,
// with no Tuimark import. The host (main.go) feeds them from the store and
// publishes their results back; the view never sees anything but JSON.

// columns are the lanes of the board, left to right.
var columns = []string{"todo", "doing", "done"}

// colTitles are the lane names the status line uses.
var colTitles = map[string]string{"todo": "Todo", "doing": "Doing", "done": "Done"}

// card is one task. Prio is 1 (low) to 3 (high); Due is YYYY-MM-DD or "".
type card struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Tag   string `json:"tag"`
	Prio  int    `json:"prio"`
	Due   string `json:"due"`
	Col   string `json:"col"`
}

const dateLayout = "2006-01-02"

func colIndex(col string) int {
	for i, c := range columns {
		if c == col {
			return i
		}
	}
	return -1
}

// visible splits cards into the lanes, keeping board order, and keeps only
// the cards the filter matches: a case-insensitive substring of the title,
// the tag, or the priority word.
func visible(cards []card, filter string) map[string][]card {
	out := map[string][]card{}
	for _, c := range columns {
		out[c] = []card{}
	}
	for _, c := range cards {
		if matches(c, filter) {
			out[c.Col] = append(out[c.Col], c)
		}
	}
	return out
}

func matches(c card, filter string) bool {
	q := strings.ToLower(strings.TrimSpace(filter))
	if q == "" {
		return true
	}
	return strings.Contains(strings.ToLower(c.Title), q) ||
		strings.Contains(strings.ToLower(c.Tag), q) ||
		strings.Contains(prioLabel(c.Prio), q)
}

func prioLabel(p int) string {
	switch {
	case p >= 3:
		return "high"
	case p == 2:
		return "medium"
	default:
		return "low"
	}
}

func prioMark(p int) string {
	switch {
	case p >= 3:
		return "▲"
	case p == 2:
		return "◆"
	default:
		return "▽"
	}
}

// parseDay reads a YYYY-MM-DD date; ok is false for "" and for bad input.
func parseDay(s string) (time.Time, bool) {
	t, err := time.Parse(dateLayout, s)
	return t, err == nil
}

// row is a card as the view wants it: every string pre-formatted and every
// condition a boolean, because the markup has no expressions.
func row(c card, today time.Time) map[string]any {
	due, soon, overdue := "", false, false
	if d, ok := parseDay(c.Due); ok {
		due = d.Format("Jan 2")
		if c.Col != "done" && !today.IsZero() {
			days := int(d.Sub(today).Hours() / 24)
			overdue = days < 0
			soon = days >= 0 && days <= 2
		}
	}
	return map[string]any{
		"id":         c.ID,
		"title":      c.Title,
		"tag":        c.Tag,
		"prio_mark":  prioMark(c.Prio),
		"prio_label": prioLabel(c.Prio),
		"p_high":     c.Prio >= 3,
		"p_med":      c.Prio == 2,
		"p_low":      c.Prio < 2,
		"is_bug":     c.Tag == "bug",
		"is_feature": c.Tag == "feature",
		"is_chore":   c.Tag == "chore",
		"due":        due,
		"overdue":    overdue,
		"soon":       soon,
	}
}

// rows renders a lane for the view.
func rows(cards []card, today time.Time) []any {
	out := make([]any, 0, len(cards))
	for _, c := range cards {
		out = append(out, row(c, today))
	}
	return out
}

// moveCards shifts every card whose id is in ids one lane (dir is -1 or 1).
// A moved card goes to the end of its new lane; a card already at the edge
// stays where it is. It returns the board and how many cards moved.
func moveCards(cards []card, ids map[string]bool, dir int) ([]card, int) {
	moved := 0
	out := make([]card, 0, len(cards))
	var tail []card
	for _, c := range cards {
		if ids[c.ID] {
			if i := colIndex(c.Col) + dir; i >= 0 && i < len(columns) {
				c.Col = columns[i]
				tail = append(tail, c)
				moved++
				continue
			}
		}
		out = append(out, c)
	}
	// Keep the order the moved cards had on the board, after the cards that
	// already sat in the lane.
	return append(out, tail...), moved
}

// reorder swaps card id with its neighbour in the same lane, as the lane is
// shown (dir -1 is up, 1 is down), so the filter is respected. It reports
// whether anything changed.
func reorder(cards []card, id string, dir int, filter string) ([]card, bool) {
	pos := -1
	for i, c := range cards {
		if c.ID == id {
			pos = i
		}
	}
	if pos < 0 {
		return cards, false
	}
	col := cards[pos].Col
	for j := pos + dir; j >= 0 && j < len(cards); j += dir {
		if cards[j].Col == col && matches(cards[j], filter) {
			out := append([]card(nil), cards...)
			out[pos], out[j] = out[j], out[pos]
			return out, true
		}
	}
	return cards, false
}

// deleteCards drops every card whose id is in ids.
func deleteCards(cards []card, ids map[string]bool) []card {
	out := make([]card, 0, len(cards))
	for _, c := range cards {
		if !ids[c.ID] {
			out = append(out, c)
		}
	}
	return out
}

func find(cards []card, id string) (card, bool) {
	for _, c := range cards {
		if c.ID == id {
			return c, true
		}
	}
	return card{}, false
}

// nextID is one past the highest numeric id of the form c<N>.
func nextID(cards []card) int {
	n := 0
	for _, c := range cards {
		if v, err := strconv.Atoi(strings.TrimPrefix(c.ID, "c")); err == nil && v > n {
			n = v
		}
	}
	return n + 1
}

// checkCards validates a board that came from outside (a -file).
func checkCards(cards []card) error {
	seen := map[string]bool{}
	for i, c := range cards {
		switch {
		case c.ID == "":
			return fmt.Errorf("card %d has no id", i+1)
		case seen[c.ID]:
			return fmt.Errorf("card %d: duplicate id %q", i+1, c.ID)
		case colIndex(c.Col) < 0:
			return fmt.Errorf("card %s: unknown column %q", c.ID, c.Col)
		case strings.TrimSpace(c.Title) == "":
			return fmt.Errorf("card %s has no title", c.ID)
		}
		if c.Due != "" {
			if _, ok := parseDay(c.Due); !ok {
				return fmt.Errorf("card %s: due %q is not YYYY-MM-DD", c.ID, c.Due)
			}
		}
		seen[c.ID] = true
	}
	return nil
}

// boardFile is the on-disk shape of -file.
type boardFile struct {
	Cards []card `json:"cards"`
}

// readBoard loads and checks a board file. A missing file is os.ErrNotExist.
func readBoard(path string) ([]card, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f boardFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := checkCards(f.Cards); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if f.Cards == nil {
		f.Cards = []card{}
	}
	return f.Cards, nil
}

// writeBoard saves the board atomically: a temporary file in the same
// directory, then a rename.
func writeBoard(path string, cards []card) error {
	raw, err := json.MarshalIndent(boardFile{Cards: cards}, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tasks-*.json")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(append(raw, '\n'))
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// unionIDs returns the ids of all lists, sorted, for stable messages.
func sortedIDs(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
