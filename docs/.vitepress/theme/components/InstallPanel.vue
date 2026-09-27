<script setup lang="ts">
import { ref } from 'vue'
import CommandCopy from './CommandCopy.vue'

type Method = 'go' | 'brew' | 'release' | 'library'

withDefaults(defineProps<{ headingLevel?: 'h2' | 'h3' }>(), { headingLevel: 'h3' })

const active = ref<Method>('go')

const methods: Array<{ id: Method; label: string; note: string }> = [
  { id: 'go', label: 'go install', note: 'Go 1.22+' },
  { id: 'brew', label: 'homebrew', note: 'macOS, Linux' },
  { id: 'release', label: 'release archive', note: 'no toolchain' },
  { id: 'library', label: 'go library', note: 'embed it' },
]

const goCommand = 'go install github.com/abdul-hamid-achik/tuimark/cmd/tuimark@latest'
const brewCommand = 'brew install --cask abdul-hamid-achik/tap/tuimark'
const archiveCommand =
  'shasum -a 256 -c checksums.txt --ignore-missing\ntar -xzf tuimark_<version>_<Os>_<arch>.tar.gz tuimark\nmkdir -p "$HOME/.local/bin"\ninstall -m 0755 tuimark "$HOME/.local/bin/tuimark"'
const libraryCommand = 'go get github.com/abdul-hamid-achik/tuimark@latest'
</script>

<template>
  <div class="install-panel">
    <div class="install-tabs" role="tablist" aria-label="Ways to install Tuimark">
      <button
        v-for="m in methods"
        :key="m.id"
        type="button"
        role="tab"
        :class="{ active: active === m.id }"
        :aria-selected="active === m.id"
        @click="active = m.id"
      >
        <span>{{ active === m.id ? '▸' : ' ' }}{{ m.label }}</span>
        <small>{{ m.note }}</small>
      </button>
    </div>

    <div class="install-content" role="tabpanel">
      <div v-if="active === 'go'" class="method">
        <div class="copy">
          <component :is="headingLevel">Install the CLI with Go</component>
          <p>
            One static binary, no CGO. It lands in <code>$(go env GOPATH)/bin</code>; make sure that is on
            your <code>PATH</code>.
          </p>
        </div>
        <CommandCopy :command="goCommand" />
        <ol class="steps">
          <li><span>01</span><div><strong>Check it</strong><code>tuimark version</code></div></li>
          <li><span>02</span><div><strong>Read the briefing</strong><code>tuimark agents</code></div></li>
          <li><span>03</span><div><strong>Render a frame</strong><code>tuimark dump app.tui</code></div></li>
        </ol>
      </div>

      <div v-else-if="active === 'brew'" class="method">
        <div class="copy">
          <component :is="headingLevel">Install a release with Homebrew</component>
          <p>The tap picks the right build for macOS or Linux on Apple Silicon, Intel, or ARM64.</p>
        </div>
        <CommandCopy :command="brewCommand" />
      </div>

      <div v-else-if="active === 'release'" class="method">
        <div class="copy">
          <component :is="headingLevel">Download a prebuilt binary</component>
          <p>
            Every release ships archives for macOS, Linux, and Windows on x86_64 and arm64, named like
            <code>tuimark_0.2.0_Darwin_arm64.tar.gz</code> (<code>.zip</code> on Windows), with a
            <code>checksums.txt</code>. Check the download, unpack it, and put the binary on your
            <code>PATH</code>.
          </p>
        </div>
        <CommandCopy :command="archiveCommand" prompt="" />
        <a class="release-link" href="https://github.com/abdul-hamid-achik/tuimark/releases/latest">
          Browse the latest release <span aria-hidden="true">→</span>
        </a>
      </div>

      <div v-else class="method">
        <div class="copy">
          <component :is="headingLevel">Add the runtime to a Go program</component>
          <p>
            The package <code>github.com/abdul-hamid-achik/tuimark</code> loads a document, binds your data,
            and runs it in the terminal. Its whole API is nine functions.
          </p>
        </div>
        <CommandCopy :command="libraryCommand" />
      </div>
    </div>
  </div>
</template>

<style scoped>
.install-panel {
  overflow: hidden;
  border: 1px solid var(--t-border);
  border-radius: 9px;
  background: var(--vp-c-bg);
}

.install-tabs {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 6px;
  border-bottom: 1px solid var(--vp-c-divider);
  padding: 12px 14px;
  background: var(--t-panel);
}

.install-tabs button {
  display: inline-flex;
  align-items: baseline;
  gap: 10px;
  border: 0;
  border-radius: 3px;
  padding: 3px 10px;
  background: transparent;
  color: var(--vp-c-text-2);
  cursor: pointer;
  font-family: var(--vp-font-family-mono);
  font-size: 13.5px;
  white-space: pre;
}

.install-tabs button:hover {
  color: var(--vp-c-text-1);
}

.install-tabs button.active {
  background: var(--vp-c-brand-1);
  color: var(--t-select-fg);
  font-weight: 700;
}

.install-tabs small {
  color: var(--vp-c-text-3);
  font-size: 11.5px;
  font-weight: 400;
}

.install-tabs button.active small {
  color: inherit;
  opacity: 0.75;
}

.install-content {
  padding: clamp(20px, 3.5vw, 30px);
}

.method {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.copy :is(h2, h3) {
  margin: 0 0 8px;
  border: 0;
  padding: 0;
  color: var(--vp-c-text-1);
  font-family: var(--vp-font-family-mono);
  font-size: clamp(18px, 2.3vw, 21px);
  font-weight: 600;
  letter-spacing: -0.03em;
  line-height: 1.25;
}

.copy p {
  max-width: 64ch;
  margin: 0;
  color: var(--vp-c-text-2);
  font-size: 15px;
  line-height: 1.6;
}

.copy code,
.steps code {
  border: 0;
  padding: 0;
  background: none;
  color: var(--vp-c-text-1);
  font-family: var(--vp-font-family-mono);
  font-size: 0.88em;
}

.steps {
  display: flex;
  flex-wrap: wrap;
  gap: 10px 30px;
  margin: 2px 0 0;
  border-top: 1px dashed var(--vp-c-divider);
  padding: 16px 0 0;
  list-style: none;
}

.steps li {
  display: flex;
  gap: 10px;
  align-items: baseline;
  margin: 0;
}

.steps li > span {
  color: var(--vp-c-brand-1);
  font-family: var(--vp-font-family-mono);
  font-size: 12.5px;
  font-weight: 700;
}

.steps div {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.steps strong {
  color: var(--vp-c-text-1);
  font-size: 14px;
  font-weight: 600;
}

.steps code {
  color: var(--vp-c-text-2);
  font-size: 12.5px;
}

.release-link {
  width: fit-content;
  color: var(--vp-c-brand-1);
  font-family: var(--vp-font-family-mono);
  font-size: 14px;
  font-weight: 600;
  text-decoration: none;
}

.release-link:hover {
  text-decoration: underline;
  text-underline-offset: 4px;
}
</style>
