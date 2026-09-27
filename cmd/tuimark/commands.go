package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/abdul-hamid-achik/tuimark/internal/dump"
	"github.com/abdul-hamid-achik/tuimark/internal/host"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/paint"
)

// parseArgs parses flags that may appear before or after positionals.
func parseArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return pos, nil
		}
		pos = append(pos, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

func (c *cli) newFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	return fs
}

func oneFile(name string, pos []string) (string, error) {
	if len(pos) != 1 {
		return "", fmt.Errorf("tuimark %s: want exactly one FILE, got %d", name, len(pos))
	}
	return pos[0], nil
}

// load reads the document and binds --data (if any) as the store root.
func load(file, data string) (*host.App, error) {
	app, err := host.Load(file)
	if err != nil {
		return nil, err
	}
	if data != "" {
		raw, err := os.ReadFile(data)
		if err != nil {
			return nil, err
		}
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, fmt.Errorf("%s: %w", data, err)
		}
		if err := app.Bind("", v); err != nil {
			return nil, fmt.Errorf("%s: %w", data, err)
		}
	}
	return app, nil
}

func (c *cli) fail(err error) int {
	fmt.Fprintln(c.stderr, "tuimark:", err)
	return 1
}

func checkSize(cols, rows int) error {
	if cols <= 0 || rows <= 0 || cols > 1000 || rows > 1000 {
		return fmt.Errorf("--cols and --rows must be between 1 and 1000 (got %dx%d)", cols, rows)
	}
	return nil
}

func (c *cli) cmdDump(args []string) int {
	fs := c.newFlags("dump")
	cols := fs.Int("cols", 80, "terminal columns")
	rows := fs.Int("rows", 24, "terminal rows")
	format := fs.String("format", "text", "text or json")
	data := fs.String("data", "", "JSON file bound as the data store")
	cells := fs.Bool("cells", false, "include per-cell ownership (json only)")
	strict := fs.Bool("strict", false, "treat missing bind paths (B003) as errors")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return 1
	}
	file, err := oneFile("dump", pos)
	if err != nil {
		return c.fail(err)
	}
	if err := checkSize(*cols, *rows); err != nil {
		return c.fail(err)
	}
	if *format != "text" && *format != "json" {
		return c.fail(fmt.Errorf("--format must be text or json"))
	}
	app, err := load(file, *data)
	if err != nil {
		return c.fail(err)
	}
	app.SetStrict(*strict)
	d := app.Dump(*cols, *rows, *cells)
	if *format == "json" {
		b, err := dump.JSON(d)
		if err != nil {
			return c.fail(err)
		}
		c.stdout.Write(b)
	} else {
		fmt.Fprint(c.stdout, dump.Text(d))
	}
	if !d.OK {
		return 2
	}
	return 0
}

// readCatalog accepts ["a","b"], [{"name":"a"}], or {"actions": either}.
func readCatalog(path string) (map[string]bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if m, ok := v.(map[string]any); ok {
		v = m["actions"]
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("%s: want a JSON array of action names (or {\"actions\": [...]})", path)
	}
	known := map[string]bool{}
	for _, e := range arr {
		switch x := e.(type) {
		case string:
			known[x] = true
		case map[string]any:
			if n, ok := x["name"].(string); ok {
				known[n] = true
			}
		}
	}
	return known, nil
}

func (c *cli) cmdValidate(args []string) int {
	fs := c.newFlags("validate")
	asJSON := fs.Bool("json", false, "print diagnostics as JSON")
	strict := fs.Bool("strict", false, "treat missing bind paths (B003) as errors")
	catalog := fs.String("catalog", "", "JSON list of host actions; unknown actions warn B004")
	data := fs.String("data", "", "JSON file bound as the data store (enables bind checks)")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return 1
	}
	file, err := oneFile("validate", pos)
	if err != nil {
		return c.fail(err)
	}
	app, err := load(file, *data)
	if err != nil {
		return c.fail(err)
	}
	app.SetStrict(*strict)
	diags := app.Validate()
	if *data == "" {
		// Without data every bind path is "missing"; report only data-free checks.
		var kept ir.Diags
		for _, d := range diags {
			if d.Code == "B001" || d.Code == "B002" || d.Code == "B003" {
				continue
			}
			kept = append(kept, d)
		}
		diags = kept
	}
	if *catalog != "" {
		known, err := readCatalog(*catalog)
		if err != nil {
			return c.fail(err)
		}
		diags = append(diags, app.CheckCatalog(known)...).Sorted()
	}
	ok := !diags.HasErrors()
	if *asJSON {
		out := map[string]any{"ok": ok, "file": file, "diagnostics": diagsOrEmpty(diags), "actions": app.Catalog()}
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Fprintln(c.stdout, string(b))
	} else {
		for _, d := range diags {
			fmt.Fprintln(c.stdout, d.String())
		}
		if ok {
			fmt.Fprintln(c.stdout, "ok")
		}
	}
	if !ok {
		return 2
	}
	return 0
}

func diagsOrEmpty(d ir.Diags) []ir.Diagnostic {
	if d == nil {
		return []ir.Diagnostic{}
	}
	return d
}

func isTTY(f *os.File) bool { return term.IsTerminal(int(f.Fd())) }

// renderPreview writes one preview: the text dump, plus an ANSI-painted
// grid first when stdout is a TTY (SPEC §15: "writes the text dump to
// stdout. If stdout is a TTY, it MAY also paint ANSI" — an addition, not a
// replacement, so the plain `=== grid ===` section is always present).
func renderPreview(w io.Writer, app *host.App, cols, rows int, tty bool) bool {
	if !tty {
		d := app.Dump(cols, rows, false)
		fmt.Fprint(w, dump.Text(d))
		return d.OK
	}
	f := app.Frame(cols, rows)
	d := dump.Build(cols, rows, f.Root, f.Modals, f.Grid, f.Diags, f.Focus, false)
	fmt.Fprint(w, paint.Full(f.Grid))
	fmt.Fprint(w, dump.Text(d))
	return d.OK
}

func (c *cli) cmdPreview(args []string) int {
	fs := c.newFlags("preview")
	cols := fs.Int("cols", 80, "terminal columns")
	rows := fs.Int("rows", 24, "terminal rows")
	data := fs.String("data", "", "JSON file bound as the data store")
	watch := fs.Bool("watch", false, "re-render when the document, stylesheets, or data change")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return 1
	}
	file, err := oneFile("preview", pos)
	if err != nil {
		return c.fail(err)
	}
	if err := checkSize(*cols, *rows); err != nil {
		return c.fail(err)
	}
	tty := c.stdout == io.Writer(os.Stdout) && isTTY(os.Stdout)
	app, err := load(file, *data)
	if err != nil {
		return c.fail(err)
	}
	if tty && *watch {
		fmt.Fprint(c.stdout, "\x1b[H\x1b[2J")
	}
	ok := renderPreview(c.stdout, app, *cols, *rows, tty)
	if !*watch {
		if !ok {
			return 2
		}
		return 0
	}
	files := append(app.Files(), *data)
	stamp := fingerprint(files)
	for n := 1; ; n++ {
		time.Sleep(150 * time.Millisecond)
		now := fingerprint(files)
		if now == stamp {
			continue
		}
		stamp = now
		next, err := load(file, *data)
		if tty {
			fmt.Fprint(c.stdout, "\x1b[H\x1b[2J")
		} else {
			fmt.Fprintf(c.stdout, "=== reload %d ===\n", n)
		}
		if err != nil {
			fmt.Fprintln(c.stdout, "tuimark:", err)
			continue
		}
		app = next
		files = append(app.Files(), *data)
		stamp = fingerprint(files)
		renderPreview(c.stdout, app, *cols, *rows, tty)
	}
}

// fingerprint summarizes size+mtime of the watched files.
func fingerprint(files []string) string {
	var b strings.Builder
	for _, f := range files {
		if f == "" {
			continue
		}
		st, err := os.Stat(f)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				b.WriteString(f + ":missing;")
			}
			continue
		}
		fmt.Fprintf(&b, "%s:%d:%d;", f, st.Size(), st.ModTime().UnixNano())
	}
	return b.String()
}
