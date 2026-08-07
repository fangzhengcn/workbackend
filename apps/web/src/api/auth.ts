/** 认证相关接口 */
import type {
  CaptchaResult,
  LoginRequest,
  LoginResult,
  MenuNode,
  UserInfo,
} from '@workbackend/shared'
import { http } from '@/utils/request'

/** 登录，返回 Token */
export function login(data: LoginRequest) {
  // silent：登录失败提示由页面自行渲染在表单上，避免重复弹窗
  return http.post<LoginResult>('/auth/login', data, { silent: true })
}

/** 登出，后端将当前 Token 写入 Redis 黑名单 */
export function logout() {
  return http.post<null>('/auth/logout')
}

/** 获取图形验证码 */
export function getCaptcha() {
  return http.get<CaptchaResult>('/auth/captcha')
}

/** 获取当前用户信息与权限集合 */
export function getUserInfo() {
  return http.get<UserInfo>('/auth/info')
}

/** 获取当前用户菜单树，用于生成动态路由 */
export function getUserMenus() {
  return http.get<MenuNode[]>('/auth/menus')
}

/** 用 Refresh Token 换取新的 Access Token */
export function refreshToken(token: string) {
  return http.post<LoginResult>('/auth/refresh', { refreshToken: token }, { silent: true })
}

/** 修改自己的个人资料（个人中心） */
export interface ProfilePayload {
  nickname?: string
  email?: string
  phone?: string
  gender?: number
}

export function updateProfile(data: ProfilePayload) {
  return http.put<null>('/auth/profile', data)
}

/** 修改自己的密码；成功后 Token 仍有效，但建议重新登录 */
export function changePassword(oldPassword: string, newPassword: string) {
  return http.put<null>('/auth/password', { oldPassword, newPassword })
}

/**
 * 上传头像，返回可访问的 URL。
 *
 * 用 FormData 而非 JSON：后端按 multipart 解析。
 * 不手动设置 Content-Type——浏览器会自动带上含 boundary 的正确值，
 * 手写会漏掉 boundary 导致后端解析失败。
 */
export function uploadAvatar(file: File) {
  const form = new FormData()
  form.append('file', file)
  return http.post<string>('/auth/avatar', form)
}
