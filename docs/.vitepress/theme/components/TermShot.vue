<script setup lang="ts">
import { computed } from 'vue'
import { withBase } from 'vitepress'
import shots from '../shots.json'

// A screenshot rendered by docs/scripts/generate.mjs from `tuimark play`
// output. When the shot has a light variant, the one matching the site's
// appearance is shown.
const props = withDefaults(
  defineProps<{
    name: string
    eager?: boolean
    bare?: boolean
  }>(),
  { eager: false, bare: false },
)

type Shot = { name: string; light: boolean; cols: number; rows: number; alt: string; cmd: string }

const shot = computed<Shot>(() => {
  const s = (shots as Shot[]).find((x) => x.name === props.name)
  if (!s) throw new Error(`TermShot: no shot named "${props.name}" in shots.json`)
  return s
})

// Must match the cell geometry in docs/scripts/svg.mjs.
const width = computed(() => Math.round(shot.value.cols * 9.6 + 28))
const height = computed(() => shot.value.rows * 20 + 28)
</script>

<template>
  <figure class="term-shot" :class="{ bare }" :style="bare ? undefined : { maxWidth: `${Math.round(width * 1.2)}px` }">
    <div v-if="!bare" class="term-bar">
      <span class="dots" aria-hidden="true"><i /><i /><i /></span>
      <code class="cmd"><span class="prompt">$</span> {{ shot.cmd }}</code>
      <span class="size">{{ shot.cols }}×{{ shot.rows }}</span>
    </div>
    <div class="term-body">
      <img
        :class="['shot', shot.light ? 'dark-only' : '']"
        :src="withBase(`/shots/${shot.name}.svg`)"
        :alt="shot.alt"
        :width="width"
        :height="height"
        :loading="eager ? 'eager' : 'lazy'"
        decoding="async"
      />
      <img
        v-if="shot.light"
        class="shot light-only"
        :src="withBase(`/shots/${shot.name}-light.svg`)"
        :alt="shot.alt"
        :width="width"
        :height="height"
        :loading="eager ? 'eager' : 'lazy'"
        decoding="async"
      />
    </div>
    <figcaption v-if="$slots.default"><slot /></figcaption>
  </figure>
</template>

<style scoped>
.term-shot {
  margin: 28px 0;
}

.term-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
  border: 1px solid var(--t-border);
  border-bottom: 0;
  border-radius: 9px 9px 0 0;
  padding: 8px 12px;
  background: var(--t-panel);
}

.dots {
  display: inline-flex;
  flex: 0 0 auto;
  gap: 6px;
}

.dots i {
  width: 9px;
  height: 9px;
  border: 1px solid var(--t-border);
  border-radius: 50%;
}

.cmd {
  flex: 1 1 auto;
  min-width: 0;
  overflow-x: auto;
  color: var(--vp-c-text-2);
  font-family: var(--vp-font-family-mono);
  font-size: 12px;
  line-height: 1.5;
  white-space: nowrap;
  scrollbar-width: none;
}

.cmd::-webkit-scrollbar {
  display: none;
}

.prompt {
  color: var(--vp-c-brand-1);
  font-weight: 700;
}

.size {
  flex: 0 0 auto;
  color: var(--vp-c-text-3);
  font-family: var(--vp-font-family-mono);
  font-size: 11.5px;
}

.term-body {
  overflow: hidden;
  border: 1px solid var(--t-border);
  border-radius: 0 0 9px 9px;
  line-height: 0;
}

.bare .term-body {
  border-radius: 9px;
}

.shot {
  display: block;
  width: 100%;
  height: auto;
}

.light-only {
  display: none;
}

html:not(.dark) .light-only {
  display: block;
}

html:not(.dark) .dark-only {
  display: none;
}

figcaption {
  margin-top: 10px;
  color: var(--vp-c-text-3);
  font-size: 13.5px;
  line-height: 1.55;
}
</style>
