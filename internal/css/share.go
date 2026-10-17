package css

// Style sharing: one cascade (one frame) computes the style of many
// elements that look alike to every selector, such as the rows of a long
// list built from one template. A computed style depends only on what
// compute reads: the rules that match (the element's type, id, classes,
// and pseudo-classes, and the same for each ancestor a child combinator
// reaches), its presentational and style="" declarations, and the
// computed style of the parent it inherits from. ShareKey names all of it,
// so two elements with equal keys get equal styles, and Shared computes
// each distinct key once per cascade.
//
// The parent chain is interned (Chain): a chain id stands for an element
// and every ancestor above it, as the matching sees them, so it covers
// selectors of any length. Each computed style gets a style id (Shared,
// Unique) that the children's keys carry, so equal ids mean equal
// inherited values. Nothing is kept across cascades: a cascade is one
// frame's media, theme, and tokens, and its V003 diagnostics are reported
// the first time a key is computed in it, as compute itself reports them
// once per cascade.

// Features is how an element looks to the selectors: its type, its id, its
// classes joined by single spaces (class names never hold one), and its
// pseudo-classes as a PseudoBits mask.
type Features struct {
	Tag, ID, Classes string
	Pseudos          uint8
}

// The pseudo-classes of SPEC §10.1 as bits of a PseudoBits mask.
const (
	PseudoFocus uint8 = 1 << iota
	PseudoSelected
	PseudoDisabled
	PseudoEmpty
	PseudoFocusWithin
	PseudoChecked
)

// PseudoBit returns the bit of pseudo-class p, 0 for a name no selector
// can hold.
func PseudoBit(p string) uint8 {
	switch p {
	case "focus":
		return PseudoFocus
	case "selected":
		return PseudoSelected
	case "disabled":
		return PseudoDisabled
	case "empty":
		return PseudoEmpty
	case "focus-within":
		return PseudoFocusWithin
	case "checked":
		return PseudoChecked
	}
	return 0
}

// PseudoBits returns el's pseudo-classes as a mask, asked through
// HasPseudo, so it is what the matching sees.
func PseudoBits(el Element) uint8 {
	var m uint8
	for _, p := range [...]string{"focus", "selected", "disabled", "empty", "focus-within", "checked"} {
		if el.HasPseudo(p) {
			m |= PseudoBit(p)
		}
	}
	return m
}

// ShareKey is everything the computed style of one element depends on in
// a cascade: its parent's chain (Chain; 0 without a parent), the style id
// of the style it inherits from (0 for none), its own features as the
// subject, and the identity of its declaration slices (their first
// element and length; a slice is never changed once built).
type ShareKey struct {
	Chain, Parent   int32
	Self            Features
	Hints, Inline   *Decl
	NHints, NInline int
}

// DeclKey returns the identity of a declaration slice for a ShareKey.
func DeclKey(ds []Decl) (*Decl, int) {
	if len(ds) == 0 {
		return nil, 0
	}
	return &ds[0], len(ds)
}

type chainKey struct {
	parent int32
	f      Features
}

// shareMemo is a cascade's sharing state. The recent slots are checked
// before the maps: siblings and the rows of a list come in runs of equal
// keys.
type shareMemo struct {
	styles   []Style
	byKey    map[ShareKey]int32
	chains   map[chainKey]int32
	nextID   int32
	ids      map[*Style]int32
	recent   [4]ShareKey
	recentID [4]int32
	recentN  int
	rChain   [4]chainKey
	rChainID [4]int32
	rChainN  int
}

// EnableSharing turns style sharing on for this cascade (Shared, Chain,
// Register, StyleID). Without it Sharing reports false and the host
// computes every style with Compute.
func (c *Cascade) EnableSharing() {
	c.share = &shareMemo{byKey: map[ShareKey]int32{}, chains: map[chainKey]int32{}, ids: map[*Style]int32{}, styles: []Style{{}}}
}

// Sharing reports whether EnableSharing was called.
func (c *Cascade) Sharing() bool { return c.share != nil }

// Shared returns the computed style of el (Compute(el, parent, hints,
// inline)) and its style id, computing it only the first time this
// cascade sees k. k must be el's key: its parent's chain and the style id
// of parent, el's features as the subject, and hints and inline.
func (c *Cascade) Shared(k ShareKey, el Element, parent *Style, hints, inline []Decl) (Style, int32) {
	m := c.share
	for i := 0; i < m.recentN; i++ {
		if m.recent[i] == k {
			return m.styles[m.recentID[i]], m.recentID[i]
		}
	}
	id, ok := m.byKey[k]
	if !ok {
		id = int32(len(m.styles))
		m.styles = append(m.styles, c.compute(el, parent, hints, inline, nil))
		m.byKey[k] = id
	}
	slot := m.recentN
	if slot == len(m.recent) {
		copy(m.recent[:], m.recent[1:])
		copy(m.recentID[:], m.recentID[1:])
		slot--
	} else {
		m.recentN++
	}
	m.recent[slot], m.recentID[slot] = k, id
	return m.styles[id], id
}

// Unique returns a style id no key shares: the id of a style computed
// without the memo, so its children share only among themselves.
func (c *Cascade) Unique() int32 {
	c.share.nextID--
	return c.share.nextID
}

// Chain returns the id of the chain made of an element with features f
// under the chain parent (0 for an element without a parent): equal ids
// mean the element and each of its ancestors look the same to every
// selector.
func (c *Cascade) Chain(parent int32, f Features) int32 {
	m := c.share
	k := chainKey{parent, f}
	for i := 0; i < m.rChainN; i++ {
		if m.rChain[i] == k {
			return m.rChainID[i]
		}
	}
	id, ok := m.chains[k]
	if !ok {
		id = int32(len(m.chains) + 1)
		m.chains[k] = id
	}
	slot := m.rChainN
	if slot == len(m.rChain) {
		copy(m.rChain[:], m.rChain[1:])
		copy(m.rChainID[:], m.rChainID[1:])
		slot--
	} else {
		m.rChainN++
	}
	m.rChain[slot], m.rChainID[slot] = k, id
	return id
}

// Register records that st, a box's computed style, has style id id, so a
// later Shared call whose parent is st (a separate entry into the
// cascade, such as a tab's content or a table's rows) can name it.
func (c *Cascade) Register(st *Style, id int32) { c.share.ids[st] = id }

// StyleID returns the style id Register recorded for st, 0 for nil, and
// ok false when st was never registered.
func (c *Cascade) StyleID(st *Style) (int32, bool) {
	if st == nil {
		return 0, true
	}
	id, ok := c.share.ids[st]
	return id, ok
}
