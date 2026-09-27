package paint

import (
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
)

func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// SPEC v0.2 §21 test 33: the detection rules of §26.3, one by one, first
// match wins.
func TestDetectProfile(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want Profile
	}{
		{"TUIMARK_COLOR wins over everything", map[string]string{"TUIMARK_COLOR": "16", "NO_COLOR": "1", "COLORTERM": "truecolor", "TERM": "xterm-kitty"}, ANSI16},
		{"TUIMARK_COLOR none", map[string]string{"TUIMARK_COLOR": "none", "TERM": "xterm-256color"}, NoColor},
		{"TUIMARK_COLOR truecolor on dumb", map[string]string{"TUIMARK_COLOR": "truecolor", "TERM": "dumb"}, TrueColor},
		{"TUIMARK_COLOR 256", map[string]string{"TUIMARK_COLOR": "256"}, ANSI256},
		{"empty TUIMARK_COLOR is unset", map[string]string{"TUIMARK_COLOR": "", "TERM": "xterm-256color"}, ANSI256},
		{"NO_COLOR", map[string]string{"NO_COLOR": "1", "COLORTERM": "truecolor", "TERM": "xterm-256color"}, NoColor},
		{"empty NO_COLOR is unset", map[string]string{"NO_COLOR": "", "TERM": "xterm-256color"}, ANSI256},
		{"NO_COLOR before WT_SESSION", map[string]string{"NO_COLOR": "x", "WT_SESSION": "abc"}, NoColor},
		{"WT_SESSION before empty TERM", map[string]string{"WT_SESSION": "abc"}, TrueColor},
		{"WT_SESSION before dumb", map[string]string{"WT_SESSION": "abc", "TERM": "dumb"}, TrueColor},
		{"empty TERM", map[string]string{}, NoColor},
		{"dumb TERM", map[string]string{"TERM": "dumb", "COLORTERM": "truecolor"}, NoColor},
		{"COLORTERM truecolor", map[string]string{"TERM": "xterm", "COLORTERM": "truecolor"}, TrueColor},
		{"COLORTERM 24bit", map[string]string{"TERM": "xterm", "COLORTERM": "24bit"}, TrueColor},
		{"COLORTERM is ASCII case-insensitive", map[string]string{"TERM": "xterm", "COLORTERM": "TrueColor"}, TrueColor},
		{"COLORTERM ignored under tmux", map[string]string{"TERM": "tmux-256color", "COLORTERM": "truecolor"}, ANSI256},
		{"COLORTERM ignored under screen", map[string]string{"TERM": "screen", "COLORTERM": "24bit"}, ANSI256},
		{"COLORTERM other value", map[string]string{"TERM": "xterm", "COLORTERM": "yes"}, ANSI16},
		{"alacritty", map[string]string{"TERM": "alacritty"}, TrueColor},
		{"contour", map[string]string{"TERM": "contour"}, TrueColor},
		{"foot", map[string]string{"TERM": "foot-extra"}, TrueColor},
		{"ghostty", map[string]string{"TERM": "xterm-ghostty"}, TrueColor},
		{"kitty", map[string]string{"TERM": "xterm-kitty"}, TrueColor},
		{"rio", map[string]string{"TERM": "rio"}, TrueColor},
		{"wezterm", map[string]string{"TERM": "wezterm"}, TrueColor},
		{"-direct", map[string]string{"TERM": "xterm-direct"}, TrueColor},
		{"256color", map[string]string{"TERM": "xterm-256color"}, ANSI256},
		{"tmux", map[string]string{"TERM": "tmux"}, ANSI256},
		{"screen", map[string]string{"TERM": "screen.xterm"}, ANSI256},
		{"otherwise 16", map[string]string{"TERM": "xterm"}, ANSI16},
		{"vt100", map[string]string{"TERM": "vt100"}, ANSI16},
	}
	for _, c := range cases {
		got, err := DetectProfile(envOf(c.env))
		if err != nil || got != c.want {
			t.Errorf("%s: DetectProfile = %v, %v; want %v", c.name, got, err, c.want)
		}
	}
	// TUIMARK_COLOR is exact and case-sensitive; an invalid value is an
	// error, never a guess.
	for _, bad := range []string{"TrueColor", "24bit", "8", "256color", " 16", "no"} {
		if _, err := DetectProfile(envOf(map[string]string{"TUIMARK_COLOR": bad, "TERM": "xterm"})); err == nil || !strings.Contains(err.Error(), "TUIMARK_COLOR") {
			t.Errorf("TUIMARK_COLOR=%q: err = %v", bad, err)
		}
	}
}

func TestParseProfile(t *testing.T) {
	for _, p := range []Profile{TrueColor, ANSI256, ANSI16, NoColor} {
		got, err := ParseProfile(p.String())
		if err != nil || got != p {
			t.Errorf("ParseProfile(%q) = %v, %v", p.String(), got, err)
		}
	}
	if _, err := ParseProfile("auto"); err == nil {
		t.Error("ParseProfile(auto) accepted")
	}
}

// §21 test 33: the C256 vectors of §26.3, plus exact cube and grey-ramp
// entries.
func TestConvert256(t *testing.T) {
	cases := []struct {
		hex  string
		want uint8
	}{
		{"#0d1117", 233}, {"#30363d", 237}, {"#e6edf3", 255}, {"#ff0000", 196}, {"#5f87af", 67},
		{"#000000", 16}, {"#ffffff", 231}, {"#010101", 16}, {"#080808", 232}, {"#eeeeee", 255},
		{"#808080", 244}, {"#8b949e", 246}, {"#161b22", 234}, {"#00ff00", 46}, {"#0000ff", 21},
		{"#d7af87", 180}, {"#303030", 236}, {"#f0f0f0", 255},
		// Squared RGB distance with the grey only when strictly nearer, as
		// tmux's colour_find_rgb: x/ansi.Convert256 (HSLuv distance) gives
		// 195, 103, and 16 for these three.
		{"#e6edf3", 255}, {"#8b949e", 246}, {"#00000d", 232},
		// Ties go to the cube: (4,4,4) is 48 from both black (16) and
		// grey 232 (8,8,8).
		{"#040404", 16},
	}
	for _, c := range cases {
		col, err := css.ParseLiteralColor(c.hex)
		if err != nil {
			t.Fatal(err)
		}
		if got := Convert256(col.R, col.G, col.B); got != c.want {
			t.Errorf("Convert256(%s) = %d, want %d", c.hex, got, c.want)
		}
	}
	// Every exact cube color maps to itself, every grey-ramp color to its
	// ramp index.
	for i := 0; i < 216; i++ {
		r, g, b := cubeLevels[i/36], cubeLevels[i/6%6], cubeLevels[i%6]
		if got := Convert256(uint8(r), uint8(g), uint8(b)); int(got) != 16+i {
			t.Errorf("cube %d (%d,%d,%d) = %d", 16+i, r, g, b, got)
		}
	}
	for i := 0; i < 24; i++ {
		v := uint8(8 + 10*i)
		if got := Convert256(v, v, v); int(got) != 232+i {
			t.Errorf("grey %d (%d) = %d", 232+i, v, got)
		}
	}
}

// §21 test 33: the C16 vectors of §26.3; ties go to the lower index.
func TestConvert16(t *testing.T) {
	names := map[string]uint8{}
	for i, n := range []string{"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white", "bright-black", "bright-red", "bright-green", "bright-yellow", "bright-blue", "bright-magenta", "bright-cyan", "bright-white"} {
		names[n] = uint8(i)
	}
	cases := []struct{ hex, want string }{
		{"#0d1117", "black"}, {"#e6edf3", "white"}, {"#8b949e", "bright-black"}, {"#ff0000", "bright-red"},
		{"#cd0000", "red"}, {"#5c5cff", "bright-blue"}, {"#0000ee", "blue"}, {"#ffffff", "bright-white"},
		{"#30363d", "black"}, {"#161b22", "black"}, {"#f6f8fa", "bright-white"}, {"#1f2328", "black"},
	}
	for _, c := range cases {
		col, _ := css.ParseLiteralColor(c.hex)
		if got := Convert16(col.R, col.G, col.B); got != names[c.want] {
			t.Errorf("Convert16(%s) = %d, want %s (%d)", c.hex, got, c.want, names[c.want])
		}
	}
	// Each palette entry maps to itself, except that exact duplicates would
	// go to the lower index (the palette has none).
	for i, c := range xtermPalette {
		if got := Convert16(uint8(c[0]), uint8(c[1]), uint8(c[2])); int(got) != i {
			t.Errorf("palette %d = %d", i, got)
		}
	}
	// (230,0,0) is 25² from both red (205,0,0) and bright-red (255,0,0):
	// the tie goes to the lower index.
	if got := Convert16(230, 0, 0); got != 1 {
		t.Errorf("(230,0,0) = %d, want red (the lower index of a tie)", got)
	}
	if got := Convert16(231, 0, 0); got != 9 {
		t.Errorf("(231,0,0) = %d, want bright-red", got)
	}
}

// §21 test 33 and the §26.3 SGR table, per profile: RGB converts, ANSI
// indexes and default stay, none drops colors and keeps attributes, and
// SGR without a profile is still truecolor.
func TestSGRProfiles(t *testing.T) {
	rgb := func(hex string) css.Color { c, _ := css.ParseLiteralColor(hex); return c }
	cell := Cell{Ch: 'x', FG: rgb("#e6edf3"), BG: rgb("#0d1117"), Attrs: Bold | Reverse}
	ansi := Cell{Ch: 'x', FG: css.Color{Kind: css.ColorANSI, Index: 3}, BG: css.Color{Kind: css.ColorANSI, Index: 12}, Attrs: Underline}
	def := Cell{Ch: 'x', FG: css.Color{Kind: css.ColorDefault}}
	cases := []struct {
		p                 Profile
		cell, ansi, deflt string
	}{
		{TrueColor, "\x1b[0;1;7;38;2;230;237;243;48;2;13;17;23m", "\x1b[0;4;33;104m", "\x1b[0;39;49m"},
		{ANSI256, "\x1b[0;1;7;38;5;255;48;5;233m", "\x1b[0;4;33;104m", "\x1b[0;39;49m"},
		{ANSI16, "\x1b[0;1;7;37;40m", "\x1b[0;4;33;104m", "\x1b[0;39;49m"},
		{NoColor, "\x1b[0;1;7m", "\x1b[0;4m", "\x1b[0m"},
	}
	for _, c := range cases {
		if got := SGRWith(cell, c.p); got != c.cell {
			t.Errorf("%v rgb: %q, want %q", c.p, got, c.cell)
		}
		if got := SGRWith(ansi, c.p); got != c.ansi {
			t.Errorf("%v ansi: %q, want %q", c.p, got, c.ansi)
		}
		if got := SGRWith(def, c.p); got != c.deflt {
			t.Errorf("%v default: %q, want %q", c.p, got, c.deflt)
		}
	}
	// Bright colors from RGB in 16 colors use the 90/100 codes.
	if got := SGRWith(Cell{FG: rgb("#ff0000"), BG: rgb("#8b949e")}, ANSI16); got != "\x1b[0;91;100m" {
		t.Errorf("16 bright = %q", got)
	}
	// Without a profile: truecolor, byte for byte the v0.1 SGR.
	if SGR(cell) != cases[0].cell || SGR(ansi) != cases[0].ansi {
		t.Errorf("SGR without a profile = %q %q", SGR(cell), SGR(ansi))
	}
	// Full and Diff apply the profile; their zero Options stay truecolor.
	g := NewGrid(2, 1)
	for i := range g.Cells {
		g.Cells[i].FG, g.Cells[i].BG, g.Cells[i].Attrs = rgb("#e6edf3"), rgb("#0d1117"), Reverse
	}
	if got := Full(g); !strings.Contains(got, "38;2;230;237;243") {
		t.Errorf("Full = %q", got)
	}
	if got := FullWith(g, Options{Profile: ANSI256}); got != AutowrapOff+"\x1b[0;7;38;5;255;48;5;233m  \x1b[0m\n"+AutowrapOn {
		t.Errorf("FullWith 256 = %q", got)
	}
	if got := DiffWith(nil, g, Options{Profile: NoColor}); got != "\x1b[0m\x1b[2J\x1b[1;1H\x1b[0;7m  \x1b[0m\x1b[?25l" {
		t.Errorf("DiffWith none = %q", got)
	}
	if got := Diff(nil, g); !strings.Contains(got, "48;2;13;17;23") {
		t.Errorf("Diff = %q", got)
	}
}
