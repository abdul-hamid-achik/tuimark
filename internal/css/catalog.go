package css

import "sort"

// Keyword is one literal value of an enumerated property, pseudo-class
// family, or media feature, with the lowest document version whose
// stylesheets accept it. `tuimark catalog` (SPEC v0.4a §15.10) is built
// from these accessors, so it reads the same tables the parser checks.
type Keyword struct {
	Name  string
	Since int
}

// MediaFeature is one @media feature (SPEC §10.5): Since is the lowest
// document version that accepts it, and Keywords lists the literal values
// of a feature that takes words (theme) rather than a number of cells.
type MediaFeature struct {
	Name     string
	Since    int
	Keywords []Keyword
}

// mediaFeatures is the @media feature table parseMedia checks (a test keeps
// the two in step).
var mediaFeatures = []MediaFeature{
	{Name: "max-cols", Since: 1},
	{Name: "max-rows", Since: 1},
	{Name: "min-cols", Since: 1},
	{Name: "min-rows", Since: 1},
	{Name: "theme", Since: 2, Keywords: []Keyword{{"dark", 2}, {"light", 2}}},
}

// MediaFeatures lists the @media features in alphabetical order.
func MediaFeatures() []MediaFeature {
	out := make([]MediaFeature, len(mediaFeatures))
	for i, f := range mediaFeatures {
		f.Keywords = append([]Keyword(nil), f.Keywords...)
		out[i] = f
	}
	return out
}

// PseudoClasses lists the pseudo-classes selectors accept (:root, which
// only defines tokens, is not one) in alphabetical order, with the lowest
// document version that accepts each.
func PseudoClasses() []Keyword {
	var out []Keyword
	for p := range allowedPseudos {
		out = append(out, Keyword{p, 1})
	}
	for p := range v2Pseudos {
		out = append(out, Keyword{p, 2})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// PropertySince is the lowest document version whose stylesheets accept
// the property: 3 for the row-gap/column-gap kind, 2 for the grid kind, 1
// otherwise. An unknown name gives 0.
func PropertySince(name string) int {
	p, ok := props[name]
	switch {
	case !ok:
		return 0
	case p.v3:
		return 3
	case p.v2:
		return 2
	}
	return 1
}

// PropertyKeywords lists the literal values of an enumerated property or
// a boolean one, each with the version that accepts it: layout's grid is
// version 2, wrap's truncate-start and truncate-middle are version 3, and
// wrap's pre-wrap is 4, the current language (SPEC v0.4d).
// A property that takes only numbers, sizes, colors, or cell values has
// none. The order is the declaration order of the table (the one
// PropertyValues prints).
func PropertyKeywords(name string) []Keyword {
	p, ok := props[name]
	if !ok {
		return nil
	}
	base := PropertySince(name)
	var out []Keyword
	switch p.kind {
	case pBool:
		return []Keyword{{"true", base}, {"false", base}}
	case pEnum:
		for _, v := range p.values {
			out = append(out, Keyword{v, base})
		}
		if name == "layout" {
			out = append(out, Keyword{layoutGrid, 2})
		}
		for _, v := range p.v3Values {
			out = append(out, Keyword{v, 3})
		}
		for _, v := range p.v4Values {
			out = append(out, Keyword{v, 4})
		}
	}
	return out
}
