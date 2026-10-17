package parse

import "github.com/abdul-hamid-achik/tuimark/internal/css"

// AttrKeyword is one literal value of an enumerated attribute, with the
// lowest document version that accepts it there.
type AttrKeyword struct {
	Name  string
	Since int
}

// attrKeywords lists the literal values of the enumerated attributes
// (SPEC v0.4a §15.10), keyed by "tag.attr" for an attribute whose values
// depend on its tag and by "attr" otherwise. The checks themselves are in
// attrValue; TestAttrKeywordsMatchTheParser keeps the two in step. An
// attribute that takes paths, text, numbers, sizes, or an action name has
// none, and so a flag attribute such as hidden lists true and false only.
var attrKeywords = map[string][]AttrKeyword{
	"border":      append([]AttrKeyword{{"0", 1}, {"1", 1}}, cssKeywords("border")...),
	"wrap":        cssKeywords("wrap"),
	"hidden":      flagKeywords(1),
	"disabled":    flagKeywords(1),
	"focusable":   flagKeywords(1),
	"secret":      flagKeywords(1),
	"open":        flagKeywords(1),
	"mouse":       flagKeywords(2),
	"scroll.axis": {{"both", 1}, {"x", 1}, {"y", 1}},
	"rule.axis":   {{"x", 1}, {"y", 1}},
	"theme":       {{"dark", 1}, {"light", 1}, {"auto", 2}},
	"version":     {{"1", 1}, {"2", 2}, {"3", 3}},
	"scope":       {{"active", 2}, {"all", 2}},
	"stick":       {{"bottom", 3}},
}

func cssKeywords(prop string) []AttrKeyword {
	var out []AttrKeyword
	for _, k := range css.PropertyKeywords(prop) {
		out = append(out, AttrKeyword{k.Name, k.Since})
	}
	return out
}

func flagKeywords(since int) []AttrKeyword {
	return []AttrKeyword{{"true", since}, {"false", since}}
}

// AttrKeywords lists the literal values the attribute takes on the tag
// (SPEC v0.4a §15.10), or nil when it takes none. The Since of a keyword is
// the lowest version that accepts the keyword on a tag that has the
// attribute from version 1. The attribute may itself be newer on its tag
// (border on a table), so a caller reporting it takes the larger of the two
// sinces, as internal/catalog does.
func AttrKeywords(tag, attr string) []AttrKeyword {
	list, ok := attrKeywords[tag+"."+attr]
	if !ok {
		list = attrKeywords[attr]
	}
	return append([]AttrKeyword(nil), list...)
}

// TakesClassGuard reports whether the tag takes class:NAME attributes in a
// version="2" or version="3" document (SPEC §6.13): every tag but tui,
// style, keymap, and bind.
func TakesClassGuard(tag string) bool { return !noClassGuard[tag] }
