// Paints a Tuimark JSON dump (`--styles --cells`) as a deterministic SVG.
//
// The dump already holds everything a terminal would show: one cluster per
// cell (`cells`) and the colors and attributes of each run of cells
// (`styles`). Text is drawn cell-aligned; box-drawing lines and block
// elements are drawn as vector shapes, so borders join and bars fill their
// cells exactly at any zoom, whatever font the viewer has.

const CW = 9.6 // cell width
const RH = 20 // row height
const FS = 16 // font size
const PAD = 14 // padding around the grid
const BASE = 15 // text baseline inside a row

// Canonical ANSI names → colors, one palette per effective theme. The
// defaults are Tuimark's built-in theme foreground and background.
const PALETTES = {
  dark: {
    default: { fg: '#e6edf3', bg: '#0d1117' },
    black: '#484f58', red: '#ff7b72', green: '#3fb950', yellow: '#d29922',
    blue: '#58a6ff', magenta: '#bc8cff', cyan: '#39c5cf', white: '#b1bac4',
    'bright-black': '#6e7681', 'bright-red': '#ffa198', 'bright-green': '#56d364', 'bright-yellow': '#e3b341',
    'bright-blue': '#79c0ff', 'bright-magenta': '#d2a8ff', 'bright-cyan': '#56d4dd', 'bright-white': '#f0f6fc',
  },
  light: {
    default: { fg: '#1f2328', bg: '#ffffff' },
    black: '#24292f', red: '#cf222e', green: '#116329', yellow: '#4d2d00',
    blue: '#0969da', magenta: '#8250df', cyan: '#1b7c83', white: '#6e7781',
    'bright-black': '#57606a', 'bright-red': '#a40e26', 'bright-green': '#1a7f37', 'bright-yellow': '#633c01',
    'bright-blue': '#218bff', 'bright-magenta': '#a475f9', 'bright-cyan': '#3192aa', 'bright-white': '#8c959f',
  },
}

// Box drawing: arms (u, d, l, r) and a weight.
const LINES = {}
const defLines = (weight, table) => {
  for (const [ch, arms] of Object.entries(table)) LINES[ch] = { weight, arms }
}
defLines('light', { '─': 'lr', '│': 'ud', '┌': 'rd', '┐': 'ld', '└': 'ru', '┘': 'lu', '├': 'udr', '┤': 'udl', '┬': 'lrd', '┴': 'lru', '┼': 'udlr', '╴': 'l', '╵': 'u', '╶': 'r', '╷': 'd' })
defLines('heavy', { '━': 'lr', '┃': 'ud', '┏': 'rd', '┓': 'ld', '┗': 'ru', '┛': 'lu', '┣': 'udr', '┫': 'udl', '┳': 'lrd', '┻': 'lru', '╋': 'udlr' })
defLines('round', { '╭': 'rd', '╮': 'ld', '╯': 'lu', '╰': 'ru' })
defLines('double', { '═': 'lr', '║': 'ud', '╔': 'rd', '╗': 'ld', '╚': 'ru', '╝': 'lu' })

// Block elements as fractions of the cell: [x, y, w, h], or a shade.
const BLOCKS = {
  '█': [0, 0, 1, 1], '▀': [0, 0, 1, 0.5], '▐': [0.5, 0, 0.5, 1],
  '▁': [0, 7 / 8, 1, 1 / 8], '▂': [0, 6 / 8, 1, 2 / 8], '▃': [0, 5 / 8, 1, 3 / 8], '▄': [0, 4 / 8, 1, 4 / 8],
  '▅': [0, 3 / 8, 1, 5 / 8], '▆': [0, 2 / 8, 1, 6 / 8], '▇': [0, 1 / 8, 1, 7 / 8],
  '▏': [0, 0, 1 / 8, 1], '▎': [0, 0, 2 / 8, 1], '▍': [0, 0, 3 / 8, 1], '▌': [0, 0, 4 / 8, 1],
  '▋': [0, 0, 5 / 8, 1], '▊': [0, 0, 6 / 8, 1], '▉': [0, 0, 7 / 8, 1],
}
const SHADES = { '░': 0.22, '▒': 0.45, '▓': 0.7 }

const STROKE = { light: 1.3, round: 1.3, heavy: 2.6, double: 1.1 }
const DOUBLE_GAP = 2.4

const esc = (s) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
const n = (v) => Number(v.toFixed(2)).toString()

export function renderSVG(dump, { title = '' } = {}) {
  const { cols, rows } = dump
  if (!Array.isArray(dump.cells) || !Array.isArray(dump.styles)) {
    throw new Error('svg: the dump needs --cells and --styles')
  }
  const pal = PALETTES[dump.theme] ?? PALETTES.dark
  const color = (c, role) => (c === 'default' ? pal.default[role] : c.startsWith('#') ? c : (pal[c] ?? pal.default[role]))

  // Resolve every cell's cluster and look.
  const grid = Array.from({ length: rows }, () => new Array(cols))
  for (const c of dump.cells) grid[c.y][c.x] = { ch: c.ch, w: c.w ?? 1 }
  for (let y = 0; y < rows; y++) {
    for (const span of dump.styles[y]) {
      const a = new Set(span.a ?? [])
      let fg = color(span.fg, 'fg')
      let bg = color(span.bg, 'bg')
      if (a.has('reverse')) [fg, bg] = [bg, fg]
      const look = { fg, bg, bold: a.has('bold'), dim: a.has('dim'), italic: a.has('italic'), underline: a.has('underline') }
      for (let x = span.x; x < span.x + span.w; x++) Object.assign(grid[y][x], look)
    }
  }

  // The canvas takes the most common background, so the padding blends in.
  const bgCount = new Map()
  for (const row of grid) for (const c of row) bgCount.set(c.bg, (bgCount.get(c.bg) ?? 0) + 1)
  const canvas = [...bgCount].sort((a, b) => b[1] - a[1])[0][0]

  const W = cols * CW + 2 * PAD
  const H = rows * RH + 2 * PAD
  const X = (x) => PAD + x * CW
  const Y = (y) => PAD + y * RH

  const rects = new Map() // fill|opacity → path data
  const strokes = new Map() // stroke|width|opacity → path data
  const texts = []
  const addRect = (fill, opacity, x, y, w, h) => {
    const k = `${fill}|${opacity}`
    rects.set(k, (rects.get(k) ?? '') + `M${n(x)} ${n(y)}h${n(w)}v${n(h)}h${n(-w)}z`)
  }
  const addStroke = (col, width, opacity, d) => {
    const k = `${col}|${width}|${opacity}`
    strokes.set(k, (strokes.get(k) ?? '') + d)
  }

  // Backgrounds, merged along each row.
  for (let y = 0; y < rows; y++) {
    let x = 0
    while (x < cols) {
      const bg = grid[y][x].bg
      let end = x + 1
      while (end < cols && grid[y][end].bg === bg) end++
      if (bg !== canvas) addRect(bg, 1, X(x), Y(y), (end - x) * CW, RH)
      x = end
    }
  }

  const drawLine = (ch, x0, y0, look) => {
    const { weight, arms } = LINES[ch]
    const op = look.dim ? 0.55 : 1
    const cx = x0 + CW / 2
    const cy = y0 + RH / 2
    const L = x0, R = x0 + CW, T = y0, B = y0 + RH
    const has = (a) => arms.includes(a)
    let d = ''
    if (weight === 'double') {
      const g = DOUBLE_GAP
      if (arms === 'lr') d = `M${n(L)} ${n(cy - g)}H${n(R)}M${n(L)} ${n(cy + g)}H${n(R)}`
      else if (arms === 'ud') d = `M${n(cx - g)} ${n(T)}V${n(B)}M${n(cx + g)} ${n(T)}V${n(B)}`
      else {
        // A corner: an outer and an inner L, meeting the neighbours' pairs.
        const sx = has('r') ? 1 : -1 // horizontal arm direction
        const sy = has('d') ? 1 : -1 // vertical arm direction
        const hx = sx > 0 ? R : L
        const vy = sy > 0 ? B : T
        d = `M${n(hx)} ${n(cy - sy * g)}H${n(cx - sx * g)}V${n(vy)}` + `M${n(hx)} ${n(cy + sy * g)}H${n(cx + sx * g)}V${n(vy)}`
      }
      addStroke(look.fg, STROKE.double, op, d)
      return
    }
    const corner = arms.length === 2 && (has('l') || has('r')) && (has('u') || has('d'))
    if (corner) {
      const hx = has('r') ? R : L
      const vy = has('d') ? B : T
      if (weight === 'round') {
        const r = Math.min(CW, RH) / 2
        const sx = has('r') ? 1 : -1
        const sy = has('d') ? 1 : -1
        d = `M${n(hx)} ${n(cy)}H${n(cx + sx * r)}Q${n(cx)} ${n(cy)} ${n(cx)} ${n(cy + sy * r)}V${n(vy)}`
      } else {
        d = `M${n(hx)} ${n(cy)}H${n(cx)}V${n(vy)}`
      }
    } else {
      if (has('l') && has('r')) d += `M${n(L)} ${n(cy)}H${n(R)}`
      else if (has('l')) d += `M${n(L)} ${n(cy)}H${n(cx)}`
      else if (has('r')) d += `M${n(cx)} ${n(cy)}H${n(R)}`
      if (has('u') && has('d')) d += `M${n(cx)} ${n(T)}V${n(B)}`
      else if (has('u')) d += `M${n(cx)} ${n(T)}V${n(cy)}`
      else if (has('d')) d += `M${n(cx)} ${n(cy)}V${n(B)}`
    }
    addStroke(look.fg, STROKE[weight], op, d)
  }

  // Text runs, box drawing, and blocks.
  for (let y = 0; y < rows; y++) {
    let run = null
    const flush = () => {
      if (!run) return
      texts.push(run)
      run = null
    }
    for (let x = 0; x < cols; x++) {
      const c = grid[y][x]
      if (c.ch === '') continue // continuation of a wide cluster
      if (c.ch === ' ') {
        flush()
        continue
      }
      if (LINES[c.ch]) {
        flush()
        drawLine(c.ch, X(x), Y(y), c)
        continue
      }
      if (BLOCKS[c.ch]) {
        flush()
        const [bx, by, bw, bh] = BLOCKS[c.ch]
        addRect(c.fg, c.dim ? 0.55 : 1, X(x) + bx * CW, Y(y) + by * RH, bw * CW, bh * RH)
        continue
      }
      if (SHADES[c.ch]) {
        flush()
        addRect(c.fg, SHADES[c.ch] * (c.dim ? 0.55 : 1), X(x), Y(y), CW, RH)
        continue
      }
      const same = run && run.x + run.w === x && c.w === 1 && run.wide === false &&
        run.fg === c.fg && run.bold === c.bold && run.dim === c.dim && run.italic === c.italic && run.underline === c.underline
      if (same) {
        run.text += c.ch
        run.w += 1
        continue
      }
      flush()
      run = { x, y, w: c.w, text: c.ch, wide: c.w !== 1, fg: c.fg, bold: c.bold, dim: c.dim, italic: c.italic, underline: c.underline }
      if (run.wide) flush()
    }
    flush()
  }

  const out = []
  out.push(
    `<svg xmlns="http://www.w3.org/2000/svg" width="${n(W)}" height="${n(H)}" viewBox="0 0 ${n(W)} ${n(H)}" role="img" aria-label="${esc(title)}">`,
  )
  if (title) out.push(`<title>${esc(title)}</title>`)
  out.push(`<rect width="${n(W)}" height="${n(H)}" rx="6" fill="${canvas}"/>`)
  for (const [k, d] of rects) {
    const [fill, op] = k.split('|')
    out.push(`<path fill="${fill}"${op === '1' ? '' : ` fill-opacity="${op}"`} d="${d}"/>`)
  }
  out.push(`<g fill="none" stroke-linecap="square" stroke-linejoin="miter">`)
  for (const [k, d] of strokes) {
    const [stroke, width, op] = k.split('|')
    out.push(`<path stroke="${stroke}" stroke-width="${width}"${op === '1' ? '' : ` stroke-opacity="${op}"`} d="${d}"/>`)
  }
  out.push('</g>')
  out.push(
    `<g font-family="'JetBrains Mono','SF Mono',SFMono-Regular,Menlo,Consolas,'DejaVu Sans Mono','Noto Sans Mono',monospace" font-size="${FS}" xml:space="preserve">`,
  )
  for (const t of texts) {
    // A wide cluster (CJK, emoji) is centered on its two cells at its own
    // width; a run of narrow cells is spread over exactly its cells.
    const attrs = t.wide
      ? [`x="${n(X(t.x) + (t.w * CW) / 2)}"`, `y="${n(Y(t.y) + BASE)}"`, 'text-anchor="middle"', `fill="${t.fg}"`]
      : [`x="${n(X(t.x))}"`, `y="${n(Y(t.y) + BASE)}"`, `textLength="${n(t.w * CW)}"`, 'lengthAdjust="spacing"', `fill="${t.fg}"`]
    if (t.bold) attrs.push('font-weight="700"')
    if (t.italic) attrs.push('font-style="italic"')
    if (t.dim) attrs.push('fill-opacity="0.55"')
    out.push(`<text ${attrs.join(' ')}>${esc(t.text)}</text>`)
    if (t.underline) out.push(`<rect x="${n(X(t.x))}" y="${n(Y(t.y) + RH - 3)}" width="${n(t.w * CW)}" height="1" fill="${t.fg}"/>`)
  }
  out.push('</g>')
  out.push('</svg>')
  return out.join('\n')
}
