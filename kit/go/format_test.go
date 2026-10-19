package kit

import (
	"encoding/json"
	"math"
	"os"
	"testing"
	"time"
)

// The tables in ../testdata/format.json are shared with the TypeScript tests
// (kit/ts/format.test.ts): the same input must give the same string.
type formatFixture struct {
	Bytes []struct {
		In  json.Number `json:"in"`
		Out string      `json:"out"`
	} `json:"bytes"`
	DurationMs []struct {
		In  int64  `json:"in"`
		Out string `json:"out"`
	} `json:"durationMs"`
	RelativeSeconds []struct {
		In  int64  `json:"in"`
		Out string `json:"out"`
	} `json:"relativeSeconds"`
	Percent []struct {
		In       float64 `json:"in"`
		Decimals int     `json:"decimals"`
		Out      string  `json:"out"`
	} `json:"percent"`
	Thousands []struct {
		In  int64  `json:"in"`
		Out string `json:"out"`
	} `json:"thousands"`
}

func loadFormatFixture(t *testing.T) formatFixture {
	t.Helper()
	raw, err := os.ReadFile("../testdata/format.json")
	if err != nil {
		t.Fatal(err)
	}
	var fx formatFixture
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatal(err)
	}
	return fx
}

func TestFormatFixtures(t *testing.T) {
	fx := loadFormatFixture(t)
	for _, c := range fx.Bytes {
		n, err := c.In.Int64()
		if err != nil {
			t.Fatal(err)
		}
		if got := Bytes(n); got != c.Out {
			t.Errorf("Bytes(%d) = %q, want %q", n, got, c.Out)
		}
	}
	for _, c := range fx.DurationMs {
		if got := Duration(time.Duration(c.In) * time.Millisecond); got != c.Out {
			t.Errorf("Duration(%dms) = %q, want %q", c.In, got, c.Out)
		}
	}
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	for _, c := range fx.RelativeSeconds {
		if got := Relative(now.Add(time.Duration(c.In)*time.Second), now); got != c.Out {
			t.Errorf("Relative(now%+ds) = %q, want %q", c.In, got, c.Out)
		}
	}
	for _, c := range fx.Percent {
		if got := Percent(c.In, c.Decimals); got != c.Out {
			t.Errorf("Percent(%v, %d) = %q, want %q", c.In, c.Decimals, got, c.Out)
		}
	}
	for _, c := range fx.Thousands {
		if got := Thousands(c.In); got != c.Out {
			t.Errorf("Thousands(%d) = %q, want %q", c.In, got, c.Out)
		}
	}
}

func TestExtremesDoNotOverflowOrLoop(t *testing.T) {
	if got := Bytes(math.MinInt64); got != "-8.0 EiB" {
		t.Errorf("Bytes(MinInt64) = %q", got)
	}
	if got := Bytes(math.MaxInt64); got != "8.0 EiB" {
		t.Errorf("Bytes(MaxInt64) = %q", got)
	}
	if got := Duration(math.MinInt64); got == "" || got[0] != '-' {
		t.Errorf("Duration(MinInt64) = %q", got)
	}
	if got := Percent(math.NaN(), 0); got != "-" {
		t.Errorf("Percent(NaN) = %q", got)
	}
	if got := Percent(math.Inf(1), 0); got != "-" {
		t.Errorf("Percent(+Inf) = %q", got)
	}
	if got := Thousands(math.MinInt64); got != "-9,223,372,036,854,775,808" {
		t.Errorf("Thousands(MinInt64) = %q", got)
	}
}
