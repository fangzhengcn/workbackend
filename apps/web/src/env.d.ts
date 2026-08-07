/// <reference types="vite/client" />

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<{}, {}, any>
  export default component
}

interface ImportMetaEnv {
  /** 接口基础路径，默认 /api/v1 */
  readonly VITE_API_BASE_URL?: string
  /** 开发代理目标后端地址 */
  readonly VITE_API_TARGET?: string
  /** 站点标题 */
  readonly VITE_APP_TITLE?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
