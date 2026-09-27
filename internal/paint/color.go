package paint

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
)

// Profile is the color depth SGR sequences are written in (SPEC v0.2
// §26.3). Colors stay canonical everywhere else (IR, cascade, paint cells,
// dump --styles); they are converted only when ANSI is written. The zero
// value is TrueColor, so paint functions called without a profile emit
// truecolor, as in v0.1.
type Profile uint8

// The color profiles, from the most colors to none.
const (
	TrueColor Profile = iota // 24-bit: 38;2;r;g;b
	ANSI256                  // xterm 256 colors: 38;5;n
	ANSI16                   // the 16 ANSI colors: 30-37, 90-97
	NoColor                  // no color codes at all; attributes stay
)

var profileNames = [...]string{TrueColor: "truecolor", ANSI256: "256", ANSI16: "16", NoColor: "none"}

// String returns the profile's name as TUIMARK_COLOR spells it.
func (p Profile) String() string {
	if int(p) < len(profileNames) {
		return profileNames[p]
	}
	return "Profile(" + strconv.Itoa(int(p)) + ")"
}

// ParseProfile parses a profile name, exact and case-sensitive:
// truecolor, 256, 16, or none (the values of TUIMARK_COLOR and of
// `preview --color`).
func ParseProfile(s string) (Profile, error) {
	for p, name := range profileNames {
		if s == name {
			return Profile(p), nil
		}
	}
	return TrueColor, fmt.Errorf("unknown color profile %q (want truecolor, 256, 16, or none)", s)
}

// truecolorTerms are TERM substrings of terminals known to render 24-bit
// color (colorprofile v0.4.3's envColorProfile list).
var truecolorTerms = []string{"alacritty", "contour", "foot", "ghostty", "kitty", "rio", "wezterm"}

// DetectProfile picks the color profile from the environment (SPEC v0.2
// §26.3), first match wins:
//
//  1. TUIMARK_COLOR, when set (an invalid value is an error)
//  2. NO_COLOR non-empty → none
//  3. WT_SESSION non-empty → truecolor
//  4. TERM empty or dumb → none
//  5. COLORTERM truecolor or 24bit (ASCII case-insensitive), unless TERM
//     starts with tmux or screen → truecolor
//  6. TERM contains alacritty, contour, foot, ghostty, kitty, rio, or
//     wezterm, or ends with -direct → truecolor
//  7. TERM ends with 256color, or starts with tmux or screen → 256
//  8. otherwise → 16
//
// An empty value is the same as unset. No external program is run. The
// caller passes os.Getenv (tests pass a map lookup); `preview --color`
// is checked by the caller before this.
func DetectProfile(getenv func(string) string) (Profile, error) {
	if v := getenv("TUIMARK_COLOR"); v != "" {
		p, err := ParseProfile(v)
		if err != nil {
			return TrueColor, fmt.Errorf("TUIMARK_COLOR=%q: want truecolor, 256, 16, or none", v)
		}
		return p, nil
	}
	if getenv("NO_COLOR") != "" {
		return NoColor, nil
	}
	if getenv("WT_SESSION") != "" {
		return TrueColor, nil
	}
	termName := getenv("TERM")
	if termName == "" || termName == "dumb" {
		return NoColor, nil
	}
	multiplexer := strings.HasPrefix(termName, "tmux") || strings.HasPrefix(termName, "screen")
	if ct := getenv("COLORTERM"); (strings.EqualFold(ct, "truecolor") || strings.EqualFold(ct, "24bit")) && !multiplexer {
		return TrueColor, nil
	}
	for _, t := range truecolorTerms {
		if strings.Contains(termName, t) {
			return TrueColor, nil
		}
	}
	if strings.HasSuffix(termName, "-direct") {
		return TrueColor, nil
	}
	if strings.HasSuffix(termName, "256color") || multiplexer {
		return ANSI256, nil
	}
	return ANSI16, nil
}

// cubeLevels are the channel values of the xterm 6×6×6 color cube.
var cubeLevels = [6]int{0x00, 0x5f, 0x87, 0xaf, 0xd7, 0xff}

// dist2 is the squared RGB distance between two colors.
func dist2(r1, g1, b1, r2, g2, b2 int) int {
	dr, dg, db := r1-r2, g1-g2, b1-b2
	return dr*dr + dg*dg + db*db
}

// Convert256 maps an RGB color to an xterm-256 index (SPEC v0.2 §26.3,
// C256): tmux's colour_find_rgb (colour.c), in integer arithmetic. It takes
// the 6×6×6 cube entry and the 24-step grey ramp entry nearest the color
// and returns the grey only when it is strictly nearer by squared RGB
// distance. x/ansi.Convert256 is not this function: it picks between the
// same two candidates by HSLuv distance, so it disagrees on some colors
// (#e6edf3 is 255 here and 195 there).
func Convert256(r, g, b uint8) uint8 {
	v2ci := func(v int) int {
		switch {
		case v < 48:
			return 0
		case v < 115:
			return 1
		}
		return (v - 35) / 40
	}
	R, G, B := int(r), int(g), int(b)
	qr, qg, qb := v2ci(R), v2ci(G), v2ci(B)
	cr, cg, cb := cubeLevels[qr], cubeLevels[qg], cubeLevels[qb]
	cube := 16 + 36*qr + 6*qg + qb
	if cr == R && cg == G && cb == B {
		return uint8(cube)
	}
	avg := (R + G + B) / 3
	gi := 23
	if avg <= 238 {
		gi = (avg - 3) / 10 // Go truncates toward zero: avg < 3 gives 0
	}
	grey := 8 + 10*gi
	if dist2(grey, grey, grey, R, G, B) < dist2(cr, cg, cb, R, G, B) {
		return uint8(232 + gi)
	}
	return uint8(cube)
}

// xtermPalette is the xterm default palette of the 16 ANSI colors.
var xtermPalette = [16][3]int{
	{0x00, 0x00, 0x00}, {0xcd, 0x00, 0x00}, {0x00, 0xcd, 0x00}, {0xcd, 0xcd, 0x00},
	{0x00, 0x00, 0xee}, {0xcd, 0x00, 0xcd}, {0x00, 0xcd, 0xcd}, {0xe5, 0xe5, 0xe5},
	{0x7f, 0x7f, 0x7f}, {0xff, 0x00, 0x00}, {0x00, 0xff, 0x00}, {0xff, 0xff, 0x00},
	{0x5c, 0x5c, 0xff}, {0xff, 0x00, 0xff}, {0x00, 0xff, 0xff}, {0xff, 0xff, 0xff},
}

// Convert16 maps an RGB color to the ANSI index 0-15 of the nearest color
// of the xterm default palette (SPEC v0.2 §26.3, C16): squared RGB
// distance, ties to the lower index.
func Convert16(r, g, b uint8) uint8 {
	best, bestD := 0, -1
	for i, c := range xtermPalette {
		if d := dist2(c[0], c[1], c[2], int(r), int(g), int(b)); bestD < 0 || d < bestD {
			best, bestD = i, d
		}
	}
	return uint8(best)
}

// ansiCode is the SGR code of ANSI color i (0-15) as a foreground, or as a
// background when bg is set.
func ansiCode(i int, bg bool) string {
	base := 30
	if bg {
		base = 40
	}
	if i >= 8 {
		return strconv.Itoa(base + 60 + i - 8)
	}
	return strconv.Itoa(base + i)
}

// colorCode is the SGR parameter string that selects c as the foreground
// (or the background when bg is set) in profile p, per the table of SPEC
// v0.2 §26.3. It is "" in the none profile, where colors are omitted.
func colorCode(c css.Color, bg bool, p Profile) string {
	if p == NoColor {
		return ""
	}
	switch c.Kind {
	case css.ColorANSI:
		return ansiCode(int(c.Index), bg)
	case css.ColorRGB:
		lead := "38"
		if bg {
			lead = "48"
		}
		switch p {
		case ANSI256:
			return lead + ";5;" + strconv.Itoa(int(Convert256(c.R, c.G, c.B)))
		case ANSI16:
			return ansiCode(int(Convert16(c.R, c.G, c.B)), bg)
		}
		return lead + ";2;" + strconv.Itoa(int(c.R)) + ";" + strconv.Itoa(int(c.G)) + ";" + strconv.Itoa(int(c.B))
	}
	if bg {
		return "49"
	}
	return "39"
}
