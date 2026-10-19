package kit

import (
	"testing"
	"time"
)

func TestBytes(t *testing.T) {
	for n, want := range map[int64]string{
		0: "0 B", 999: "999 B", 1023: "1023 B", 1024: "1.0 KiB", 1536: "1.5 KiB",
		1048576: "1.0 MiB", 5 << 20: "5.0 MiB", 150 << 20: "150 MiB",
		3 << 30: "3.0 GiB", 1<<20 - 1: "1.0 MiB", -2048: "-2.0 KiB",
	} {
		if got := Bytes(n); got != want {
			t.Errorf("Bytes(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{
		0: "0ms", 450 * time.Millisecond: "450ms", time.Second: "1s", 59 * time.Second: "59s",
		2*time.Minute + 5*time.Second: "2m 5s", 2 * time.Minute: "2m",
		time.Hour + 5*time.Minute: "1h 5m", 3 * time.Hour: "3h",
		76 * time.Hour: "3d 4h", -90 * time.Second: "-1m 30s",
	} {
		if got := Duration(d); got != want {
			t.Errorf("Duration(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestRelative(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	for off, want := range map[time.Duration]string{
		-3 * time.Second: "just now", 5 * time.Second: "just now",
		-30 * time.Second: "30 seconds ago", -time.Minute: "1 minute ago", -5 * time.Minute: "5 minutes ago",
		-time.Hour: "1 hour ago", -23 * time.Hour: "23 hours ago", -48 * time.Hour: "2 days ago",
		-45 * 24 * time.Hour: "1 month ago", -400 * 24 * time.Hour: "1 year ago",
		2 * time.Hour: "in 2 hours", 24 * time.Hour: "in 1 day",
	} {
		if got := Relative(now.Add(off), now); got != want {
			t.Errorf("Relative(now%+v) = %q, want %q", off, got, want)
		}
	}
}

func TestPercentAndThousands(t *testing.T) {
	if got := Percent(0.256, 0); got != "26%" {
		t.Errorf("Percent(0.256, 0) = %q", got)
	}
	if got := Percent(0.256, 1); got != "25.6%" {
		t.Errorf("Percent(0.256, 1) = %q", got)
	}
	if got := Percent(1, -1); got != "100%" {
		t.Errorf("Percent(1, -1) = %q", got)
	}
	for n, want := range map[int64]string{
		0: "0", 12: "12", 123: "123", 1234: "1,234", 12345: "12,345", 123456: "123,456",
		1234567: "1,234,567", -1234567: "-1,234,567", -999: "-999",
	} {
		if got := Thousands(n); got != want {
			t.Errorf("Thousands(%d) = %q, want %q", n, got, want)
		}
	}
}
