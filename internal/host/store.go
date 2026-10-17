package host

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"unsafe"
)

// ToJSON coerces any Go value to JSON-shaped data (null, bool, float64,
// string, []any, map[string]any) by a marshal round trip.
func ToJSON(v any) (any, error) {
	switch x := v.(type) {
	case nil, bool, string:
		return v, nil
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return nil, fmt.Errorf("value is not JSON-shaped: %v is not a JSON number", x)
		}
		return v, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("value is not JSON-shaped: %w", err)
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// lookup resolves a dotted path in a JSON value. Numeric segments index
// arrays (hosts may use them; markup cannot).
func lookup(root any, path string) (any, bool) {
	if path == "" {
		return root, true
	}
	cur := root
	for _, seg := range strings.Split(path, ".") {
		switch c := cur.(type) {
		case map[string]any:
			v, ok := c[seg]
			if !ok {
				return nil, false
			}
			cur = v
		case []any:
			i, err := strconv.Atoi(seg)
			if err != nil || i < 0 || i >= len(c) {
				return nil, false
			}
			cur = c[i]
		default:
			return nil, false
		}
	}
	return cur, true
}

// assign sets path in root to v, creating objects as needed, and returns
// the (possibly new) root.
func assign(root any, path string, v any) (any, error) {
	return assignIn(root, path, v, nil)
}

// assignIn is assign through cow: with a non-nil cow (a Batch's trial),
// every container along path that the trial does not own yet is copied
// first, and the write changes only the copies (cowSet). With a nil cow
// it changes the containers in place, as assign does.
func assignIn(root any, path string, v any, cow cowSet) (any, error) {
	if path == "" {
		return v, nil
	}
	segs := strings.Split(path, ".")
	return assignSegs(root, segs, v, path, cow)
}

// cowSet is what a Batch's trial owns of the store (applyBatchLocked):
// the containers it copied, or created, along the paths it wrote. A write
// of the trial copies each container on its path the first time it meets
// one the trial does not own, and changes only copies, so the store the
// trial started from is never changed and stays the rollback, while every
// container the batch does not write stays shared with it. A container is
// identified by its map, or by the first element of its array (a store
// holds no two arrays over the same memory: every value in it comes from
// a JSON coercion, a deep copy, or a fresh slice). A nil cowSet owns
// everything: a write changes the containers in place.
type cowSet map[unsafe.Pointer]struct{}

// ownMap returns c, or, when the trial does not own it yet, a shallow
// copy of it, which the trial then owns.
func (s cowSet) ownMap(c map[string]any) map[string]any {
	if s == nil {
		return c
	}
	if _, ok := s[reflect.ValueOf(c).UnsafePointer()]; ok {
		return c
	}
	out := make(map[string]any, len(c)+1)
	for k, e := range c {
		out[k] = e
	}
	s[reflect.ValueOf(out).UnsafePointer()] = struct{}{}
	return out
}

// ownArray is ownMap for an array. A write only ever indexes one, so an
// empty array is never written and never copied.
func (s cowSet) ownArray(c []any) []any {
	if s == nil || len(c) == 0 {
		return c
	}
	if _, ok := s[unsafe.Pointer(unsafe.SliceData(c))]; ok {
		return c
	}
	out := append([]any(nil), c...)
	s[unsafe.Pointer(unsafe.SliceData(out))] = struct{}{}
	return out
}

// created records m, an object the trial made, as owned.
func (s cowSet) created(m map[string]any) {
	if s != nil {
		s[reflect.ValueOf(m).UnsafePointer()] = struct{}{}
	}
}

// deepCopyJSON returns a deep copy of a JSON-shaped value (nil, bool,
// float64, string, []any, or map[string]any). Get hands this out instead
// of the store's own maps, so a host can never mutate state behind the
// lock (SPEC v0.3 §18.1, §30.4 decision 3).
func deepCopyJSON(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = deepCopyJSON(e)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = deepCopyJSON(e)
		}
		return out
	default:
		return v // nil, bool, float64, string are immutable
	}
}

// canAssign reports whether assign(root, path, v) would succeed, without
// changing anything: every value along the path, before its last segment,
// is an object, null, or missing (assign creates objects there), or an
// array indexed by an in-range numeric segment.
func canAssign(root any, path string) bool {
	if path == "" {
		return true
	}
	cur := root
	for _, seg := range strings.Split(path, ".") {
		switch c := cur.(type) {
		case nil:
			return true
		case map[string]any:
			cur = c[seg]
		case []any:
			i, err := strconv.Atoi(seg)
			if err != nil || i < 0 || i >= len(c) {
				return false
			}
			cur = c[i]
		default:
			return false
		}
	}
	return true
}

// assignSegs writes v at segs under cur and returns cur, or the copy of
// cur that cow made to write it. A container is changed (or copied) only
// once the write below it has succeeded, so a write that fails changes
// nothing and copies nothing.
func assignSegs(cur any, segs []string, v any, full string, cow cowSet) (any, error) {
	if len(segs) == 0 {
		return v, nil
	}
	seg := segs[0]
	switch c := cur.(type) {
	case []any:
		i, err := strconv.Atoi(seg)
		if err != nil || i < 0 || i >= len(c) {
			return nil, fmt.Errorf("set %q: %q is not an index of a %d-element array", full, seg, len(c))
		}
		nv, err := assignSegs(c[i], segs[1:], v, full, cow)
		if err != nil {
			return nil, err
		}
		c = cow.ownArray(c)
		c[i] = nv
		return c, nil
	case map[string]any:
		nv, err := assignSegs(c[seg], segs[1:], v, full, cow)
		if err != nil {
			return nil, err
		}
		c = cow.ownMap(c)
		c[seg] = nv
		return c, nil
	case nil:
		m := map[string]any{}
		nv, err := assignSegs(nil, segs[1:], v, full, cow)
		if err != nil {
			return nil, err
		}
		m[seg] = nv
		cow.created(m)
		return m, nil
	}
	return nil, fmt.Errorf("set %q: %q is inside a %T, not an object", full, seg, cur)
}

// Format renders a JSON value as display text.
func Format(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		if x == float64(int64(x)) && x < 1e15 && x > -1e15 {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'g', -1, 64)
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// scope resolves paths against each-aliases first, then the store.
type scope struct {
	alias  string
	value  any
	key    any
	parent *scope
}

func (s *scope) resolve(store any, path string) (any, bool) {
	head, rest, _ := strings.Cut(path, ".")
	for sc := s; sc != nil; sc = sc.parent {
		if sc.alias == head {
			if rest == "" {
				return sc.value, true
			}
			return lookup(sc.value, rest)
		}
	}
	return lookup(store, path)
}
