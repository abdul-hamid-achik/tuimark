package play

import (
	"fmt"
	"math/big"
	"time"
)

// The clock step tick:DUR (SPEC v0.4b §15.4.1). DUR is a subset of Go's
// duration syntax, parsed here by hand: time.ParseDuration reports an
// overflow where the SPEC says "at most 1h", and accepts forms the
// grammar rejects (.5s, 250us).
//
//	DUR    = 1*( NUMBER UNIT )
//	NUMBER = 1*DIGIT [ "." 1*DIGIT ]
//	UNIT   = "ms" / "s" / "m" / "h"

const (
	minTick = time.Millisecond
	maxTick = time.Hour
)

// TickStep builds the tick step whose DUR is dur and whose text as
// written is raw: the --input token, the --script line, or one of Play's
// steps. It is the one place the grammar and its reasons live, so play,
// test, tuimark_play, and Play agree (SPEC §15.4.1).
func TickStep(raw, dur string) (Step, error) {
	d, err := ParseDur(dur)
	if err != nil {
		return Step{}, err
	}
	return Step{Kind: KindTick, Raw: raw, Dur: d}, nil
}

// ParseDur parses the DUR of a tick step. The checks run in this order
// and the first that fails gives the reason: the grammar, whole
// milliseconds, at least 1ms, at most 1h. The sum is exact (big.Rat), so a
// grammar-valid 99999999999999999999h is "at most 1h".
func ParseDur(text string) (time.Duration, error) {
	total, ok := parseDurMillis(text)
	if !ok {
		return 0, fmt.Errorf("tick: %q is not a duration (want a number and a unit ms, s, m, or h, such as 250ms)", text)
	}
	if !total.IsInt() {
		return 0, fmt.Errorf("tick: duration must be a whole number of milliseconds")
	}
	if total.Cmp(big.NewRat(int64(minTick/time.Millisecond), 1)) < 0 {
		return 0, fmt.Errorf("tick: duration must be at least 1ms")
	}
	if total.Cmp(big.NewRat(int64(maxTick/time.Millisecond), 1)) > 0 {
		return 0, fmt.Errorf("tick: duration must be at most 1h")
	}
	return time.Duration(total.Num().Int64()) * time.Millisecond, nil
}

// parseDurMillis parses text by the grammar above and returns the sum of
// its parts in milliseconds, or false when text does not match it.
func parseDurMillis(text string) (*big.Rat, bool) {
	total := new(big.Rat)
	i := 0
	for parts := 0; ; parts++ {
		if i == len(text) {
			return total, parts > 0
		}
		start := i
		for i < len(text) && isDigit(text[i]) {
			i++
		}
		if i == start {
			return nil, false
		}
		if i < len(text) && text[i] == '.' {
			i++
			frac := i
			for i < len(text) && isDigit(text[i]) {
				i++
			}
			if i == frac {
				return nil, false
			}
		}
		num, ok := new(big.Rat).SetString(text[start:i])
		if !ok {
			return nil, false
		}
		var unit int64
		switch {
		case hasPrefixAt(text, i, "ms"): // before "m"
			unit, i = 1, i+2
		case hasPrefixAt(text, i, "s"):
			unit, i = 1000, i+1
		case hasPrefixAt(text, i, "m"):
			unit, i = 60000, i+1
		case hasPrefixAt(text, i, "h"):
			unit, i = 3600000, i+1
		default:
			return nil, false
		}
		total.Add(total, num.Mul(num, new(big.Rat).SetInt64(unit)))
	}
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func hasPrefixAt(s string, i int, prefix string) bool {
	return len(s)-i >= len(prefix) && s[i:i+len(prefix)] == prefix
}
