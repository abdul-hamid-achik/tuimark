package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/dump"
	"github.com/abdul-hamid-achik/tuimark/internal/host"
)

// inlineNeedsCurrent is the error of --inline with a document that writes
// version (SPEC v0.4d §15.1, decision 241): the runtime's own message
// with the command in front, "tuimark: CMD: inline mode needs a <tui>
// without version".
func inlineNeedsCurrent(cmd string) error {
	return fmt.Errorf("%s: %s", cmd, strings.TrimPrefix(host.ErrInlineNeedsCurrent.Error(), "tuimark: "))
}

// applyInline makes app lay its frames out as an inline region when on is
// set (--inline of dump and inspect, SPEC v0.4d §26.15.6). A document that
// writes version is the error of inlineNeedsCurrent, before anything is
// printed.
func applyInline(app *host.App, cmd string, on bool) error {
	if !on {
		return nil
	}
	if _, err := app.SetInline(true); err != nil {
		if errors.Is(err, host.ErrInlineNeedsCurrent) {
			return inlineNeedsCurrent(cmd)
		}
		return err
	}
	return nil
}

// writeScrollbackSection appends the "=== scrollback ===" section of
// tuimark play --inline's text format (SPEC v0.4d §15.4): the committed
// rows, one per line, or the single line none.
func writeScrollbackSection(b *strings.Builder, sb *dump.Scrollback) {
	b.WriteString("=== scrollback ===\n")
	if len(sb.Grid) == 0 {
		b.WriteString("none\n")
		return
	}
	for _, row := range sb.Grid {
		b.WriteString(row)
		b.WriteByte('\n')
	}
}
