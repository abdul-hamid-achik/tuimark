// Smoke test for the real tuimark.wasm: loads it with Go's wasm_exec.js under
// Node and drives __tuimark (open, render, step, validate, reload, close).
//
//   node cmd/tuimark-wasm/testdata/smoke.mjs bin/tuimark.wasm [--wasm-exec PATH] [--cli bin/tuimark]
//
// --wasm-exec defaults to the lib/wasm/wasm_exec.js of `go env GOROOT` (the
// file must come from the Go that built the wasm; `task wasm` copies it next
// to the wasm, and then it is found there first). With --cli, every document under
// examples/ and kit/ (not a <fragment>) is also rendered at 80x24 and 120x40 and its
// dump is compared, member for member, with `tuimark dump --format json
// --styles --cells` (the native-vs-wasm parity of ADR 0033).
//
// Exits 0 on success, 1 on the first failed check.

import { execFileSync } from 'node:child_process'
import { existsSync, readFileSync, readdirSync, statSync } from 'node:fs'
import { createRequire } from 'node:module'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import assert from 'node:assert/strict'

const here = dirname(fileURLToPath(import.meta.url))
const root = resolve(here, '../../..')

const args = process.argv.slice(2)
const flag = (name) => {
  const i = args.indexOf(name)
  return i < 0 ? undefined : args.splice(i, 2)[1]
}
const wasmExecArg = flag('--wasm-exec')
const cliArg = flag('--cli')
const wasmPath = args[0]
if (!wasmPath) {
  console.error('usage: smoke.mjs TUIMARK.wasm [--wasm-exec PATH] [--cli TUIMARK]')
  process.exit(1)
}

function findWasmExec() {
  if (wasmExecArg) return resolve(wasmExecArg)
  const beside = join(dirname(resolve(wasmPath)), 'wasm_exec.js')
  if (existsSync(beside)) return beside
  const goroot = execFileSync('go', ['env', 'GOROOT'], { encoding: 'utf8' }).trim()
  for (const d of ['lib/wasm', 'misc/wasm']) if (existsSync(join(goroot, d, 'wasm_exec.js'))) return join(goroot, d, 'wasm_exec.js')
  throw new Error('wasm_exec.js not found')
}

// wasm_exec.js sets globalThis.Go; give it a bare environment.
createRequire(import.meta.url)(findWasmExec())
const go = new Go()
go.env = {}
const { instance } = await WebAssembly.instantiate(readFileSync(wasmPath), go.importObject)
go.run(instance) // never resolves: main blocks forever
for (let i = 0; !globalThis.__tuimark?.ready; i++) {
  if (i > 500) throw new Error('__tuimark never became ready')
  await new Promise((r) => setTimeout(r, 10))
}
const api = globalThis.__tuimark
const call = (m, ...a) => JSON.parse(api[m](...a))
const ok = (r) => (assert.equal(r.error, undefined, r.error), r)

let checks = 0
const check = (name, fn) => {
  try {
    fn()
    checks++
  } catch (e) {
    console.error(`FAIL ${name}: ${e.message}`)
    process.exit(1)
  }
}
const gridText = (r) => r.dump.grid.join('\n')

// --- the API on examples/tasks -------------------------------------------
const read = (p) => readFileSync(join(root, p), 'utf8')
const tasks = Object.fromEntries(['app.tui', 'theme.tcss', 'sample.json', 'mock.json'].map((f) => [f, read(`examples/tasks/${f}`)]))

assert.deepEqual(Object.keys(api).sort(), ['close', 'open', 'ready', 'reload', 'render', 'step', 'validate'])

const opened = ok(call('open', tasks, { cols: 80, rows: 24 }))
const id = opened.session
check('open: first frame', () => {
  assert.ok(Number.isInteger(id) && id > 0)
  assert.equal(opened.dump.cols, 80)
  assert.equal(opened.dump.rows, 24)
  assert.equal(opened.dump.ok, true)
  assert.ok(opened.dump.styles && opened.dump.cells, 'styles and cells')
  assert.match(gridText(opened), /Todo/)
  assert.deepEqual(opened.events.filter((e) => e.step !== 0), [])
})

check('render: size and theme', () => {
  const r = ok(call('render', id, { cols: 120, rows: 40, theme: 'light' }))
  assert.equal(r.dump.cols, 120)
  assert.equal(r.dump.rows, 40)
  assert.equal(r.dump.theme, 'light')
  assert.ok(call('render', id, { theme: 'neon' }).error)
})

check('step: keys open the mocked modal', () => {
  ok(call('render', id, { cols: 80, rows: 24 }))
  const r = ok(call('step', id, ['down', 'down', 'n']))
  assert.ok(r.events.some((e) => e.action === 'card_new'), JSON.stringify(r.events))
  assert.match(gridText(r), /New card in Todo/)
})

check('step: typing and submitting fires card_save with the value', () => {
  const r = ok(call('step', id, ['text:WASM ok', 'enter']))
  const save = r.events.find((e) => e.action === 'card_save')
  assert.ok(save, JSON.stringify(r.events))
  assert.equal(save.value, 'WASM ok')
})

check('step: one string, and the cap on paste:', () => {
  ok(call('step', id, 'esc'))
  assert.ok(call('step', id, ['paste:' + 'x'.repeat(16 * 1024 + 1)]).error)
  assert.ok(call('step', id, ['wat:1']).error)
})

check('validate: validate --json', () => {
  const v = ok(call('validate', id))
  assert.equal(v.ok, true)
  assert.equal(v.file, 'app.tui')
  assert.equal(v.data, 'sample.json')
  assert.ok(Array.isArray(v.diagnostics) && Array.isArray(v.actions))
})

check('reload: edits show, the store stays', () => {
  const before = ok(call('render', id))
  assert.match(gridText(before), /▤ tasks/)
  const edited = { ...tasks, 'app.tui': tasks['app.tui'].replace('▤ tasks', '▤ reloaded') }
  const r = ok(call('reload', id, edited))
  assert.equal(r.dump.ok, true)
  assert.match(gridText(r), /▤ reloaded/)
  assert.equal(r.dump.focus, before.dump.focus)
  assert.ok(call('reload', id, { 'app.tui': '<tui', 'sample.json': '{' }).error, 'bad data fails the reload')
  assert.match(gridText(ok(call('render', id))), /▤ reloaded/)
})

check('a broken document opens with diagnostics', () => {
  const b = ok(call('open', { 'app.tui': '<tui><screen id="s"><text>hi</screen></tui>' }))
  assert.equal(b.dump.ok, false)
  assert.ok(b.dump.errors.some((d) => d.code === 'V005'))
  call('close', b.session)
})

check('close, and errors for a closed session', () => {
  assert.deepEqual(call('close', id), {})
  assert.equal(call('render', id).error, 'no such session')
  assert.ok(call('open', {}).error)
})

// --- parity with the native CLI ------------------------------------------
if (cliArg) {
  const cli = resolve(cliArg)
  const files = {}
  const walk = (dir) => {
    for (const name of readdirSync(join(root, dir))) {
      const rel = dir + '/' + name
      if (statSync(join(root, rel)).isDirectory()) {
        if (name !== 'node_modules') walk(rel)
      } else if (/\.(tui|tcss|json)$/.test(name) && name !== 'mock.json') files[rel] = read(rel)
    }
  }
  walk('examples')
  walk('kit')
  // A file whose root is <fragment> (kit/kit.tui) is for documents to include,
  // not a document to open: it is checked through the documents that include
  // it (examples/kit-gallery), as the CLI's validate checks it on its own.
  const isFragment = (src) =>
    /^<fragment[\s>/]/.test(src.replace(/<!--[\s\S]*?-->/g, '').replace(/<\?[\s\S]*?\?>/g, '').trimStart())
  assert.ok(isFragment(files['kit/kit.tui']), 'kit/kit.tui is a fragment')
  const entries = Object.keys(files).filter((p) => p.endsWith('.tui') && !isFragment(files[p])).sort()
  assert.ok(entries.length >= 40, `only ${entries.length} presets`)
  const sampleOf = (e) => {
    for (const s of [e.replace(/\.tui$/, '.sample.json'), e.slice(0, e.lastIndexOf('/')) + '/sample.json']) if (files[s]) return s
    return ''
  }
  const native = (e, data, cols, rows) => {
    const a = ['dump', e, '--format', 'json', '--styles', '--cells', '--cols', String(cols), '--rows', String(rows)]
    if (data) a.push('--data', data)
    let out
    try {
      out = execFileSync(cli, a, { cwd: root, encoding: 'utf8', maxBuffer: 1 << 28 })
    } catch (err) {
      if (err.status !== 2) throw err // 2 is "the document has errors"; the dump is still printed
      out = err.stdout
    }
    return JSON.parse(out)
  }
  for (const e of entries) {
    const data = sampleOf(e)
    const o = ok(call('open', files, { entry: e }))
    for (const [cols, rows] of [[80, 24], [120, 40]]) {
      const w = cols === 80 ? o : ok(call('render', o.session, { cols, rows }))
      check(`parity ${e} ${cols}x${rows}`, () => assert.deepStrictEqual(w.dump, native(e, data, cols, rows)))
    }
    call('close', o.session)
  }
}

console.log(`ok: ${checks} checks${cliArg ? ' (including native parity)' : ''}`)
process.exit(0)
