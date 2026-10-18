package main

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
)

// Log line kinds. The document colors a line by kind with class: guards.
const (
	kCmd  = "cmd"
	kInfo = "info"
	kOK   = "ok"
	kWarn = "warn"
	kErr  = "err"
	kDim  = "dim"
)

// plannedLine is one line a job will print, At seconds after it starts.
type plannedLine struct {
	At   float64
	Kind string
	Text string
}

// plan is everything a job attempt will do, decided when the attempt
// starts from a seed that depends only on (seed, job, attempt): how long
// it runs, where it fails (0 for never), and every log line with its time.
// Nothing here reads a clock or a global random source, so a retry or a
// rerun gives the same plan however many ticks came before.
type plan struct {
	Dur    float64
	FailAt float64
	Lines  []plannedLine
}

// spec describes one job of the pipeline.
type spec struct {
	ID    string
	Stage string
	Base  float64 // typical duration in simulated seconds
	Kind  string  // which log script it prints
	Flaky bool    // fails on its first attempt when the run is flaky
}

// stages run one after the other; the jobs of a stage run in parallel.
var stages = []string{"build", "test", "lint", "package", "deploy"}

var specs = []spec{
	{ID: "compile", Stage: "build", Base: 6, Kind: "compile"},
	{ID: "assets", Stage: "build", Base: 5, Kind: "assets"},
	{ID: "unit", Stage: "test", Base: 10, Kind: "unit"},
	{ID: "integration", Stage: "test", Base: 16, Kind: "integration", Flaky: true},
	{ID: "e2e", Stage: "test", Base: 18, Kind: "e2e"},
	{ID: "fmt", Stage: "lint", Base: 3, Kind: "fmt"},
	{ID: "vet", Stage: "lint", Base: 5, Kind: "vet"},
	{ID: "staticcheck", Stage: "lint", Base: 7, Kind: "staticcheck"},
	{ID: "docker", Stage: "package", Base: 9, Kind: "docker"},
	{ID: "binaries", Stage: "package", Base: 7, Kind: "binaries"},
	{ID: "deploy", Stage: "deploy", Base: 8, Kind: "deploy"},
}

func stageIndex(name string) int {
	for i, s := range stages {
		if s == name {
			return i
		}
	}
	return -1
}

// round1 rounds to one decimal, so the store holds short, stable numbers.
func round1(x float64) float64 { return math.Round(x*10) / 10 }

// attemptRand is the random source of one attempt of one job.
func attemptRand(seed int64, idx, try int) *rand.Rand {
	return rand.New(rand.NewSource(seed*1000003 + int64(idx+1)*7919 + int64(try)*104729))
}

// pickupDelay is how long a job that became eligible waits for a runner.
func pickupDelay(seed int64, idx, try int) float64 {
	r := rand.New(rand.NewSource(seed*999983 + int64(idx+1)*6151 + int64(try)*31))
	return round1(0.5 + r.Float64()*1.0)
}

// builder collects the lines of one plan.
type builder struct {
	r     *rand.Rand
	lines []plannedLine
}

func (b *builder) add(at float64, kind, format string, args ...any) {
	b.lines = append(b.lines, plannedLine{At: round1(at), Kind: kind, Text: fmt.Sprintf(format, args...)})
}

// spread prints one line per item, spaced over [from, to] with a little
// jitter, in order.
func (b *builder) spread(items []string, kind string, from, to float64, format string) {
	n := float64(len(items))
	for i, it := range items {
		pos := (float64(i) + 0.5 + (b.r.Float64()-0.5)*0.5) / n
		b.add(from+(to-from)*pos, kind, format, it)
	}
}

// ms is a short test duration like 0.412s.
func (b *builder) secs() string { return fmt.Sprintf("%.3fs", 0.05+b.r.Float64()*0.9) }

var goPackages = []string{
	"internal/ir", "internal/parse", "internal/css", "internal/layout",
	"internal/paint", "internal/dump", "internal/host", "internal/play",
	"internal/catalog", "internal/lsp", "cmd/tuimark", "tuimark",
}

var hostTests = []string{
	"TestHostRoundTrip", "TestHostBatchAtomic", "TestHostDeriveOrder", "TestHostEveryTicks",
	"TestHostPlayParity", "TestHostMouseClick", "TestHostFocusCycle", "TestHostTabsBind",
	"TestHostReconnect", "TestHostReplyTimeout", "TestHostRedactSecret", "TestHostLogRotate",
}

var e2eSpecs = []string{
	"inbox_open", "inbox_search", "dashboard_deploy", "agent_prompt", "monitor_sort",
	"tasks_move", "logs_follow", "gitui_stage", "http_send", "preview_watch",
	"host_ts_events", "resize_reflow",
}

// makePlan decides one attempt of a job. fail makes a flaky job fail.
func makePlan(seed int64, sp spec, idx, try int, fail bool) plan {
	r := attemptRand(seed, idx, try)
	dur := round1(sp.Base * (0.9 + 0.2*r.Float64()))
	if dur < 2 {
		dur = 2
	}
	b := &builder{r: r}
	p := plan{Dur: dur}
	switch sp.Kind {
	case "compile":
		b.add(0, kCmd, "$ go build -trimpath ./...")
		b.add(0.3, kDim, "go: downloading github.com/rivo/uniseg v0.4.7")
		pk := goPackages[:9]
		b.spread(pk, kInfo, 0.6, dur-0.8, "  compile %s")
		b.add(dur, kOK, "built %d packages in %.1fs", len(pk), dur)
	case "assets":
		b.add(0, kCmd, "$ bun run docs:build")
		b.add(0.4, kInfo, "vite v5.4 building for production")
		pages := []string{"index.md", "guide/install.md", "guide/layout.md", "reference/tags.md", "reference/css.md", "examples/index.md", "examples/logs.md"}
		b.spread(pages, kInfo, 0.8, dur-1.0, "  render docs/%s")
		b.add(dur, kOK, "dist/ written, %d pages", len(pages)+13)
	case "unit":
		b.add(0, kCmd, "$ go test -race -count=1 ./...")
		b.spread(goPackages, kOK, 0.5, dur-0.6, "ok    tuimark/%s")
		for i := range b.lines {
			if b.lines[i].Kind == kOK {
				b.lines[i].Text += "  " + b.secs()
			}
		}
		b.add(dur, kOK, "PASS: %d packages, %d tests", len(goPackages), 380+r.Intn(60))
	case "integration":
		b.add(0, kCmd, "$ go test -tags integration ./internal/host/...")
		k := indexOf(hostTests, "TestHostReconnect")
		if fail {
			failAt := round1(dur * (0.55 + 0.15*r.Float64()))
			if failAt < 3 {
				failAt = 3
			}
			p.FailAt = failAt
			for i := 0; i < k; i++ {
				at := 0.6 + (failAt-1.8)*(float64(i)+0.5)/float64(k)
				b.add(at, kOK, "--- PASS: %s (%s)", hostTests[i], b.secs())
			}
			b.add(failAt-0.8, kWarn, "host_test.go:88: reply took 1.9s (budget 2s)")
			b.add(failAt-0.6, kInfo, "=== RUN   %s", hostTests[k])
			b.add(failAt, kErr, "--- FAIL: %s (2.01s)", hostTests[k])
			b.add(failAt, kErr, "    host_test.go:142: timeout waiting for reply (seq 7)")
			b.add(failAt, kErr, "FAIL  tuimark/internal/host  %.2fs", failAt)
			b.add(failAt, kErr, "exit status 1")
			break
		}
		for i, t := range hostTests {
			at := 0.6 + (dur-1.8)*(float64(i)+0.5)/float64(len(hostTests))
			b.add(at, kOK, "--- PASS: %s (%s)", t, b.secs())
		}
		b.add(dur, kOK, "PASS: %d tests, 0 failures", len(hostTests))
	case "e2e":
		b.add(0, kCmd, "$ glyph run specs/glyphrun/*.yml")
		b.spread(e2eSpecs, kOK, 0.6, dur-0.8, "spec %s ... ok")
		for i := range b.lines {
			if b.lines[i].Kind == kOK {
				b.lines[i].Text += fmt.Sprintf(" (%.1fs)", 0.4+r.Float64()*1.2)
			}
		}
		b.add(dur, kOK, "%d specs, 0 failures", len(e2eSpecs))
	case "fmt":
		b.add(0, kCmd, "$ gofmt -l .")
		b.add(dur*0.6, kInfo, "checked %d files", 140+r.Intn(20))
		b.add(dur, kOK, "no files need formatting")
	case "vet":
		b.add(0, kCmd, "$ go vet ./...")
		b.spread(goPackages[:6], kDim, 0.4, dur-0.6, "# tuimark/%s")
		b.add(dur, kOK, "vet clean")
	case "staticcheck":
		b.add(0, kCmd, "$ staticcheck ./...")
		b.add(0.6, kInfo, "analyzing %d packages", len(goPackages))
		b.add(dur*0.7, kWarn, "internal/css/cascade.go:212: SA4006 value never used")
		b.add(dur, kOK, "0 errors, 1 warning")
	case "docker":
		b.add(0, kCmd, "$ docker build -t tuimark:a3f9c21 .")
		steps := []string{
			"FROM golang:1.22-alpine", "WORKDIR /src", "COPY go.mod go.sum ./",
			"RUN go mod download", "COPY . .", "RUN go build -o /out/tuimark ./cmd/tuimark",
			"FROM alpine:3.20", "COPY --from=build /out/tuimark /usr/bin/",
		}
		for i, s := range steps {
			steps[i] = fmt.Sprintf("[%d/%d] %s", i+1, len(steps), s)
		}
		b.spread(steps, kInfo, 0.5, dur-1.2, "%s")
		b.add(dur-0.6, kDim, "sha256:%012x", r.Int63n(1<<48))
		b.add(dur, kOK, "tagged tuimark:a3f9c21 (%.1f MB)", 17+r.Float64()*3)
	case "binaries":
		b.add(0, kCmd, "$ goreleaser build --snapshot --clean")
		targets := []string{"linux/amd64", "linux/arm64", "darwin/arm64", "darwin/amd64", "windows/amd64"}
		b.spread(targets, kInfo, 0.5, dur-0.8, "  building %s")
		b.add(dur, kOK, "%d binaries in dist/", len(targets))
	case "deploy":
		b.add(0, kCmd, "$ ./deploy.sh staging")
		b.add(0.6, kInfo, "pushing tuimark:a3f9c21 to registry")
		b.add(dur*0.4, kInfo, "rolling out web: 1/3 ready")
		b.add(dur*0.6, kInfo, "rolling out web: 2/3 ready")
		b.add(dur*0.8, kInfo, "rolling out web: 3/3 ready")
		b.add(dur-0.5, kInfo, "health check: 200 OK")
		b.add(dur, kOK, "deployed https://staging.example.com")
	}
	sort.SliceStable(b.lines, func(i, j int) bool { return b.lines[i].At < b.lines[j].At })
	for i := range b.lines {
		if b.lines[i].At > dur {
			b.lines[i].At = dur
		}
	}
	p.Lines = b.lines
	return p
}

func indexOf(xs []string, s string) int {
	for i, x := range xs {
		if x == s {
			return i
		}
	}
	return -1
}

// fmtClock is a pipeline-relative timestamp like 01:07.3.
func fmtClock(sec float64) string {
	if sec < 0 {
		sec = 0
	}
	// Round to tenths first, so 59.96 reads 01:00.0 and never 00:60.0.
	t := int(math.Round(sec * 10))
	return fmt.Sprintf("%02d:%02d.%d", t/600, t%600/10, t%10)
}

// fmtElapsed is a duration like 0:08.
func fmtElapsed(sec float64) string {
	s := int(sec)
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}
