package main

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
)

// SPEC v0.4a §21 test 126: examples/monitor on 0.4a. The monitor stays
// version="3"; under both themes it gives no diagnostic at 40, 80, and 120
// columns, and its geometry is v0.3.2's. testdata/v0.3.2 holds the JSON
// dumps the v0.3.2 binary printed (`tuimark dump studio.tui --data
// sample.json --cols N --rows 24 --format json`); a JSON dump carries no
// colors, so the light theme's is the same file. They are frozen: a
// difference is a regression, never a fixture to regenerate.
func TestMonitorOn04a(t *testing.T) {
	for _, cols := range []int{40, 80, 120} {
		raw, err := os.ReadFile(fmt.Sprintf("testdata/v0.3.2/dump-%d.json", cols))
		if err != nil {
			t.Fatal(err)
		}
		var want any
		if err := json.Unmarshal(raw, &want); err != nil {
			t.Fatal(err)
		}
		for _, theme := range []string{"dark", "light"} {
			ui := loadDoc(t, "studio.tui")
			if err := ui.Set("@theme", theme); err != nil {
				t.Fatal(err)
			}
			if diags := ui.Validate(); len(diags) != 0 {
				t.Errorf("%d cols, %s: Validate gave %v", cols, theme, diags)
			}
			d, err := ui.Dump(cols, 24)
			if err != nil {
				t.Fatal(err)
			}
			if len(d.Errors) != 0 {
				t.Errorf("%d cols, %s: the dump has diagnostics %v", cols, theme, d.Errors)
			}
			b, err := json.Marshal(d)
			if err != nil {
				t.Fatal(err)
			}
			var got any
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%d cols, %s: the dump differs from v0.3.2's (testdata/v0.3.2/dump-%d.json)", cols, theme, cols)
			}
		}
	}
}
