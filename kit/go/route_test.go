package kit

import "testing"

func TestInstanceAndPart(t *testing.T) {
	for _, c := range []struct{ source, inst, part string }{
		{"confirm__yes", "confirm", "yes"},
		{"profile__email", "profile", "email"},
		{"nav", "nav", ""},
		{"", "", ""},
		{"a__b__c", "a__b", "c"}, // the last "__" splits
		{"toast__", "toast", ""},
		{"__x", "", "x"},
	} {
		if got := Instance(c.source); got != c.inst {
			t.Errorf("Instance(%q) = %q, want %q", c.source, got, c.inst)
		}
		if got := Part(c.source); got != c.part {
			t.Errorf("Part(%q) = %q, want %q", c.source, got, c.part)
		}
	}
}
