/** 日志查询接口（操作日志 / 登录日志） */
import type { PageResult } from '@workbackend/shared'
import { download, http } from '@/utils/request'

/** 操作日志列表项（不含请求参数与响应体，那两项只在详情返回） */
export interface OperLogItem {
  id: number
  title: string
  businessType: number
  method: string
  requestUrl: string
  operUserId: number | null
  operName: string
  operIp: string
  status: number
  errorMsg: string
  /** 耗时，毫秒 */
  costTime: number
  createdAt: string
}

/** 操作日志详情 */
export interface OperLogDetail extends OperLogItem {
  /** 已由后端写入时脱敏（密码置 ***、手机号邮箱打码） */
  requestParam: string
  jsonResult: string
}

export interface LoginLogItem {
  id: number
  username: string
  ipaddr: string
  location: string
  browser: string
  os: string
  status: number
  msg: string
  loginTime: string
}

export interface OperLogQuery {
  page?: number
  size?: number
  title?: string
  operName?: string
  businessType?: number
  status?: number
  beginTime?: string
  endTime?: string
}

export interface LoginLogQuery {
  page?: number
  size?: number
  username?: string
  ipaddr?: string
  status?: number
  beginTime?: string
  endTime?: string
}

// ---- 操作日志 ----

export function listOperLogs(params: OperLogQuery) {
  return http.get<PageResult<OperLogItem>>('/oper-logs', params)
}

export function getOperLog(id: number) {
  return http.get<OperLogDetail>(`/oper-logs/${id}`)
}

/** 批量删除，单次上限 200 条 */
export function deleteOperLogs(ids: number[]) {
  return http.delete<null>('/oper-logs', { ids })
}

export function cleanOperLogs() {
  return http.delete<null>('/oper-logs/clean')
}

/** 导出操作日志为 CSV；不含请求参数与响应体两个大字段 */
export function exportOperLogs(params: OperLogQuery) {
  return download('/oper-logs/export', params)
}

// ---- 登录日志 ----

export function listLoginLogs(params: LoginLogQuery) {
  return http.get<PageResult<LoginLogItem>>('/login-logs', params)
}

export function deleteLoginLogs(ids: number[]) {
  return http.delete<null>('/login-logs', { ids })
}

export function cleanLoginLogs() {
  return http.delete<null>('/login-logs/clean')
}

/** 导出登录日志为 CSV */
export function exportLoginLogs(params: LoginLogQuery) {
  return download('/login-logs/export', params)
}
