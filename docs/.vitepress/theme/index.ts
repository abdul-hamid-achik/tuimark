import type { Theme } from 'vitepress'
import DefaultTheme from 'vitepress/theme'
import '@fontsource-variable/geist'
import '@fontsource-variable/jetbrains-mono'
import CommandCopy from './components/CommandCopy.vue'
import HomeLanding from './components/HomeLanding.vue'
import InstallPanel from './components/InstallPanel.vue'
import TermShot from './components/TermShot.vue'
import './custom.css'

export default {
  extends: DefaultTheme,
  enhanceApp({ app }) {
    app.component('CommandCopy', CommandCopy)
    app.component('HomeLanding', HomeLanding)
    app.component('InstallPanel', InstallPanel)
    app.component('TermShot', TermShot)
  },
} satisfies Theme
