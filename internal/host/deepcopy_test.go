package host

import (
	"reflect"
	"testing"
	"unsafe"
)

// insertCopy is deepCopyJSON as it was written, inserting every member
// into a new map: the reference the cloning copy must agree with.
func insertCopy(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = insertCopy(e)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = insertCopy(e)
		}
		return out
	default:
		return v
	}
}

// containers collects the identity of every map and every non-empty
// array in v.
func containers(v any, into map[unsafe.Pointer]bool) {
	switch x := v.(type) {
	case map[string]any:
		into[reflect.ValueOf(x).UnsafePointer()] = true
		for _, e := range x {
			containers(e, into)
		}
	case []any:
		if len(x) > 0 {
			into[unsafe.Pointer(unsafe.SliceData(x))] = true
		}
		for _, e := range x {
			containers(e, into)
		}
	}
}

// scribble changes every container of v in place.
func scribble(v any) {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			scribble(e)
			if _, ok := e.(map[string]any); !ok {
				x[k] = "scribbled"
			}
		}
		x["added"] = true
	case []any:
		for i, e := range x {
			scribble(e)
			if _, ok := e.([]any); !ok {
				x[i] = 0.5
			}
		}
	}
}

// deepCopyJSON gives a value equal to its source, nil maps and arrays
// made empty ones, that shares no map or array with it: changing every
// container of the copy leaves the source as it was.
func TestDeepCopyJSONIsEqualAndSharesNothing(t *testing.T) {
	values := []any{
		nil, true, 1.5, "s",
		map[string]any(nil), []any(nil), map[string]any{}, []any{},
		map[string]any{"a": 1.0, "b": "x", "c": nil, "d": false},
		map[string]any{"m": map[string]any{"n": map[string]any{"o": []any{1.0, map[string]any{"p": "q"}}}}, "nil": map[string]any(nil), "arr": []any(nil)},
		[]any{[]any{[]any{}}, map[string]any{}, nil, "x", []any{map[string]any{"k": []any{2.0}}}},
	}
	big := map[string]any{}
	for i := 0; i < 40; i++ {
		big[string(rune('a'+i%26))+string(rune('A'+i/26))] = map[string]any{"i": float64(i), "l": []any{float64(i)}}
	}
	delete(big, "aA")
	values = append(values, big)
	for i, v := range values {
		got, want := deepCopyJSON(v), insertCopy(v)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("value %d: copy %#v, want %#v", i, got, want)
		}
		src := map[unsafe.Pointer]bool{}
		containers(v, src)
		cp := map[unsafe.Pointer]bool{}
		containers(got, cp)
		for p := range cp {
			if src[p] {
				t.Fatalf("value %d: the copy shares a container with its source", i)
			}
		}
		snap := insertCopy(v)
		scribble(got)
		if !reflect.DeepEqual(insertCopy(v), snap) {
			t.Fatalf("value %d: changing the copy changed its source: %#v", i, v)
		}
	}
}
