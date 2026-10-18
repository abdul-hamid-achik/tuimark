package main

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// sample.json carries a literal snapshot of the three derived values
// (cols, counts, summary) so that `tuimark validate`, `dump` and
// `run --mock` can show the board with no Go at all. The host's Derive
// recomputes them from the cards, so this test keeps the two from
// drifting. UPDATE_SAMPLE=1 go test ./examples/tasks -run TestSampleSnapshot
// prints the fresh values instead of failing.
func TestSampleSnapshotMatchesDerive(t *testing.T) {
	var sample map[string]any
	raw, err := os.ReadFile("sample.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &sample); err != nil {
		t.Fatal(err)
	}
	a := newApp(t)
	for _, k := range []string{"cols", "counts", "summary"} {
		got, _ := a.ui.Get(k)
		// Through JSON, so both sides hold the same value types.
		b, _ := json.Marshal(got)
		var norm any
		_ = json.Unmarshal(b, &norm)
		if !reflect.DeepEqual(norm, sample[k]) {
			if os.Getenv("UPDATE_SAMPLE") != "" {
				t.Logf("%s: %s", k, b)
			}
			t.Errorf("sample.json %q is stale: Derive gives %s", k, b)
		}
	}
}
