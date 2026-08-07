/**
 * 路由守卫：登录拦截 + 动态路由重建 + 权限校验。
 */
import type { Router } from 'vue-router'
import { message } from 'ant-design-vue'
import { usePermissionStore } from '@/store/permission'
import { useUserStore } from '@/store/user'

const WHITE_LIST = ['/login', '/403', '/404']
const APP_TITLE = import.meta.env.VITE_APP_TITLE ?? '权限管理后台'

export function setupGuard(router: Router): void {
  router.beforeEach(async (to) => {
    const userStore = useUserStore()
    const permissionStore = usePermissionStore()

    // 已登录用户访问登录页，直接回首页
    if (to.path === '/login' && userStore.isLoggedIn) {
      return { path: '/' }
    }

    if (!userStore.isLoggedIn) {
      if (WHITE_LIST.includes(to.path) || to.meta.public) {
        return true
      }
      // 记录来源，登录后跳回原目标页
      return { path: '/login', query: { redirect: to.fullPath } }
    }

    /*
     * 处理「F5 刷新后动态路由丢失」（设计文档 附-1）。
     *
     * Pinia 状态在刷新后清空，但 localStorage 里的 Token 还在，
     * 此时必须重新拉取菜单并 addRoute，否则目标路由匹配不到而白屏。
     */
    if (!permissionStore.loaded) {
      try {
        await userStore.fetchInfo()
        const routes = await permissionStore.generateRoutes()
        routes.forEach((route) => router.addRoute(route))

        /*
         * 用 replace 重新进入目标地址。
         *
         * 新路由是本次守卫执行「之后」才注册的，当前这次导航仍按旧路由表匹配，
         * 会落到 404。因此必须重新导航一次，让新路由生效。
         */
        return { ...to, replace: true }
      } catch {
        // 拉取失败（Token 失效、后端异常等）：清理登录态回登录页
        userStore.resetState()
        permissionStore.resetState()
        message.error('获取用户信息失败，请重新登录')
        return { path: '/login', query: { redirect: to.fullPath } }
      }
    }

    // 匹配不到任何路由，交给 404
    if (to.matched.length === 0) {
      return { path: '/404' }
    }

    // 菜单级权限兜底校验：有 perms 要求但用户不具备时跳 403
    const requiredPerm = to.meta.perms as string | undefined
    if (requiredPerm && !userStore.hasPerm(requiredPerm)) {
      return { path: '/403' }
    }

    return true
  })

  router.afterEach((to) => {
    const title = to.meta.title as string | undefined
    document.title = title ? `${title} · ${APP_TITLE}` : APP_TITLE
  })
}
