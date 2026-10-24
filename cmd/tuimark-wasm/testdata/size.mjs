// Size budget of tuimark.wasm (ADR 0033): prints raw, gzip -9, and brotli
// q11 sizes and fails when brotli is above 1.8 MB or gzip above 2.5 MB
// (raised in 0.5.0: the <img> decoders — png/jpeg/gif plus x/image webp —
// legitimately cost ~0.3 MB and the playground renders example images).
// (decimal megabytes). Brotli and gzip come from Node's zlib, so the check
// never depends on a `brotli` binary being installed.
//
//   node cmd/tuimark-wasm/testdata/size.mjs bin/tuimark.wasm

import { appendFileSync, readFileSync } from 'node:fs'
import { brotliCompressSync, constants, gzipSync } from 'node:zlib'

const BROTLI_MAX = 1_800_000
const GZIP_MAX = 2_500_000

const file = process.argv[2]
if (!file) {
  console.error('usage: size.mjs TUIMARK.wasm')
  process.exit(1)
}
const raw = readFileSync(file)
const gz = gzipSync(raw, { level: 9 }).length
const br = brotliCompressSync(raw, {
  params: {
    [constants.BROTLI_PARAM_QUALITY]: 11,
    [constants.BROTLI_PARAM_SIZE_HINT]: raw.length,
    [constants.BROTLI_PARAM_LGWIN]: 24,
  },
}).length
const mb = (n) => (n / 1e6).toFixed(2) + ' MB'
const lines = [
  `raw    ${mb(raw.length)} (${raw.length} bytes)`,
  `gzip   ${mb(gz)} (budget ${mb(GZIP_MAX)})`,
  `brotli ${mb(br)} (budget ${mb(BROTLI_MAX)})`,
]
console.log(lines.join('\n'))
if (process.env.GITHUB_STEP_SUMMARY) {
  appendFileSync(process.env.GITHUB_STEP_SUMMARY, '### tuimark.wasm size\n```\n' + lines.join('\n') + '\n```\n')
}
if (br > BROTLI_MAX || gz > GZIP_MAX) {
  console.error('tuimark.wasm is over its size budget')
  process.exit(1)
}
