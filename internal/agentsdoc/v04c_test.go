package agentsdoc

import (
	"strings"
	"testing"
)

// SPEC v0.4c §22: props and the slot are a section of their own, right after
// the 0.4b composition section and before the current language's defaults,
// in the repository briefing and in the project briefing alike.
func TestPropsAndSlotSectionFollowsComposition(t *testing.T) {
	for name, md := range map[string]string{"Markdown": Markdown(), "ProjectMarkdown": ProjectMarkdown()} {
		comp := strings.Index(md, "### v0.4b: composition\n")
		props := strings.Index(md, "### v0.4c: props and the slot\n")
		defaults := strings.Index(md, "### v0.4b: the defaults of the current language\n")
		if comp < 0 || props < 0 || defaults < 0 || !(comp < props && props < defaults) {
			t.Fatalf("%s: section order wrong (composition %d, props and the slot %d, defaults %d)", name, comp, props, defaults)
		}
		sec := md[props:defaults]
		for _, want := range []string{
			"`<template id=\"T\" props=\"title max\">`",
			"`{$name}` may stand only in these places",
			"A prop the use does not pass",
			"Props are text, never data",
			"A template's **slot**",
			"The fill belongs to the place where it is written",
			"its ids are not scoped by the use",
			"`<use template=\"panel\" title=\"{$title}\"><text>Notes</text><slot/></use>`",
		} {
			if !strings.Contains(sec, want) {
				t.Errorf("%s: the props and slot section is missing %q", name, want)
			}
		}
	}
}

// SPEC v0.4c §22: the conventions say which of `with`, a prop, and the slot
// to reach for.
func TestPropsAndSlotConventions(t *testing.T) {
	for name, md := range map[string]string{"Markdown": Markdown(), "ProjectMarkdown": ProjectMarkdown()} {
		for _, want := range []string{
			"Pass literal text as a prop and data through `with`",
			"Put a body in the slot",
			"Size or vary a copy with a `style` prop",
			"A `style` prop on the root (`style=\"{$style}\"`) lets each use size its copy",
			"literal text (a title, a size, a variant) is a prop, and a body is the slot",
			"A template can also take literal text as props",
		} {
			if !strings.Contains(md, want) {
				t.Errorf("%s: the conventions are missing %q", name, want)
			}
		}
		for _, stale := range []string{"no slots", "takes no content, because"} {
			if strings.Contains(md, stale) {
				t.Errorf("%s: still says %q", name, stale)
			}
		}
	}
}
