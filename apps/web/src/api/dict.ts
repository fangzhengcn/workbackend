/** 字典管理接口 */
import type { PageResult } from '@workbackend/shared'
import { http } from '@/utils/request'

/** 字典类型 */
export interface DictTypeItem {
  id: number
  name: string
  type: string
  status: number
  remark: string
  createdAt: string
}

/** 字典数据项 */
export interface DictDataItem {
  id: number
  dictTypeId: number
  dictType: string
  label: string
  value: string
  sort: number
  isDefault: number
  status: number
  remark: string
  createdAt: string
}

export interface DictTypeQuery {
  page?: number
  size?: number
  name?: string
  type?: string
  status?: number
}

export interface DictTypePayload {
  name?: string
  type?: string
  status?: number
  remark?: string
}

export interface DictDataQuery {
  page?: number
  size?: number
  dictType?: string
  label?: string
  status?: number
}

export interface DictDataPayload {
  /** 新增时必填；归属类型不可修改，故编辑时不传 */
  dictTypeId?: number
  label?: string
  value?: string
  sort?: number
  isDefault?: number
  status?: number
  remark?: string
}

// ---- 字典类型 ----

export function listDictTypes(params: DictTypeQuery) {
  return http.get<PageResult<DictTypeItem>>('/dicts/types', params)
}

export function createDictType(data: DictTypePayload) {
  return http.post<null>('/dicts/types', data)
}

export function updateDictType(id: number, data: DictTypePayload) {
  return http.put<null>(`/dicts/types/${id}`, data)
}

/** 删除类型会级联删除其下全部数据 */
export function deleteDictType(id: number) {
  return http.delete<null>(`/dicts/types/${id}`)
}

// ---- 字典数据 ----

export function listDictData(params: DictDataQuery) {
  return http.get<PageResult<DictDataItem>>('/dicts/data', params)
}

/**
 * 按类型取启用的字典项，供业务页面下拉框使用。
 * 路径含 /type 段是为了与 /data/:id 区分开（否则 gin 路由冲突）。
 */
export function getDictDataByType(dictType: string) {
  return http.get<DictDataItem[]>(`/dicts/data/type/${dictType}`)
}

export function createDictData(data: DictDataPayload) {
  return http.post<null>('/dicts/data', data)
}

export function updateDictData(id: number, data: DictDataPayload) {
  return http.put<null>(`/dicts/data/${id}`, data)
}

export function deleteDictData(id: number) {
  return http.delete<null>(`/dicts/data/${id}`)
}
