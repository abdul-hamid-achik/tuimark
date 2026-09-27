#!/bin/sh
# POSIX sh helper for the preview_watch glyph spec.
#
# Copies a phase-0 fixture into a fresh temp dir, starts
# `tuimark preview --watch` on it in the background, edits the file after a
# moment, waits for the watcher to notice, then stops it.
#
# Usage: preview_watch.sh /path/to/tuimark
set -eu

tuimark_bin=$1
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

cat > "$tmp/app.tui" <<'EOF'
<app width="100%" height="100%">
  <col id="root" width="1fr" height="1fr">
    <row id="body" width="1fr" height="1fr">
      <box id="inbox" width="30%" height="1fr" border="1">
        <text>Inbox</text>
      </box>
      <box id="detail" width="1fr" height="1fr" border="1">
        <text>Detail</text>
      </box>
    </row>
    <box id="status" width="1fr" height="1">
      <text>ready</text>
    </box>
  </col>
</app>
EOF

"$tuimark_bin" preview "$tmp/app.tui" --watch &
pid=$!

sleep 1

cat > "$tmp/app.tui" <<'EOF'
<app width="100%" height="100%">
  <col id="root" width="1fr" height="1fr">
    <row id="body" width="1fr" height="1fr">
      <box id="inbox" width="30%" height="1fr" border="1">
        <text>Inbox</text>
      </box>
      <box id="detail" width="1fr" height="1fr" border="1">
        <text>Detail</text>
      </box>
    </row>
    <box id="status" width="1fr" height="1">
      <text>updated</text>
    </box>
  </col>
</app>
EOF

sleep 1
kill "$pid" 2>/dev/null || true
wait "$pid" 2>/dev/null || true
