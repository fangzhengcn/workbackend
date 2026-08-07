/**
 * Axios 封装：统一注入 Token、解包响应、处理 401/403。
 */
import axios, {
  type AxiosInstance,
  type AxiosRequestConfig,
  type InternalAxiosRequestConfig,
  type AxiosResponse,
} from 'axios'
import { message } from 'ant-design-vue'
import { ApiCode, type ApiResult } from '@workbackend/shared'
import { clearTokens, getAccessToken } from './token'

const BASE_URL = import.meta.env.VITE_API_BASE_URL ?? '/api/v1'

/** 标记本次请求不弹出错误提示，由调用方自行处理 */
export interface RequestConfig extends AxiosRequestConfig {
  silent?: boolean
}

/*
 * 不设全局 Content-Type。
 *
 * 原先写死了 application/json，结果与 FormData 上传直接冲突：
 * 它会盖掉浏览器为 multipart 自动生成的、含 boundary 的头，
 * 后端切不开表单，报「请选择要上传的图片」。
 * 试过在拦截器里删除该头，但 axios v1 的 AxiosHeaders 做了键名归一化，
 * 删除时机与方式都容易失效——不设默认值才是可靠做法：
 * axios 会按 data 类型自动推断（普通对象→json，FormData→multipart+boundary）。
 */
const instance: AxiosInstance = axios.create({
  baseURL: BASE_URL,
  timeout: 15_000,
})

/** 请求拦截器：注入 Bearer Token */
instance.interceptors.request.use(
  (config: InternalAxiosRequestConfig) => {
    const token = getAccessToken()
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  },
  (error) => Promise.reject(error),
)

/**
 * 401 只跳转一次，避免并发请求同时失败时弹出多个提示、反复跳转。
 */
let redirecting = false
function redirectToLogin(): void {
  if (redirecting) return
  redirecting = true
  clearTokens()
  const redirect = encodeURIComponent(window.location.pathname + window.location.search)
  window.location.href = `/login?redirect=${redirect}`
  // 跳转后本页即将卸载，无需复位 redirecting
}

/** 响应拦截器：解包 data、统一错误处理 */
instance.interceptors.response.use(
  (response: AxiosResponse<ApiResult>) => {
    // 文件下载（responseType=blob）的响应体不是统一结构，跳过解包。
    // 后端出错时仍会返回 JSON，那种情况由 download() 自行识别并提示。
    if (response.config.responseType === 'blob') {
      return response
    }

    const body = response.data
    // 后端始终返回 {code, message, data}，code 非 200 视为业务失败
    if (body.code !== ApiCode.Success) {
      const silent = (response.config as RequestConfig).silent
      if (!silent) message.error(body.message || '请求失败')
      return Promise.reject(new Error(body.message || '请求失败'))
    }
    return response
  },
  (error) => {
    const silent = (error.config as RequestConfig | undefined)?.silent
    const status = error.response?.status
    const body = error.response?.data as ApiResult | undefined
    const text = body?.message

    switch (status) {
      case ApiCode.Unauthorized:
        // Token 失效/过期：清理并回登录页
        if (!silent) message.error(text || '登录已过期，请重新登录')
        redirectToLogin()
        break
      case ApiCode.Forbidden:
        if (!silent) message.error(text || '无操作权限')
        break
      case undefined:
        if (!silent) message.error('网络异常，请检查网络连接')
        break
      default:
        if (!silent) message.error(text || `请求失败(${status})`)
    }
    return Promise.reject(error)
  },
)

/** 发起请求并直接返回 data 字段 */
export async function request<T>(config: RequestConfig): Promise<T> {
  const response = await instance.request<ApiResult<T>>(config)
  return response.data.data
}

export const http = {
  get: <T>(url: string, params?: unknown, config?: RequestConfig) =>
    request<T>({ ...config, url, method: 'GET', params }),
  post: <T>(url: string, data?: unknown, config?: RequestConfig) =>
    request<T>({ ...config, url, method: 'POST', data }),
  put: <T>(url: string, data?: unknown, config?: RequestConfig) =>
    request<T>({ ...config, url, method: 'PUT', data }),
  // 允许带请求体：批量删除需要在 body 里传 ID 集合，
  // 放到 query 上会在数量多时撞上 URL 长度限制。
  delete: <T>(url: string, data?: unknown, config?: RequestConfig) =>
    request<T>({ ...config, url, method: 'DELETE', data }),
}

/**
 * 下载文件（导出用）。
 *
 * 不能走 request()：那里会取 response.data.data，而下载响应体是二进制。
 * 走 Axios 而非直接 window.open，是为了带上 Authorization 头——
 * 导出接口需要鉴权，open 出去的请求不会携带它。
 */
export async function download(url: string, params?: unknown): Promise<void> {
  const response = await instance.request<Blob>({
    url,
    method: 'GET',
    params,
    responseType: 'blob',
  })

  /*
   * 后端出错时返回的是 JSON 而非文件，但 responseType=blob 会把它也包成
   * Blob，直接保存会得到一个内容是错误信息的「文件」。
   * 故先按类型判断：是 JSON 就读出来当错误提示抛出。
   */
  const blob = response.data
  if (blob.type.includes('application/json')) {
    const text = await blob.text()
    let msg = '导出失败'
    try {
      msg = (JSON.parse(text) as ApiResult).message || msg
    } catch {
      // 不是合法 JSON 就用默认提示
    }
    message.error(msg)
    throw new Error(msg)
  }

  // 文件名优先取后端 Content-Disposition 里的 filename*（含中文，已 URL 编码）
  const filename = parseFilename(response.headers['content-disposition'] as string | undefined)

  const href = window.URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = href
  link.download = filename
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
  // 必须释放：createObjectURL 的引用不会被 GC 自动回收，
  // 频繁导出会持续占用内存直到页面刷新。
  window.URL.revokeObjectURL(href)
}

/** 从 Content-Disposition 解析文件名，失败时回退到一个通用名 */
function parseFilename(disposition?: string): string {
  if (!disposition) return 'export.csv'

  // filename*=UTF-8''xxx 优先，它才承载中文名
  const utf8Match = /filename\*=UTF-8''([^;]+)/i.exec(disposition)
  if (utf8Match?.[1]) {
    try {
      return decodeURIComponent(utf8Match[1])
    } catch {
      // 编码异常则继续尝试 ASCII 形式
    }
  }
  const asciiMatch = /filename="?([^";]+)"?/i.exec(disposition)
  return asciiMatch?.[1] ?? 'export.csv'
}

export default instance
