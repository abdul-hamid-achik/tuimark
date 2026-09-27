package host

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/abdul-hamid-achik/tuimark/internal/paint"
)

// The environment variables Run reads (SPEC v0.2 §26.10). Dump, Validate,
// and the deterministic CLI commands never read them.
const (
	envColor     = "TUIMARK_COLOR" // truecolor | 256 | 16 | none
	envTheme     = "TUIMARK_THEME" // dark | light
	envSync      = "TUIMARK_SYNC"  // 0 | 1
	envLog       = "TUIMARK_LOG"   // a file path (SPEC v0.2b §26.12)
	envColorFGBG = "COLORFGBG"     // the theme="auto" fallback (SPEC v0.2b §26.4)
)

// Terminal sequences of a Run session (SPEC v0.2 §26.9).
const (
	// enterScreen enters the alternate screen, hides the cursor, turns
	// autowrap (DECAWM) off so a cluster the terminal draws wider than
	// Tuimark measured can never wrap or scroll the screen (paint.AutowrapOff),
	// clears, and turns bracketed paste on.
	enterScreen = "\x1b[?1049h\x1b[?25l\x1b[?7l\x1b[H\x1b[2J\x1b[?2004h"
	// leaveScreen turns bracketed paste off, resets attributes, shows the
	// cursor, turns autowrap back on (paint.AutowrapOn), and leaves the
	// alternate screen. Mode 2027 is turned off
	// before it when the session turned it on (termSession.leave).
	leaveScreen   = "\x1b[?2004l\x1b[0m\x1b[?25h\x1b[?7h\x1b[?1049l"
	graphemeOn    = "\x1b[?2027h"
	graphemeOff   = "\x1b[?2027l"
	syncBegin     = "\x1b[?2026h"
	syncEnd       = "\x1b[?2026l"
	querySync     = "\x1b[?2026$p" // DECRQM: synchronized output
	queryGrapheme = "\x1b[?2027$p" // DECRQM: grapheme clustering
	queryDA1      = "\x1b[c"       // DA1, the probe's sentinel
	// queryBackground asks for the terminal's background color, OSC 11
	// (SPEC v0.2b §26.2); only when the theme is resolved from the
	// terminal (theme="auto").
	queryBackground = "\x1b]11;?\a"
	// mouseModesOn and mouseModesOff turn mode 1000 (presses, releases,
	// and the wheel) and mode 1006 (SGR encoding) on and off together
	// (SPEC v0.2b §26.11). Mode 1003 (motion) is never written.
	mouseModesOn  = "\x1b[?1000h\x1b[?1006h"
	mouseModesOff = "\x1b[?1000l\x1b[?1006l"
)

// probeWait is how long the capability probe waits for its DA1 sentinel
// (SPEC v0.2 §26.2). Tests shorten or lengthen it.
var probeWait = 250 * time.Millisecond

// syncMode is TUIMARK_SYNC: unset (the probe decides), 0, or 1.
type syncMode uint8

const (
	syncAuto syncMode = iota
	syncOff
	syncOn
)

// runConfig is what Run reads from the environment before it touches the
// terminal (SPEC v0.2 §26.1 step 1).
type runConfig struct {
	profile paint.Profile
	theme   string // "" keeps the document's theme
	sync    syncMode
	fgbg    string // the theme COLORFGBG gives, "" for none (§26.4)
	log     string // TUIMARK_LOG, "" for no log (§26.12)
}

// readRunConfig reads and checks TUIMARK_COLOR, TUIMARK_THEME, and
// TUIMARK_SYNC, and detects the color profile. An empty value is the same
// as unset; values are exact and case-sensitive; an invalid value is an
// error (a usage error: Run returns it before touching the terminal).
func readRunConfig(getenv func(string) string) (runConfig, error) {
	var cfg runConfig
	p, err := DetectColorProfile("", getenv)
	if err != nil {
		return cfg, fmt.Errorf("tuimark: %v", err)
	}
	cfg.profile = p
	switch v := getenv(envTheme); v {
	case "", "dark", "light":
		cfg.theme = v
	default:
		return cfg, fmt.Errorf("tuimark: %s=%q: want dark or light", envTheme, v)
	}
	switch v := getenv(envSync); v {
	case "":
		cfg.sync = syncAuto
	case "0":
		cfg.sync = syncOff
	case "1":
		cfg.sync = syncOn
	default:
		return cfg, fmt.Errorf("tuimark: %s=%q: want 0 or 1", envSync, v)
	}
	// COLORFGBG has no invalid value: anything else gives no theme.
	// TUIMARK_LOG is any path; only a path that cannot be opened fails
	// (openRunLog).
	cfg.fgbg = ThemeFromCOLORFGBG(getenv(envColorFGBG))
	cfg.log = getenv(envLog)
	return cfg, nil
}

// DetectColorProfile is the color profile detection of SPEC v0.2 §26.3,
// the one Run performs once per session, shared with the CLI's preview
// (once per run, when it paints ANSI). flag is preview's --color value, ""
// when the flag is absent (Run passes ""); when set it wins and getenv is
// not called. Otherwise the first match wins: TUIMARK_COLOR, NO_COLOR,
// WT_SESSION, TERM empty or dumb, COLORTERM, TERM (paint.DetectProfile),
// read through getenv (os.Getenv in production). An invalid flag or
// TUIMARK_COLOR value (exact and case-sensitive: truecolor, 256, 16, none)
// is an error naming it, a usage error for the caller. Nothing else is
// read and no terminal is queried. internal/host is not public API.
func DetectColorProfile(flag string, getenv func(string) string) (paint.Profile, error) {
	if flag != "" {
		p, err := paint.ParseProfile(flag)
		if err != nil {
			return paint.TrueColor, fmt.Errorf("--color=%q: want truecolor, 256, 16, or none", flag)
		}
		return p, nil
	}
	return paint.DetectProfile(getenv)
}

// probeQueries returns the probe's queries, in one string for one write
// (SPEC v0.2 §26.2): DECRQM 2026 (left out when TUIMARK_SYNC is set),
// DECRQM 2027, then the DA1 sentinel.
func probeQueries(s syncMode) string {
	q := ""
	if s == syncAuto {
		q += querySync
	}
	return q + queryGrapheme + queryDA1
}

// probePlan decides the probe of SPEC v0.2b §26.2: whether it runs, and
// whether the skip rule leaves the DECRQM queries out. themeQuery is set
// when the theme is resolved from the terminal: the write then starts
// with OSC 11. The probe runs when the output is a terminal, TERM is not
// dumb, and at least one query other than DA1 is to be sent: under the
// Apple Terminal/SSH skip rule that is only OSC 11 (so a skipped terminal
// gets OSC 11 and DA1, and only for theme="auto").
func probePlan(getenv func(string) string, outTTY bool, s syncMode, themeQuery bool) (probe, skipDECRQM bool) {
	if !outTTY || getenv("TERM") == "dumb" {
		return false, false
	}
	decrqm := shouldProbe(getenv, outTTY, s)
	return decrqm || themeQuery, !decrqm
}

// knownProbeTerms are TERM substrings of terminals the probe skip rule
// exempts (after Bubble Tea v2.0.10's shouldQuerySynchronizedOutput).
var knownProbeTerms = []string{"ghostty", "wezterm", "alacritty", "kitty", "rio"}

// shouldProbe reports whether Run runs the capability probe (SPEC v0.2
// §26.2): the output is a terminal, TERM is not dumb, at least one DECRQM
// query is to be sent, and the skip rule does not apply. The skip rule
// skips Apple Terminal (TERM_PROGRAM contains Apple) and SSH sessions
// (SSH_TTY set), unless TERM names ghostty, wezterm, alacritty, kitty, or
// rio, or WT_SESSION is set. Without the probe nothing is written and
// every capability is "not supported" unless the environment forces it.
func shouldProbe(getenv func(string) string, outTTY bool, s syncMode) bool {
	if !outTTY || getenv("TERM") == "dumb" {
		return false
	}
	if !strings.Contains(probeQueries(s), "$p") {
		return false // only DA1 would be sent: never alone
	}
	if strings.Contains(getenv("TERM_PROGRAM"), "Apple") || getenv("SSH_TTY") != "" {
		if getenv("WT_SESSION") != "" {
			return true
		}
		termName := getenv("TERM")
		for _, t := range knownProbeTerms {
			if strings.Contains(termName, t) {
				return true
			}
		}
		return false
	}
	return true
}

// termCaps is what the probe found (SPEC v0.2 §26.2). A missing reply
// means "not supported".
type termCaps struct {
	sync     int    // Ps of the DECRPM reply for mode 2026 (0: none)
	grapheme int    // Ps of the DECRPM reply for mode 2027 (0: none)
	bg       string // the background of the first OSC 11 reply: dark, light, or "" (none)
	done     bool   // the DA1 sentinel arrived: later replies are ignored
}

// record takes one reply; it reports whether the probe is over (the DA1
// sentinel arrived). Only the first OSC 11 reply whose SPEC is rgb: or
// rgba: records the background (SPEC v0.2b §26.2).
func (c *termCaps) record(r reply) bool {
	if c.done {
		return true
	}
	switch {
	case r.da1:
		c.done = true
	case r.osc11:
		if c.bg == "" {
			c.bg = r.bg
		}
	case r.mode == 2026:
		c.sync = r.value
	case r.mode == 2027:
		c.grapheme = r.value
	}
	return c.done
}

// graphemeState names mode 2027's state for the caps record of
// TUIMARK_LOG (SPEC v0.2b §26.12): enabled (Run turned it on), already
// (it was on), or none.
func (c termCaps) graphemeState() string {
	switch {
	case c.turnGraphemeOn():
		return "enabled"
	case c.graphemeActive():
		return "already"
	}
	return "none"
}

// syncSupported: DEC 2026 is used for Ps 1 or 2 only.
func (c termCaps) syncSupported() bool { return c.sync == 1 || c.sync == 2 }

// turnGraphemeOn: Ps = 2 means supported and off, so Run turns mode 2027
// on (and off when it leaves).
func (c termCaps) turnGraphemeOn() bool { return c.grapheme == 2 }

// graphemeActive: mode 2027 is on for the session (Ps 1 or 3: already on;
// Ps 2: Run turns it on), so frames need no CHA re-positioning.
func (c termCaps) graphemeActive() bool { return c.grapheme >= 1 && c.grapheme <= 3 }

// termSession is one terminal session of the loop: what Run decided
// before the loop starts, and the state its restore paths share. Loop's
// zero session is the v0.1 behavior plus bracketed paste: truecolor, no
// probe, no synchronized output, CHA re-positioning on.
type termSession struct {
	profile paint.Profile
	probe   bool          // run the capability probe (§26.2)
	sync    syncMode      // TUIMARK_SYNC
	wait    time.Duration // the probe's cap; probeWait when 0
	// themeQuery puts OSC 11 at the start of the probe (the theme is
	// resolved from the terminal, SPEC v0.2b §26.2); skipDECRQM leaves the
	// DECRQM queries out (the skip rule applies).
	themeQuery bool
	skipDECRQM bool
	fgbg       string  // the theme COLORFGBG gives, "" for none
	log        *runLog // TUIMARK_LOG, nil for none
	eof        bool    // the session ended because the input ended

	mu       sync.Mutex
	grapheme bool // mode 2027 was turned on: leaving turns it off
	mouse    bool // the mouse modes are on: leaving turns them off
}

// leave is the leave sequence: mode 2027 off when the session turned it
// on, the mouse modes off while they are on (SPEC v0.2b §26.1), then
// leaveScreen. The loop's own way out and the signal restore
// path both write it.
func (s *termSession) leave() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := ""
	if s.grapheme {
		out += graphemeOff
	}
	if s.mouse {
		out += mouseModesOff
	}
	return out + leaveScreen
}

// mouseChange returns the sequence that brings the terminal's mouse
// modes to on, or "" when they are already there, and records the new
// state (SPEC v0.2b §26.11). It is recorded before the sequence is
// written, so a restore that races with the write still turns them off.
func (s *termSession) mouseChange(on bool) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mouse == on {
		return ""
	}
	s.mouse = on
	if on {
		return mouseModesOn
	}
	return mouseModesOff
}

// setGrapheme records that mode 2027 is being turned on. It is recorded
// before the sequence is written, so a restore that races with the write
// still turns it off.
func (s *termSession) setGrapheme() {
	s.mu.Lock()
	s.grapheme = true
	s.mu.Unlock()
}

// queries is the probe's write, in one string (SPEC v0.2b §26.2): OSC 11
// when the theme is resolved from the terminal, then the DECRQM queries
// unless the skip rule leaves them out, then the DA1 sentinel.
func (s *termSession) queries() string {
	q := ""
	if s.themeQuery {
		q = queryBackground
	}
	if s.skipDECRQM {
		return q + queryDA1
	}
	return q + probeQueries(s.sync)
}

// probeCap is the probe's wait.
func (s *termSession) probeCap() time.Duration {
	if s.wait > 0 {
		return s.wait
	}
	return probeWait
}

// syncFrames reports whether frames are wrapped in synchronized output
// (SPEC v0.2 §26.6): TUIMARK_SYNC=1, or unset and the probe found 2026.
func (s *termSession) syncFrames(c termCaps) bool {
	return s.sync == syncOn || (s.sync == syncAuto && c.syncSupported())
}
