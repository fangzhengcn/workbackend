/**
 * v-permission 按钮权限指令。
 *
 * 用法：
 *   <a-button v-permission="'system:user:add'">新增</a-button>
 *   <a-button v-permission="['system:user:edit', 'system:user:add']">编辑</a-button>  // 或关系
 *
 * 无权限时直接从 DOM 移除元素，而非仅隐藏——隐藏元素仍能被开发者工具改回来触发点击。
 *
 * 再次强调：这只是体验优化，后端接口必须独立校验权限（设计文档 §1.2）。
 */
import type { App, Directive, DirectiveBinding } from 'vue'
import { useUserStore } from '@/store/user'

function checkPermission(el: HTMLElement, binding: DirectiveBinding<string | string[]>): void {
  const { value } = binding
  if (!value || (Array.isArray(value) && value.length === 0)) {
    console.warn('[v-permission] 需要提供权限标识，如 v-permission="\'system:user:add\'"')
    return
  }

  const userStore = useUserStore()
  if (!userStore.hasPerm(value)) {
    el.parentNode?.removeChild(el)
  }
}

export const permission: Directive<HTMLElement, string | string[]> = {
  mounted: checkPermission,
}

/** 注册所有自定义指令 */
export function setupDirectives(app: App): void {
  app.directive('permission', permission)
}
