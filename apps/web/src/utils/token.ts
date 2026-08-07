/**
 * Token 本地存取。
 *
 * 存 localStorage 的权衡：实现简单、刷新页面不丢，但无法防御 XSS。
 * 更安全的做法是把 Refresh Token 放在 HttpOnly Cookie 中，
 * 需要后端配合改造；当前实现优先保证脚手架可用性。
 */
const ACCESS_TOKEN_KEY = 'rbac_access_token'
const REFRESH_TOKEN_KEY = 'rbac_refresh_token'

export function getAccessToken(): string {
  return localStorage.getItem(ACCESS_TOKEN_KEY) ?? ''
}

export function getRefreshToken(): string {
  return localStorage.getItem(REFRESH_TOKEN_KEY) ?? ''
}

export function setTokens(accessToken: string, refreshToken: string): void {
  localStorage.setItem(ACCESS_TOKEN_KEY, accessToken)
  localStorage.setItem(REFRESH_TOKEN_KEY, refreshToken)
}

export function clearTokens(): void {
  localStorage.removeItem(ACCESS_TOKEN_KEY)
  localStorage.removeItem(REFRESH_TOKEN_KEY)
}
