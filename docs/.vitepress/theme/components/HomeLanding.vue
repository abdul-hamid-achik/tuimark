<script setup lang="ts">
import { computed, ref } from 'vue'
import { withBase } from 'vitepress'
import CommandCopy from './CommandCopy.vue'
import InstallPanel from './InstallPanel.vue'
import TermShot from './TermShot.vue'
import { highlightGo, highlightJson, highlightTcss, highlightTui } from './highlight'
// The landing page shows the same files and output as the guide: the
// snippets under docs/snippets, rendered by docs/scripts/generate.mjs.
import tourTui from '../../../snippets/tour/app.tui?raw'
import tourTcss from '../../../snippets/tour/theme.tcss?raw'
import tourJson from '../../../snippets/tour/sample.json?raw'
import tourGrid from '../../../snippets/out/tour-80.txt?raw'
import tourDump from '../../../snippets/out/tour-json.jsonc?raw'
import tourPlay from '../../../snippets/out/tour-play.txt?raw'
// Counts and tag lists come from `tuimark agents`, via the generator.
import catalog from '../catalog.json'

const installCommand = 'go install github.com/abdul-hamid-achik/tuimark/cmd/tuimark@latest'

const files = [
  { name: 'app.tui', note: 'structure', html: highlightTui(tourTui.trimEnd()) },
  { name: 'theme.tcss', note: 'style', html: highlightTcss(tourTcss.trimEnd()) },
  { name: 'sample.json', note: 'data', html: highlightJson(tourJson.trimEnd()) },
]
const activeFile = ref(0)

type Output = 'frame' | 'narrow' | 'dump'
const output = ref<Output>('frame')

// An excerpt of the tour's JSON dump, one node per line: the named nodes
// and the selected row, the geometry an agent reads back.
const dumpExcerpt = computed(() => {
  const d = JSON.parse(tourDump.replace(/^(\/\/.*\n)+/, ''))
  const node = (n: Record<string, unknown>) => JSON.stringify(n).replace(/,"/g, ', "').replace(/":/g, '": ')
  const shown = d.nodes.filter((n: Record<string, unknown>) => n.id || n.selected)
  return highlightJson(
    [
      '{',
      `  "cols": ${d.cols}, "rows": ${d.rows}, "ok": ${d.ok}, "focus": ${JSON.stringify(d.focus)},`,
      `  "errors": ${JSON.stringify(d.errors)},`,
      '  "nodes": [',
      ...shown.map((n: Record<string, unknown>) => `    ${node(n)},`),
      `    … ${d.nodes.length - shown.length} more`,
      '  ],',
      `  "grid": [ … ${d.grid.length} rows … ]`,
      '}',
    ].join('\n'),
  )
})

const playEvents = tourPlay.split('=== events ===\n')[1]?.trimEnd() ?? ''

const hostCode = highlightGo(`ui, err := tuimark.Load("app.tui")
if err != nil {
	return err
}
_ = ui.Bind("", data) // the JSON store
ui.On("preview", func(ev tuimark.Event) error {
	return ui.Set("current", lookup(ev.Keys["t"]))
})
ui.On("open", openTicket)
return ui.Run(os.Stdout)`)

const api = [
  ['Load', 'read a .tui file'],
  ['Parse', 'read one from an io.Reader'],
  ['Bind', 'put data in the store'],
  ['Set', 'change it while running'],
  ['On', 'handle a named action'],
  ['Catalog', 'list the actions used'],
  ['Dump', 'render a frame to JSON'],
  ['Validate', 'list the diagnostics'],
  ['Run', 'drive the terminal'],
]

// The closed vocabulary, grouped by role; version="2" tags are marked.
const roles: Record<string, string[]> = {
  document: ['tui', 'style', 'keymap', 'bind', 'screen'],
  layout: ['col', 'row', 'box', 'scroll', 'spacer', 'tabs', 'tab'],
  content: ['text', 'rule', 'sparkline', 'hints'],
  interactive: ['list', 'item', 'input', 'button', 'progress', 'table', 'column'],
  overlay: ['modal'],
}
const allTags = [...catalog.tags, ...catalog.tagsV2]
const grouped = new Set(Object.values(roles).flat())
const vocabulary = [
  ...Object.entries(roles).map(([group, tags]) => ({ group, tags: tags.filter((t) => allTags.includes(t)) })),
  { group: 'more', tags: allTags.filter((t) => !grouped.has(t)) },
]
  .filter((g) => g.tags.length > 0)
  .map((g) => ({ group: g.group, tags: g.tags.map((t) => ({ name: t, v2: catalog.tagsV2.includes(t) })) }))
const tagCount = allTags.length
const v2Count = catalog.tagsV2.length

const loop = [
  { n: '1', verb: 'edit', cmd: 'app.tui · theme.tcss · sample.json', note: 'Markup, a reduced CSS, and JSON. No Go in the view.' },
  { n: '2', verb: 'validate', cmd: 'tuimark validate app.tui --data sample.json', note: 'Every problem is a code: V parse, L layout, B bind.' },
  { n: '3', verb: 'dump', cmd: 'tuimark dump app.tui --data sample.json --cols 80 --rows 10 --format json', note: 'The frame as data: the grid, every node’s rect, the errors.' },
  { n: '4', verb: 'play', cmd: 'tuimark play app.tui --data sample.json --cols 80 --rows 10 --input "down enter"', note: 'Replay keys, typing and clicks with no terminal; read which actions fired.' },
]
</script>

<template>
  <div class="home">
    <!-- ── Hero ─────────────────────────────────────────────────────────── -->
    <section class="shell hero">
      <div class="hero-copy">
        <p class="eyebrow"><span class="dot">▌</span> v0.3.1 · Go runtime · MIT</p>
        <h1>Terminal UIs,<br />written as text.</h1>
        <p class="lede">
          Tuimark is a view language for terminals. Structure lives in <code>.tui</code> markup, style in
          <code>.tcss</code>, data in JSON, and behavior in named actions. A small Go runtime lays it out on a
          cell grid, paints it, and dumps every frame as JSON, so a coding agent can check what it built
          without ever looking at a screen.
        </p>
        <div class="actions">
          <a class="btn primary" :href="withBase('/guide/getting-started')">Get started →</a>
          <a class="btn" :href="withBase('/guide/agents')">For coding agents</a>
          <a class="btn" href="https://github.com/abdul-hamid-achik/tuimark">GitHub</a>
        </div>
        <CommandCopy class="install" :command="installCommand" />
      </div>

      <aside class="tf facts" aria-label="What Tuimark is">
        <span class="tf-title">is / is not</span>
        <ul>
          <li><b class="yes">✓</b> interpreted at run time: nothing generates Go</li>
          <li><b class="yes">✓</b> a closed catalog: {{ tagCount }} tags, {{ catalog.properties }} properties</li>
          <li><b class="yes">✓</b> integer cells, grapheme-aware widths</li>
          <li><b class="yes">✓</b> deterministic dumps you can diff and golden-test</li>
          <li><b class="no">✗</b> no pixels, no expressions, no logic in markup</li>
          <li><b class="no">✗</b> not a wrapper around another TUI framework</li>
        </ul>
        <a :href="withBase('/guide/language')">the language →</a>
      </aside>
    </section>

    <section class="shell">
      <TermShot name="monitor-processes" eager>
        <code>examples/monitor</code>: nine tabs, a sortable process table with multi-select, key hints
        generated from the keymap, a light and a dark palette. The view is two text files; the Go host only
        supplies data and handles named actions. This picture is rendered from <code>tuimark play</code>
        output, like every screenshot on this site.
      </TermShot>
    </section>

    <!-- ── Three files, one frame ───────────────────────────────────────── -->
    <section id="three-files" class="shell block">
      <header class="head">
        <h2>Three files, one frame.</h2>
        <p>
          A list bound to an array, a detail pane bound to the selection, key hints built from the keymap,
          and a media query that stacks the panes on a narrow terminal.
        </p>
      </header>

      <div class="split">
        <div class="pane code-pane">
          <div class="pane-tabs" role="tablist" aria-label="Files">
            <button
              v-for="(f, i) in files"
              :key="f.name"
              type="button"
              role="tab"
              :aria-selected="activeFile === i"
              :class="{ on: activeFile === i }"
              @click="activeFile = i"
            >
              {{ activeFile === i ? '▸' : ' ' }}{{ f.name }} <small>{{ f.note }}</small>
            </button>
          </div>
          <pre class="code"><code v-html="files[activeFile].html" /></pre>
        </div>

        <div class="pane out-pane">
          <div class="pane-tabs" role="tablist" aria-label="Output">
            <button type="button" role="tab" :aria-selected="output === 'frame'" :class="{ on: output === 'frame' }" @click="output = 'frame'">
              {{ output === 'frame' ? '▸' : ' ' }}80 cols
            </button>
            <button type="button" role="tab" :aria-selected="output === 'narrow'" :class="{ on: output === 'narrow' }" @click="output = 'narrow'">
              {{ output === 'narrow' ? '▸' : ' ' }}56 cols
            </button>
            <button type="button" role="tab" :aria-selected="output === 'dump'" :class="{ on: output === 'dump' }" @click="output = 'dump'">
              {{ output === 'dump' ? '▸' : ' ' }}text dump
            </button>
          </div>
          <div class="out-body">
            <TermShot v-if="output === 'frame'" name="tour" bare />
            <TermShot v-else-if="output === 'narrow'" name="tour-narrow" bare />
            <pre v-else class="grid-text">{{ tourGrid }}</pre>
          </div>
          <p class="out-cmd"><span class="prompt">$</span> tuimark dump app.tui --data sample.json --cols {{ output === 'narrow' ? 56 : 80 }}</p>
        </div>
      </div>
    </section>

    <!-- ── The loop ─────────────────────────────────────────────────────── -->
    <section id="loop" class="shell block">
      <header class="head">
        <h2>Built for the agent loop.</h2>
        <p>
          An agent cannot see a terminal. Tuimark gives it the frame as data instead, and a briefing,
          <code>tuimark agents</code>, generated from the runtime's own catalogs so it never drifts.
        </p>
      </header>

      <div class="loop">
        <ol class="steps">
          <li v-for="s in loop" :key="s.n" class="tf">
            <span class="tf-title"><b>{{ s.n }}</b> {{ s.verb }}</span>
            <code class="step-cmd">{{ s.cmd }}</code>
            <p>{{ s.note }}</p>
          </li>
        </ol>
        <div class="loop-out">
          <div class="tf">
            <span class="tf-title">dump --format json <small>(excerpt)</small></span>
            <pre class="code small"><code v-html="dumpExcerpt" /></pre>
          </div>
          <div class="tf">
            <span class="tf-title">play: events <small>step · action · source · keys · value</small></span>
            <pre class="code small">{{ playEvents }}</pre>
          </div>
        </div>
      </div>
      <p class="more">
        <a :href="withBase('/guide/agents')">Working with agents →</a>
        <a :href="withBase('/guide/tools')">Dumps and tools →</a>
      </p>
    </section>

    <!-- ── Vocabulary ───────────────────────────────────────────────────── -->
    <section id="vocabulary" class="shell block">
      <header class="head">
        <h2>A vocabulary you can hold in your head.</h2>
        <p>
          The catalog is closed. An unknown tag is <code>V001</code>, not a new widget; an unknown property is
          <code>V003</code>. <code>version="2"</code> documents add {{ v2Count }} tags (marked ²) and the rest of the
          0.2 vocabulary; <code>version="1"</code> documents keep their exact meaning.
        </p>
      </header>
      <div class="vocab">
        <div v-for="g in vocabulary" :key="g.group" class="tf">
          <span class="tf-title">{{ g.group }}</span>
          <ul>
            <li v-for="t in g.tags" :key="t.name" :class="{ v2: t.v2 }">&lt;{{ t.name }}&gt;<sup v-if="t.v2">2</sup></li>
          </ul>
        </div>
      </div>
      <p class="more">
        <a :href="withBase('/reference/tags')">Tags and attributes →</a>
        <a :href="withBase('/reference/css')">CSS properties →</a>
        <a :href="withBase('/guide/widgets')">Widgets →</a>
      </p>
    </section>

    <!-- ── Examples ─────────────────────────────────────────────────────── -->
    <section id="gallery" class="shell block">
      <header class="head">
        <h2>Real apps, same few files.</h2>
        <p>Every example in the repository is a <code>.tui</code>, a <code>.tcss</code>, and sample data, with an optional Go host.</p>
      </header>
      <div class="gallery">
        <a :href="withBase('/examples#monitor-studio')" class="shot-link">
          <TermShot name="monitor-overview" bare />
          <span>monitor studio · grid, gauges, sparklines</span>
        </a>
        <a :href="withBase('/examples#agent')" class="shot-link">
          <TermShot name="agent" bare />
          <span>coding-agent chat · transcript, tools, approvals</span>
        </a>
        <a :href="withBase('/examples#inbox')" class="shot-link">
          <TermShot name="inbox" bare />
          <span>inbox · search, list, detail, @media</span>
        </a>
      </div>
      <p class="more"><a :href="withBase('/examples')">All examples →</a></p>
    </section>

    <!-- ── Go host ──────────────────────────────────────────────────────── -->
    <section id="host" class="shell block">
      <header class="head">
        <h2>The host is a few lines of Go.</h2>
        <p>
          It loads the document, binds data, and answers named actions. Layout, focus, keys, the mouse,
          colors, and terminal handling stay in the runtime.
        </p>
      </header>
      <div class="split host">
        <div class="tf">
          <span class="tf-title">main.go</span>
          <pre class="code"><code v-html="hostCode" /></pre>
        </div>
        <div class="tf api">
          <span class="tf-title">the whole API</span>
          <ul>
            <li v-for="[fn, what] in api" :key="fn"><code>{{ fn }}</code><span>{{ what }}</span></li>
          </ul>
          <a :href="withBase('/guide/go-api')">Go API →</a>
        </div>
      </div>
    </section>

    <!-- ── Install ──────────────────────────────────────────────────────── -->
    <section id="install" class="shell block last">
      <header class="head">
        <h2>Install.</h2>
        <p>Then run <code>tuimark agents</code> and hand the output to your agent.</p>
      </header>
      <InstallPanel heading-level="h3" />
    </section>
  </div>
</template>

<style scoped>
.home {
  padding: 0 0 96px;
  background-image: linear-gradient(var(--t-cell-line) 1px, transparent 1px),
    linear-gradient(90deg, var(--t-cell-line) 1px, transparent 1px);
  background-position: -1px -1px;
  background-size: 12px 24px;
  background-repeat: repeat;
  -webkit-mask-image: none;
}

.shell {
  max-width: 1180px;
  margin: 0 auto;
  padding: 0 24px;
}

/* ── hero ── */
.hero {
  display: grid;
  grid-template-columns: minmax(0, 1.35fr) minmax(0, 0.9fr);
  gap: 48px;
  align-items: end;
  padding-top: clamp(48px, 8vw, 96px);
  padding-bottom: 40px;
}

.eyebrow {
  margin: 0 0 18px;
  color: var(--vp-c-text-3);
  font-family: var(--vp-font-family-mono);
  font-size: 13px;
}

.eyebrow .dot {
  color: var(--vp-c-brand-1);
}

h1 {
  margin: 0;
  color: var(--vp-c-text-1);
  font-family: var(--vp-font-family-mono);
  font-size: clamp(38px, 6vw, 66px);
  font-weight: 650;
  letter-spacing: -0.055em;
  line-height: 1.02;
}

.lede {
  max-width: 62ch;
  margin: 24px 0 0;
  color: var(--vp-c-text-2);
  font-size: 17.5px;
  line-height: 1.65;
}

.lede code,
.head code {
  color: var(--vp-c-text-1);
  font-family: var(--vp-font-family-mono);
  font-size: 0.9em;
}

.actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin-top: 28px;
}

.btn {
  display: inline-flex;
  align-items: center;
  border: 1px solid var(--t-border);
  border-radius: 6px;
  padding: 9px 16px;
  color: var(--vp-c-text-1);
  font-family: var(--vp-font-family-mono);
  font-size: 14px;
  font-weight: 600;
  text-decoration: none;
  transition: border-color 140ms ease, background-color 140ms ease;
}

.btn:hover {
  border-color: var(--vp-c-brand-1);
}

.btn.primary {
  border-color: var(--vp-c-brand-1);
  background: var(--vp-c-brand-1);
  color: var(--t-select-fg);
}

.btn.primary:hover {
  background: var(--vp-c-brand-2);
}

.install {
  max-width: 620px;
  margin-top: 22px;
}

.facts ul {
  margin: 0;
  padding: 0;
  list-style: none;
}

.facts li {
  padding: 5px 0;
  color: var(--vp-c-text-2);
  font-size: 14.5px;
  line-height: 1.5;
}

.facts b {
  display: inline-block;
  width: 1.4em;
  font-family: var(--vp-font-family-mono);
}

.yes {
  color: var(--t-ok);
}

.no {
  color: var(--t-danger);
}

.facts a,
.api a,
.more a {
  color: var(--vp-c-brand-1);
  font-family: var(--vp-font-family-mono);
  font-size: 13.5px;
  font-weight: 600;
  text-decoration: none;
}

.facts a:hover,
.api a:hover,
.more a:hover {
  text-decoration: underline;
  text-underline-offset: 4px;
}

.facts a {
  display: inline-block;
  margin-top: 10px;
}

/* ── sections ── */
.block {
  padding-top: clamp(64px, 9vw, 104px);
  scroll-margin-top: 0;
}

.head {
  max-width: 760px;
  margin-bottom: 30px;
}

.head h2 {
  margin: 0;
  border: 0;
  padding: 0;
  color: var(--vp-c-text-1);
  font-family: var(--vp-font-family-mono);
  font-size: clamp(24px, 3.2vw, 34px);
  font-weight: 650;
  letter-spacing: -0.045em;
  line-height: 1.15;
}

.head p {
  margin: 14px 0 0;
  color: var(--vp-c-text-2);
  font-size: 16.5px;
  line-height: 1.65;
}

.more {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 24px;
  margin: 22px 0 0;
}

.split {
  display: grid;
  grid-template-columns: minmax(0, 1.05fr) minmax(0, 1fr);
  gap: 20px;
  align-items: start;
}

.split > * {
  min-width: 0;
}

.pane {
  overflow: hidden;
  border: 1px solid var(--t-border);
  border-radius: 9px;
  background: var(--vp-c-bg);
}

.pane-tabs {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  border-bottom: 1px solid var(--vp-c-divider);
  padding: 9px 10px;
  background: var(--t-panel);
}

.pane-tabs button {
  border: 0;
  border-radius: 3px;
  padding: 2px 9px;
  background: transparent;
  color: var(--vp-c-text-2);
  cursor: pointer;
  font-family: var(--vp-font-family-mono);
  font-size: 13px;
  white-space: pre;
}

.pane-tabs button:hover {
  color: var(--vp-c-text-1);
}

.pane-tabs button.on {
  background: var(--vp-c-brand-1);
  color: var(--t-select-fg);
  font-weight: 700;
}

.pane-tabs small {
  opacity: 0.7;
  font-size: 11px;
  font-weight: 400;
}

.code {
  margin: 0;
  overflow-x: auto;
  padding: 16px 18px;
  color: var(--vp-c-text-1);
  font-family: var(--vp-font-family-mono);
  font-size: 12.5px;
  line-height: 1.62;
  tab-size: 4;
}

.code.small {
  padding: 4px 0 0;
  font-size: 12px;
  line-height: 1.55;
}

.code-pane .code {
  min-height: 420px;
}

.out-body {
  padding: 14px;
}

.out-body :deep(.term-shot) {
  margin: 0;
}

.grid-text {
  margin: 0;
  overflow-x: auto;
  color: var(--vp-c-text-1);
  font-family: var(--vp-font-family-mono);
  font-size: 11.5px;
  line-height: 1.32;
  font-variant-ligatures: none;
}

.out-cmd {
  margin: 0;
  border-top: 1px solid var(--vp-c-divider);
  padding: 10px 14px;
  overflow-x: auto;
  color: var(--vp-c-text-2);
  font-family: var(--vp-font-family-mono);
  font-size: 12px;
  white-space: nowrap;
}

.prompt {
  color: var(--vp-c-brand-1);
  font-weight: 700;
}

/* ── loop ── */
.loop {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  gap: 22px;
  align-items: start;
}

.steps {
  display: grid;
  gap: 22px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.step-cmd {
  display: block;
  color: var(--vp-c-text-1);
  font-family: var(--vp-font-family-mono);
  font-size: 12.5px;
  line-height: 1.55;
  overflow-wrap: anywhere;
}

.steps p {
  margin: 6px 0 0;
  color: var(--vp-c-text-3);
  font-size: 14px;
  line-height: 1.5;
}

.loop-out {
  display: grid;
  gap: 22px;
  min-width: 0;
}

.loop-out .tf,
.steps .tf {
  min-width: 0;
}

.tf-title small {
  margin-left: 6px;
  color: var(--vp-c-text-3);
  font-weight: 400;
}

/* ── vocabulary ── */
.vocab {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(190px, 1fr));
  gap: 22px 16px;
}

.vocab ul {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.vocab li {
  border: 1px solid var(--vp-c-divider);
  border-radius: 4px;
  padding: 1px 7px;
  color: var(--vp-c-text-1);
  font-family: var(--vp-font-family-mono);
  font-size: 12.5px;
}

.vocab li.v2 {
  border-color: color-mix(in srgb, var(--vp-c-brand-1) 45%, var(--vp-c-divider));
  color: var(--vp-c-brand-1);
}

.vocab sup {
  margin-left: 1px;
  font-size: 9px;
}

/* ── gallery ── */
.gallery {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 18px;
}

.shot-link {
  display: flex;
  flex-direction: column;
  gap: 10px;
  color: var(--vp-c-text-2);
  font-family: var(--vp-font-family-mono);
  font-size: 12.5px;
  text-decoration: none;
}

.shot-link :deep(.term-shot) {
  margin: 0;
}

/* Thumbnails share one frame shape, cropped from the top left. */
.shot-link :deep(.term-body) {
  aspect-ratio: 16 / 9;
}

.shot-link :deep(.shot) {
  height: 100%;
  object-fit: cover;
  object-position: left top;
}

.shot-link:hover :deep(.term-body) {
  border-color: var(--vp-c-brand-1);
}

.shot-link:hover span {
  color: var(--vp-c-brand-1);
}

/* ── host ── */
.host .tf {
  min-width: 0;
}

.host .code {
  padding: 4px 0 0;
}

.api ul {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(190px, 1fr));
  gap: 8px 18px;
  margin: 0 0 14px;
  padding: 0;
  list-style: none;
}

.api li {
  display: flex;
  flex-direction: column;
}

.api code {
  color: var(--vp-c-brand-1);
  font-family: var(--vp-font-family-mono);
  font-size: 13.5px;
  font-weight: 600;
}

.api span {
  color: var(--vp-c-text-3);
  font-size: 13.5px;
}

/* ── highlighter tokens ── */
.code :deep(.tk-tag),
.code :deep(.tk-sel) {
  color: var(--vp-c-brand-1);
}

.code :deep(.tk-attr),
.code :deep(.tk-prop) {
  color: var(--vp-c-text-2);
}

.code :deep(.tk-str),
.code :deep(.tk-val) {
  color: var(--t-ok);
}

.code :deep(.tk-interp) {
  color: var(--t-warn);
  font-weight: 600;
}

.code :deep(.tk-kw),
.code :deep(.tk-num) {
  color: var(--t-danger);
}

.code :deep(.tk-com) {
  color: var(--vp-c-text-3);
  font-style: italic;
}

@media (max-width: 960px) {
  .hero,
  .split,
  .loop {
    grid-template-columns: minmax(0, 1fr);
  }

  .hero {
    align-items: start;
    gap: 36px;
  }

  .gallery {
    grid-template-columns: minmax(0, 1fr);
  }

  .code-pane .code {
    min-height: 0;
  }
}

@media (max-width: 520px) {
  .shell {
    padding: 0 16px;
  }

  .lede {
    font-size: 16px;
  }
}
</style>
