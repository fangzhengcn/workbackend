/** 角色管理接口 */
import type { PageResult, RoleItem } from '@workbackend/shared'
import { http } from '@/utils/request'

/** 角色详情：在列表项之上附带已分配的菜单与部门 ID */
export interface RoleDetail extends RoleItem {
  menuIds: number[]
  /** 仅 dataScope=2（自定义）时有意义 */
  deptIds: number[]
}

export interface RoleQuery {
  page?: number
  size?: number
  name?: string
  code?: string
  status?: number
}

export interface RolePayload {
  name?: string
  code?: string
  sort?: number
  dataScope?: number
  status?: number
  remark?: string
  menuIds?: number[]
  /** dataScope=2（自定义）时生效 */
  deptIds?: number[]
}

export function listRoles(params: RoleQuery) {
  return http.get<PageResult<RoleItem>>('/roles', params)
}

/** 不分页的全部角色，用于用户分配角色的下拉框 */
export function listAllRoles() {
  return http.get<RoleItem[]>('/roles/all')
}

/** 角色详情，含已分配的菜单与部门 ID */
export function getRole(id: number) {
  return http.get<RoleDetail>(`/roles/${id}`)
}

export function createRole(data: RolePayload) {
  return http.post<null>('/roles', data)
}

export function updateRole(id: number, data: RolePayload) {
  return http.put<null>(`/roles/${id}`, data)
}

export function deleteRole(id: number) {
  return http.delete<null>(`/roles/${id}`)
}

/** 查询角色已分配的菜单 ID，用于回显权限树勾选状态 */
export function getRoleMenuIds(id: number) {
  return http.get<number[]>(`/roles/${id}/menus`)
}

/** 分配角色菜单权限；后端会同步刷新 Casbin 策略 */
export function assignMenus(id: number, menuIds: number[]) {
  return http.put<null>(`/roles/${id}/menus`, { menuIds })
}

/** 设置数据权限范围 */
export function setDataScope(id: number, dataScope: number, deptIds: number[]) {
  return http.put<null>(`/roles/${id}/data-scope`, { dataScope, deptIds })
}
