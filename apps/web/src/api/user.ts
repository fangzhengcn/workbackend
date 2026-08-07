/** 用户管理接口 */
import type { PageResult, UserItem } from '@workbackend/shared'
import { download, http } from '@/utils/request'

export interface UserQuery {
  page?: number
  size?: number
  username?: string
  /** 精确匹配：后端会转成 HMAC 盲索引查询，不支持模糊搜索 */
  phone?: string
  status?: number
  deptId?: number
}

export interface UserPayload {
  username?: string
  password?: string
  nickname?: string
  email?: string
  phone?: string
  gender?: number
  deptId?: number | null
  status?: number
  remark?: string
  roleIds?: number[]
}

export function listUsers(params: UserQuery) {
  return http.get<PageResult<UserItem>>('/users', params)
}

export function getUser(id: number) {
  return http.get<UserItem>(`/users/${id}`)
}

export function createUser(data: UserPayload) {
  return http.post<null>('/users', data)
}

export function updateUser(id: number, data: UserPayload) {
  return http.put<null>(`/users/${id}`, data)
}

export function deleteUser(id: number) {
  return http.delete<null>(`/users/${id}`)
}

export function resetPassword(id: number, password: string) {
  return http.put<null>(`/users/${id}/password`, { password })
}

export function assignRoles(id: number, roleIds: number[]) {
  return http.put<null>(`/users/${id}/roles`, { roleIds })
}

/**
 * 导出用户为 CSV，筛选条件与列表一致。
 *
 * 导出的手机号/邮箱是脱敏值（后端与列表用同一套 VO），
 * 需要真实值请走数据库导出流程并做审批。
 */
export function exportUsers(params: UserQuery) {
  return download('/users/export', params)
}
