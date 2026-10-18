<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { withBase } from 'vitepress'
import shots from '../shots.json'

// A screenshot rendered by docs/scripts/generate.mjs from `tuimark play`
// output. When the shot has a light variant, the one matching the site's
// appearance is shown. An animated shot (an example's -demo) plays in a
// loop, with a button that swaps it for its still last frame; a reader who
// prefers reduced motion starts on the still frame.
const props = withDefaults(
  defineProps<{
    name: string
    eager?: boolean
    bare?: boolean
  }>(),
  { eager: false, bare: false },
)

type Shot = { name: string; light: boolean; cols: number; rows: number; alt: string; cmd: string; animated?: boolean }

const shot = computed<Shot>(() => {
  const s = (shots as Shot[]).find((x) => x.name === props.name)
  if (!s) throw new Error(`TermShot: no shot named "${props.name}" in shots.json`)
  return s
})

const playing = ref(true)
onMounted(() => {
  if (shot.value.animated && window.matchMedia('(prefers-reduced-motion: reduce)').matches) playing.value = false
})
const base = computed(() => (shot.value.animated && !playing.value ? `${shot.value.name}-still` : shot.value.name))

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
      <button
        v-if="shot.animated"
        type="button"
        class="play"
        :aria-pressed="!playing"
        :aria-label="playing ? 'Pause the animation' : 'Play the animation'"
        :title="playing ? 'Pause' : 'Play'"
        @click="playing = !playing"
      >
        <svg v-if="playing" viewBox="0 0 10 10" aria-hidden="true"><path d="M2 1.5h2v7H2zM6 1.5h2v7H6z" /></svg>
        <svg v-else viewBox="0 0 10 10" aria-hidden="true"><path d="M2.5 1.2 8.8 5 2.5 8.8z" /></svg>
      </button>
    </div>
    <div class="term-body">
      <img
        :class="['shot', shot.light ? 'dark-only' : '']"
        :src="withBase(`/shots/${base}.svg`)"
        :alt="shot.alt"
        :width="width"
        :height="height"
        :loading="eager ? 'eager' : 'lazy'"
        decoding="async"
      />
      <img
        v-if="shot.light"
        class="shot light-only"
        :src="withBase(`/shots/${base}-light.svg`)"
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

.play {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border: 1px solid var(--t-border);
  border-radius: 5px;
  color: var(--vp-c-text-2);
  background: transparent;
  cursor: pointer;
}

.play:hover {
  color: var(--vp-c-brand-1);
  border-color: var(--vp-c-brand-1);
}

.play svg {
  width: 10px;
  height: 10px;
  fill: currentColor;
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
