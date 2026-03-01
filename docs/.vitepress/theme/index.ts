import { h } from 'vue'
import DefaultTheme from 'vitepress/theme'
import './custom.css'
import AiAgent from './components/AiAgent.vue'
import ArchitectureDiagram from './components/ArchitectureDiagram.vue'
import HeroPill from './components/HeroPill.vue'
import FlickeringGrid from './components/FlickeringGrid.vue'

export default {
  extends: DefaultTheme,
  Layout() {
    return h(DefaultTheme.Layout, null, {
      'layout-bottom': () => [h(AiAgent), h(FlickeringGrid)],
      'home-hero-info-before': () => h(HeroPill),
      'home-features-before': () => h(ArchitectureDiagram),
    })
  },
}
