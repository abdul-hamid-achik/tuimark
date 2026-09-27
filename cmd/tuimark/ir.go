package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/host"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// irTokens resolves the theme + :root token table for a loaded document,
// media conditions ignored (SPEC §13.1). This is the one piece of token
// resolution that lives outside internal/parse: it needs the app's already
// -loaded stylesheets (host.App.Sheets, which resolved style src= against
// the document's directory), so parse.BuildIR takes the result as a plain
// map instead of reaching into the host itself.
func irTokens(theme string, sheets []*css.Sheet) map[string]string {
	if theme == "" {
		theme = "dark"
	}
	base, ok := css.Themes[theme]
	if !ok {
		base = css.Themes["dark"]
	}
	tokens := make(map[string]string, len(base))
	for k, v := range base {
		tokens[k] = v
	}
	for _, sh := range sheets {
		for _, r := range sh.Rules {
			for _, sel := range r.Selectors {
				if !sel.Root {
					continue
				}
				for _, d := range r.Decls {
					tokens[strings.TrimPrefix(d.Prop, "--")] = d.Value
				}
			}
		}
	}
	return tokens
}

func (c *cli) cmdIR(args []string) int {
	fs := c.newFlags("ir")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return 1
	}
	file, err := oneFile("ir", pos)
	if err != nil {
		return c.fail(err)
	}
	app, err := host.Load(file)
	if err != nil {
		return c.fail(err)
	}
	doc := app.Document()
	// A document with any error-severity diagnostic — not only one so
	// broken doc.Root is nil — must not print IR: SPEC §13.1/§23 restrict
	// a node's kind to the 18-value enum ("unknown kind fails before
	// layout"), and an invalid document (duplicate id, bad size, wrong
	// content model, ...) is exit 2 on every other command (SPEC §14).
	//
	// The gate must also cover the document's stylesheets, not only
	// doc.Diags: app.Static() additionally holds V003 for rules inside
	// <style>/.tcss, V006 for a bad style src=, and the unknown-token
	// diagnostics from css.CheckTokens — all of which validate and dump
	// already report as errors on the same document (decisiones.md,
	// ronda 1: "tuimark ir sale con 2 ... si el documento tiene errores").
	diags := app.Static()
	if doc.Root == nil || diags.HasErrors() {
		for _, d := range diags.Sorted() {
			fmt.Fprintln(c.stderr, d.String())
		}
		return 2
	}
	tokens := irTokens(doc.Theme, app.Sheets())
	irDoc := parse.BuildIR(doc, tokens)
	b, err := json.MarshalIndent(irDoc, "", "  ")
	if err != nil {
		return c.fail(err)
	}
	c.stdout.Write(b)
	c.stdout.Write([]byte("\n"))
	return 0
}
