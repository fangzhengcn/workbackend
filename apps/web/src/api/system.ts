/** 菜单与部门管理接口 */
import type { DeptNode, MenuNode } from '@workbackend/shared'
import { http } from '@/utils/request'

/** 完整菜单树（管理用，含隐藏与停用项） */
export function getMenuTree() {
  return http.get<MenuNode[]>('/menus/tree')
}

export interface MenuPayload {
  parentId?: number
  name?: string
  type?: number
  path?: string
  component?: string
  perms?: string
  icon?: string
  sort?: number
  visible?: number
  status?: number
  isFrame?: number
}

export function createMenu(data: MenuPayload) {
  return http.post<null>('/menus', data)
}

export function updateMenu(id: number, data: MenuPayload) {
  return http.put<null>(`/menus/${id}`, data)
}

export function deleteMenu(id: number) {
  return http.delete<null>(`/menus/${id}`)
}

/** 部门树 */
export function getDeptTree() {
  return http.get<DeptNode[]>('/depts/tree')
}

export interface DeptPayload {
  parentId?: number
  name?: string
  sort?: number
  leader?: string
  phone?: string
  status?: number
}

export function createDept(data: DeptPayload) {
  return http.post<null>('/depts', data)
}

export function updateDept(id: number, data: DeptPayload) {
  return http.put<null>(`/depts/${id}`, data)
}

export function deleteDept(id: number) {
  return http.delete<null>(`/depts/${id}`)
}
