package kit

import (
	"context"
	"errors"
	"testing"
)

func TestLoadWritesLoadingThenData(t *testing.T) {
	app := bareApp(t)
	l := NewLoader(app)
	release := make(chan struct{})
	task, err := l.Load("repos", func(ctx context.Context) (any, error) {
		<-release
		return []any{map[string]any{"name": "tuimark"}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !getBool(t, app, "repos.loading") || getStr(t, app, "repos.error") != "" {
		t.Fatal("while the work runs: loading true, no error")
	}
	if v := mustGet(t, app, "repos.data"); v != nil {
		t.Errorf("data before the result = %v, want null", v)
	}
	close(release)
	task.Wait()
	if getBool(t, app, "repos.loading") {
		t.Error("loading should be false after the result")
	}
	data := mustGet(t, app, "repos.data").([]any)
	if len(data) != 1 || data[0].(map[string]any)["name"] != "tuimark" {
		t.Errorf("data = %v", data)
	}
}

func TestLoadErrorKeepsTheOldData(t *testing.T) {
	app := bareApp(t)
	l := NewLoader(app)
	t1, _ := l.Load("x", func(context.Context) (any, error) { return "v1", nil })
	t1.Wait()
	t2, _ := l.Load("x", func(context.Context) (any, error) { return nil, errors.New("503 from upstream") })
	t2.Wait()
	if got := getStr(t, app, "x.error"); got != "503 from upstream" {
		t.Errorf("error = %q", got)
	}
	if getBool(t, app, "x.loading") || getStr(t, app, "x.data") != "v1" {
		t.Errorf("loading %v data %v: want false and the old data", getBool(t, app, "x.loading"), mustGet(t, app, "x.data"))
	}
	t3, _ := l.Load("x", func(context.Context) (any, error) { return "v3", nil })
	t3.Wait()
	if getStr(t, app, "x.error") != "" || getStr(t, app, "x.data") != "v3" {
		t.Error("a good run clears the error and replaces the data")
	}
}

func TestALaterLoadDropsTheStaleResult(t *testing.T) {
	app := bareApp(t)
	l := NewLoader(app)
	started := make(chan struct{})
	slow, _ := l.Load("q", func(ctx context.Context) (any, error) {
		close(started)
		<-ctx.Done() // canceled by the later Load, then returns a stale value anyway
		return "stale", nil
	})
	<-started
	fast, _ := l.Load("q", func(context.Context) (any, error) { return "fresh", nil })
	slow.Wait()
	fast.Wait()
	if got := getStr(t, app, "q.data"); got != "fresh" {
		t.Errorf("data = %q, want fresh", got)
	}
	if getBool(t, app, "q.loading") {
		t.Error("loading should be false")
	}
}

func TestCancelDropsTheResult(t *testing.T) {
	app := bareApp(t)
	l := NewLoader(app)
	started := make(chan struct{})
	task, _ := l.Load("c", func(ctx context.Context) (any, error) {
		close(started)
		<-ctx.Done()
		return "late", nil
	})
	<-started
	task.Cancel()
	task.Wait()
	if v := mustGet(t, app, "c.data"); v != nil {
		t.Errorf("data = %v, want null", v)
	}
	if getBool(t, app, "c.loading") {
		t.Error("a canceled load must not leave loading true")
	}
}

func TestLoadsOfDifferentPathsDoNotInterfere(t *testing.T) {
	app := bareApp(t)
	l := NewLoader(app)
	a, _ := l.Load("a", func(context.Context) (any, error) { return 1.0, nil })
	b, _ := l.Load("b", func(context.Context) (any, error) { return 2.0, nil })
	a.Wait()
	b.Wait()
	if mustGet(t, app, "a.data") != 1.0 || mustGet(t, app, "b.data") != 2.0 {
		t.Error("each path keeps its own result")
	}
}
