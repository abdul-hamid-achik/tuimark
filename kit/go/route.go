// route: split a kit node id into its instance and its part.
//
// Copy this file into your project and change the package name. It has no
// dependencies. dialog.go and form.go call Instance and Part, so copy this
// file with them.
//
// A kit id is "instance__part" (confirm__yes, profile__email). An action does
// not say which instance fired it, so a host routes by the instance of the
// event's source. A source with no "__" is its own instance.
package kit

import "strings"

// Instance is everything before the last "__" of source, or the whole id when
// there is none: Instance("confirm__yes") is "confirm", Instance("nav") is
// "nav".
func Instance(source string) string {
	inst, _ := splitID(source)
	return inst
}

// Part is everything after the last "__" of source, or "" when there is
// none: Part("confirm__yes") is "yes", Part("nav") is "".
func Part(source string) string {
	_, part := splitID(source)
	return part
}

func splitID(id string) (instance, part string) {
	if i := strings.LastIndex(id, "__"); i >= 0 {
		return id[:i], id[i+2:]
	}
	return id, ""
}
