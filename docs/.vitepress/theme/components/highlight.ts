// A tiny highlighter for the landing page's code frames (.tui, .tcss, JSON,
// Go). The guide pages use VitePress's Shiki highlighting; this only has to
// color short, known snippets without shipping a grammar to the browser.

const escape = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
const span = (cls: string, s: string) => `<span class="tk-${cls}">${escape(s)}</span>`

/** {path} interpolations inside literal text. */
function interp(text: string, base = ''): string {
  let out = ''
  let last = 0
  for (const m of text.matchAll(/\{[A-Za-z_][\w.]*\}/g)) {
    const before = text.slice(last, m.index)
    out += base ? span(base, before) : escape(before)
    out += span('interp', m[0])
    last = m.index! + m[0].length
  }
  const rest = text.slice(last)
  return out + (base ? span(base, rest) : escape(rest))
}

export function highlightTui(src: string): string {
  let out = ''
  let i = 0
  while (i < src.length) {
    if (src.startsWith('<!--', i)) {
      const end = src.indexOf('-->', i)
      const stop = end < 0 ? src.length : end + 3
      out += span('com', src.slice(i, stop))
      i = stop
      continue
    }
    if (src[i] === '<') {
      const end = src.indexOf('>', i)
      const stop = end < 0 ? src.length : end + 1
      const tag = src.slice(i, stop)
      const m = /^<(\/?)([\w-]+)/.exec(tag)
      if (!m) {
        out += escape(tag)
        i = stop
        continue
      }
      out += escape('<' + m[1]) + span('tag', m[2])
      let rest = tag.slice(m[0].length)
      rest = rest.replace(/\s+|([\w:-]+)(=)("[^"]*")|\/?>|[^\s]/g, (tok, name, eq, str) => {
        if (name) return span('attr', name) + escape(eq) + interp(str, 'str')
        return escape(tok)
      })
      out += rest
      i = stop
      continue
    }
    const next = src.indexOf('<', i)
    const stop = next < 0 ? src.length : next
    out += interp(src.slice(i, stop))
    i = stop
  }
  return out
}

export function highlightTcss(src: string): string {
  return src
    .split('\n')
    .map((line) => {
      if (/^\s*\/\*/.test(line)) return span('com', line)
      const at = /^(\s*)(@media)(.*)$/.exec(line)
      if (at) {
        const open = /\{\s*$/.test(at[3])
        return escape(at[1]) + span('kw', at[2]) + span('val', at[3].replace(/\s*\{\s*$/, '')) + (open ? ' {' : '')
      }
      // selector { prop: value; prop: value; }
      const rule = /^(\s*)([^{}]*?)(\s*\{)(.*?)(\}?)\s*$/.exec(line)
      if (rule && rule[2]) {
        return escape(rule[1]) + span('sel', rule[2]) + escape(rule[3]) + decls(rule[4]) + escape(rule[5])
      }
      if (/^\s*[\w-]+\s*:/.test(line)) return decls(line)
      return escape(line)
    })
    .join('\n')
}

function decls(s: string): string {
  return s.replace(/([\w-]+)(\s*:\s*)([^;]*)(;?)/g, (_m, p, colon, v, semi) => {
    const val = escape(v).replace(/(\$[\w-]+|var\(--[\w-]+\))/g, '<span class="tk-interp">$1</span>')
    return span('prop', p) + escape(colon) + `<span class="tk-val">${val}</span>` + escape(semi)
  })
}

export function highlightJson(src: string): string {
  return src.replace(
    /("(?:[^"\\]|\\.)*")(\s*:)?|\b(true|false|null)\b|(-?\d+(?:\.\d+)?)|([^"\w-]+|[\w-]+)/g,
    (m, str, colon, lit, num, other) => {
      if (str) return colon ? span('attr', str) + escape(colon) : span('str', str)
      if (lit) return span('kw', lit)
      if (num) return span('num', num)
      return escape(other ?? m)
    },
  )
}

export function highlightGo(src: string): string {
  return src.replace(
    /(\/\/[^\n]*)|("(?:[^"\\]|\\.)*")|\b(func|return|if|nil|package|import|var|type|struct|any|error)\b|\b(tuimark)\b(\.)(\w+)|(\w+|[^\w"/]+|\/)/g,
    (m, com, str, kw, pkg, dot, ident, other) => {
      if (com) return span('com', com)
      if (str) return span('str', str)
      if (kw) return span('kw', kw)
      if (pkg) return span('tag', pkg) + escape(dot) + span('attr', ident)
      return escape(other ?? m)
    },
  )
}

