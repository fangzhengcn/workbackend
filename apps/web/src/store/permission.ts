/**
 * 权限状态：拉取菜单树并转换为 Vue Router 动态路由。
 */
import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { RouteRecordRaw } from 'vue-router'
import { MenuType, Status, type MenuNode } from '@workbackend/shared'
import { getUserMenus } from '@/api/auth'
import BasicLayout from '@/layouts/BasicLayout.vue'

/**
 * 页面组件按需加载。
 *
 * import.meta.glob 在构建期静态收集 views 下所有组件，
 * 因为 Vite 无法分析纯运行时拼接的动态 import 路径。
 */
const viewModules = import.meta.glob('../views/**/*.vue')

/**
 * 把后端返回的 component 字段（如 "system/user/index"）解析为组件加载器。
 */
function resolveComponent(component: string) {
  const target = `../views/${component}.vue`
  const loader = viewModules[target]
  if (loader) return loader
  // 组件路径配错时不应白屏，退化为 404 页并在控制台留下线索
  console.error(`[permission] 未找到组件: ${component}，请检查 sys_menu.component 配置`)
  return viewModules['../views/error/404.vue']
}

/** 菜单节点转路由记录；返回 null 表示该节点不产生路由 */
function menuToRoute(menu: MenuNode): RouteRecordRaw | null {
  // 按钮只承载 perms，不生成路由
  if (menu.type === MenuType.Button) return null
  // 停用的菜单不生成路由，避免绕过显隐直接访问
  if (menu.status === Status.Disabled) return null

  const children = (menu.children ?? [])
    .map(menuToRoute)
    .filter((route): route is RouteRecordRaw => route !== null)

  // 目录本身无页面，只作为父级容器
  if (menu.type === MenuType.Dir) {
    if (children.length === 0) return null
    return {
      path: menu.path,
      name: `dir-${menu.id}`,
      redirect: children[0].path,
      meta: { title: menu.name, icon: menu.icon, hidden: menu.visible === Status.Disabled },
      children,
    }
  }

  return {
    path: menu.path,
    name: `menu-${menu.id}`,
    component: resolveComponent(menu.component),
    meta: {
      title: menu.name,
      icon: menu.icon,
      perms: menu.perms,
      hidden: menu.visible === Status.Disabled,
    },
    children: children.length > 0 ? children : undefined,
  }
}

export const usePermissionStore = defineStore('permission', () => {
  /** 后端返回的原始菜单树，供侧边栏渲染 */
  const menus = ref<MenuNode[]>([])
  /** 已生成的动态路由，登出时用于逐个移除 */
  const dynamicRoutes = ref<RouteRecordRaw[]>([])
  /** 路由是否已构建，路由守卫据此决定是否需要拉取 */
  const loaded = ref(false)

  /**
   * 拉取菜单并生成动态路由。
   * 所有业务路由都挂在 BasicLayout 之下，共享侧边栏与顶栏。
   */
  async function generateRoutes(): Promise<RouteRecordRaw[]> {
    // 未被授予任何菜单的角色，接口可能返回 null；容错成空数组。
    // 否则 .map() 会抛「Cannot read properties of null」，
    // 表现为登录成功却停在登录页，报错信息与真实原因（无菜单授权）完全无关。
    const tree = (await getUserMenus()) ?? []
    menus.value = tree

    const children = tree
      .map(menuToRoute)
      .filter((route): route is RouteRecordRaw => route !== null)

    const layoutRoute: RouteRecordRaw = {
      path: '/',
      name: 'layout',
      component: BasicLayout,
      redirect: '/dashboard',
      children: [
        {
          path: 'dashboard',
          name: 'dashboard',
          component: () => import('@/views/dashboard/index.vue'),
          meta: { title: '首页', icon: 'dashboard', affix: true },
        },
        {
          /*
           * 个人中心与菜单授权无关：任何登录用户都该能查看和修改自己的资料，
           * 所以固定注册而不走 sys_menu。
           * hidden 让它不出现在侧边栏——入口在右上角头像下拉里。
           */
          path: 'profile',
          name: 'profile',
          component: () => import('@/views/profile/index.vue'),
          meta: { title: '个人中心', hidden: true },
        },
        ...children,
      ],
    }

    dynamicRoutes.value = [layoutRoute]
    loaded.value = true
    return dynamicRoutes.value
  }

  /** 重置权限状态，登出或切换账号时调用 */
  function resetState(): void {
    menus.value = []
    dynamicRoutes.value = []
    loaded.value = false
  }

  return { menus, dynamicRoutes, loaded, generateRoutes, resetState }
})
