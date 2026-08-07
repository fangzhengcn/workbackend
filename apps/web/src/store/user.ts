/**
 * 用户状态：Token、用户信息、角色与权限集合。
 */
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { SUPER_ADMIN_ROLE_CODE, type LoginRequest, type UserInfo } from '@workbackend/shared'
import * as authApi from '@/api/auth'
import { clearTokens, getAccessToken, setTokens } from '@/utils/token'

/** 超级管理员的权限通配符，由后端在 perms 中返回 */
const ALL_PERMS = '*:*:*'

export const useUserStore = defineStore('user', () => {
  const token = ref(getAccessToken())
  const info = ref<UserInfo | null>(null)
  const roles = ref<string[]>([])
  /** 用 Set 存权限，判断复杂度 O(1) */
  const perms = ref<Set<string>>(new Set())

  const isLoggedIn = computed(() => token.value !== '')
  const isSuperAdmin = computed(
    () => roles.value.includes(SUPER_ADMIN_ROLE_CODE) || perms.value.has(ALL_PERMS),
  )

  /**
   * 判断是否拥有某权限点。
   *
   * 仅用于控制前端显隐（体验优化），绝不能当作安全边界，
   * 后端接口必须再校验一次（设计文档 §1.2）。
   */
  function hasPerm(code: string | string[]): boolean {
    if (isSuperAdmin.value) return true
    const codes = Array.isArray(code) ? code : [code]
    // 多个权限点之间是「或」关系：任一满足即可
    return codes.some((item) => perms.value.has(item))
  }

  /** 判断是否拥有某角色 */
  function hasRole(code: string): boolean {
    return roles.value.includes(code)
  }

  async function login(payload: LoginRequest): Promise<void> {
    const result = await authApi.login(payload)
    setTokens(result.accessToken, result.refreshToken)
    token.value = result.accessToken
  }

  /** 拉取用户信息与权限，登录后与刷新页面时调用 */
  async function fetchInfo(): Promise<UserInfo> {
    const data = await authApi.getUserInfo()
    info.value = data
    roles.value = data.roles ?? []
    perms.value = new Set(data.perms ?? [])
    return data
  }

  /** 清空本地登录态，不发请求 */
  function resetState(): void {
    clearTokens()
    token.value = ''
    info.value = null
    roles.value = []
    perms.value = new Set()
  }

  async function logout(): Promise<void> {
    try {
      await authApi.logout()
    } catch {
      // 登出接口失败也要清本地状态，否则用户会卡在登录态出不去
    } finally {
      resetState()
    }
  }

  return {
    token,
    info,
    roles,
    perms,
    isLoggedIn,
    isSuperAdmin,
    hasPerm,
    hasRole,
    login,
    fetchInfo,
    logout,
    resetState,
  }
})
