import { h } from 'vue'
import DefaultTheme from 'vitepress/theme'
import './custom.css'
import AiAgent from './components/AiAgent.vue'

export default {
  extends: DefaultTheme,
  Layout() {
    return h(DefaultTheme.Layout, null, {
      'layout-bottom': () => h(AiAgent),
    })
  },
}
