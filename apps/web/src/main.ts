import { createApp } from 'vue'
import { createPinia } from 'pinia'
// ant-design-vue v4 使用 CSS-in-JS，只需引入 reset 样式；
// 组件由 unplugin-vue-components 按需自动引入，故不做全量 app.use(Antd)。
import 'ant-design-vue/dist/reset.css'

import App from './App.vue'
import router from './router'
import { setupDirectives } from './directives/permission'
import './assets/style.css'

const app = createApp(App)

app.use(createPinia())
// 路由守卫内部会用到 store，因此必须在 Pinia 之后注册
app.use(router)
setupDirectives(app)

app.mount('#app')
