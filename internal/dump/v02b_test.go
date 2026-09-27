package dump

import (
	"encoding/json"
	"strings"
	"testing"
)

// SPEC v0.2b §13.2, §13.3: classes appear only in dumps of version="2"
// documents, after the existing node members; a checked row dumps
// "checked": true and its text line ends with " checked", after
// " selected".
func TestClassesAndChecked(t *testing.T) {
	root, modals, g := frame()
	it1 := root.Children[1].Children[0]
	it1.Classes = []string{"row", "hot"}
	it1.Checked = true
	root.Children[0].Classes = []string{"q"}

	v1 := BuildWith(6, 3, root, modals, g, nil, "q", Options{})
	b, _ := json.Marshal(v1.Nodes)
	if strings.Contains(string(b), "classes") {
		t.Errorf("a version=\"1\" dump has classes: %s", b)
	}
	if !strings.Contains(string(b), `"key":"t-1","selected":true,"checked":true}`) {
		t.Errorf("checked is not appended after the existing members: %s", b)
	}

	v2 := BuildWith(6, 3, root, modals, g, nil, "q", Options{V2: true})
	b, _ = json.Marshal(v2.Nodes)
	if !strings.Contains(string(b), `"key":"t-1","selected":true,"classes":["row","hot"],"checked":true}`) {
		t.Errorf("v2 item node: %s", b)
	}
	if !strings.Contains(string(b), `"focused":true,"classes":["q"]}`) {
		t.Errorf("v2 input node: %s", b)
	}
	// A node without classes has no member at all (never []).
	if strings.Contains(string(b), `"classes":[]`) {
		t.Errorf("empty classes emitted: %s", b)
	}
	text := Text(v2)
	if !strings.Contains(text, "6x1 @(0,1) selected checked\n") {
		t.Errorf("text form:\n%s", text)
	}
	if strings.Contains(text, "hot") {
		t.Error("classes are not shown in the text form")
	}
	if c := CellAt(g, 0, 0); c.Ch != "•" || c.ID != "q" {
		t.Errorf("CellAt = %+v", c)
	}
}
