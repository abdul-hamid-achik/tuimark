package main

import (
	"fmt"
	"math"
	"strings"
)

// Derived paths, recomputed by the runtime inside every store write. The
// markup reads them like any other path; nothing here is ever Set.
//
//	lib_rows    the library after the filter, one row per track
//	queue_rows  the queue, with the track's title and artist
//	album_rows  one row per album
//	counts      the numbers the tab labels show
//	now         the now-playing fields: title, labels, position bar
//	vol         the volume bar and its label
//
// They read the host-written paths: player, queue, filter.
func (a *app) derive() error {
	ui := a.ui
	d := func(path string, fn func(v view) any) error {
		return ui.Derive(path, func(get func(string) (any, bool)) (any, error) {
			return fn(readView(get)), nil
		})
	}
	if err := d("lib_rows", func(v view) any { return libRows(v) }); err != nil {
		return err
	}
	if err := d("queue_rows", func(v view) any { return queueRows(v) }); err != nil {
		return err
	}
	if err := d("album_rows", func(v view) any { return albumRows(v) }); err != nil {
		return err
	}
	if err := d("counts", func(v view) any { return countsOf(v) }); err != nil {
		return err
	}
	if err := d("now", func(v view) any { return nowInfo(v) }); err != nil {
		return err
	}
	return d("vol", func(v view) any { return volInfo(v) })
}

// view is the host-written part of the store, read back as typed values.
type view struct {
	current string
	playing bool
	shuffle bool
	repeat  string
	volume  int
	posMs   int64
	queue   []qitem
	filter  string
}

func readView(get func(string) (any, bool)) view {
	var v view
	if x, ok := get("player"); ok {
		m, _ := x.(map[string]any)
		v.current, _ = m["current"].(string)
		v.playing, _ = m["playing"].(bool)
		v.shuffle, _ = m["shuffle"].(bool)
		v.repeat, _ = m["repeat"].(string)
		if f, ok := m["volume"].(float64); ok {
			v.volume = int(f)
		}
		if f, ok := m["pos"].(float64); ok {
			v.posMs = int64(math.Round(f * 1000))
		}
	}
	if x, ok := get("queue"); ok {
		list, _ := x.([]any)
		for _, it := range list {
			m, _ := it.(map[string]any)
			qid, _ := m["qid"].(string)
			id, _ := m["id"].(string)
			v.queue = append(v.queue, qitem{QID: qid, ID: id})
		}
	}
	if x, ok := get("filter"); ok {
		v.filter, _ = x.(string)
	}
	return v
}

// clock formats whole seconds as m:ss.
func clock(secs int) string { return fmt.Sprintf("%d:%02d", secs/60, secs%60) }

// matches reports whether every word of the filter occurs in the track's
// title, artist, or album (case-insensitively).
func matches(t track, filter string) bool {
	hay := strings.ToLower(t.Title + " " + t.Artist + " " + t.Album)
	for _, w := range strings.Fields(strings.ToLower(filter)) {
		if !strings.Contains(hay, w) {
			return false
		}
	}
	return true
}

func queued(v view, id string) bool {
	for _, q := range v.queue {
		if q.ID == id {
			return true
		}
	}
	return false
}

// libRows is the filtered library. mark is the first column: a play
// symbol on the current track, a dot on a queued one.
func libRows(v view) []any {
	rows := []any{}
	for _, t := range catalog {
		if !matches(t, v.filter) {
			continue
		}
		mark := ""
		switch {
		case t.ID == v.current && v.playing:
			mark = "▶"
		case t.ID == v.current:
			mark = "‖"
		case queued(v, t.ID):
			mark = "+"
		}
		rows = append(rows, map[string]any{
			"id": t.ID, "mark": mark, "title": t.Title, "artist": t.Artist,
			"album": t.Album, "time": clock(t.Secs), "playing": t.ID == v.current,
		})
	}
	return rows
}

func queueRows(v view) []any {
	rows := []any{}
	for i, q := range v.queue {
		t := catalog[trackIndex(q.ID)]
		rows = append(rows, map[string]any{
			"qid": q.QID, "n": fmt.Sprint(i + 1), "id": t.ID, "title": t.Title,
			"artist": t.Artist, "time": clock(t.Secs),
		})
	}
	return rows
}

// albumList groups the catalog by album, in catalog order.
func albumList() []album {
	var out []album
	for _, t := range catalog {
		n := len(out)
		if n == 0 || out[n-1].Name != t.Album {
			out = append(out, album{Name: t.Album, Artist: t.Artist})
			n++
		}
		out[n-1].IDs = append(out[n-1].IDs, t.ID)
		out[n-1].Secs += t.Secs
	}
	return out
}

type album struct {
	Name, Artist string
	IDs          []string
	Secs         int
}

func albumRows(v view) []any {
	cur := ""
	if i := trackIndex(v.current); i >= 0 {
		cur = catalog[i].Album
	}
	rows := []any{}
	for _, al := range albumList() {
		mark := ""
		if al.Name == cur {
			mark = "▶"
			if !v.playing {
				mark = "‖"
			}
		}
		rows = append(rows, map[string]any{
			"key": al.Name, "mark": mark, "album": al.Name, "artist": al.Artist,
			"tracks": fmt.Sprint(len(al.IDs)), "time": clock(al.Secs), "playing": al.Name == cur,
		})
	}
	return rows
}

func countsOf(v view) map[string]any {
	return map[string]any{
		"library": len(libRows(v)), "queue": len(v.queue), "albums": len(albumList()),
		"queue_empty": len(v.queue) == 0, "library_empty": len(libRows(v)) == 0,
	}
}

// upNext names what a track end would play next.
func upNext(v view) string {
	if v.repeat == repeatOne {
		return catalog[max(trackIndex(v.current), 0)].Title
	}
	if len(v.queue) > 0 {
		return catalog[trackIndex(v.queue[0].ID)].Title
	}
	if v.shuffle {
		return "random"
	}
	i := trackIndex(v.current)
	if i+1 < len(catalog) {
		return catalog[i+1].Title
	}
	if v.repeat == repeatAll {
		return catalog[0].Title
	}
	return "end of library"
}

func nowInfo(v view) map[string]any {
	i := trackIndex(v.current)
	if i < 0 {
		i = 0
	}
	t := catalog[i]
	state, icon := "stopped", "■"
	switch {
	case v.playing:
		state, icon = "playing", "▶"
	case v.posMs > 0:
		state, icon = "paused", "⏸"
	}
	pos := int(v.posMs / 1000)
	pct := math.Round(float64(v.posMs)/float64(t.Secs)) / 10
	shuffle := "⇄ shuffle off"
	if v.shuffle {
		shuffle = "⇄ shuffle on"
	}
	rep := v.repeat
	if rep == "" {
		rep = repeatOff
	}
	return map[string]any{
		"title": t.Title, "artist": t.Artist, "album": t.Album,
		"state": state, "icon": icon,
		"pos": clock(pos), "dur": clock(t.Secs), "time": clock(pos) + " / " + clock(t.Secs),
		"pct":           pct,
		"shuffle_label": shuffle,
		"repeat_label":  "↻ repeat " + rep,
		"repeat_on":     rep != repeatOff,
		"next":          upNext(v),
	}
}

func volInfo(v view) map[string]any {
	return map[string]any{"pct": v.volume, "label": fmt.Sprintf("%d%%", v.volume)}
}
