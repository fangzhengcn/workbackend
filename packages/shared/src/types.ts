/**
 * 与后端 internal/vo 对应的接口类型。
 *
 * 注意：后端 VO 字段变更时需同步本文件。
 */
import type { DataScope, Gender, MenuType, Status } from './enums'

/** 统一响应结构（设计文档 §9.1） */
export interface ApiResult<T = unknown> {
  code: number
  message: string
  data: T
}

/** 分页返回结构 */
export interface PageResult<T> {
  list: T[]
  total: number
  page: number
  size: number
}

/** 分页查询公共参数 */
export interface PageQuery {
  page?: number
  size?: number
}

/** 登录请求 */
export interface LoginRequest {
  username: string
  password: string
  /** 验证码 ID，与 captchaCode 配对校验 */
  captchaId?: string
  captchaCode?: string
}

/** 登录响应 */
export interface LoginResult {
  accessToken: string
  refreshToken: string
  /** Access Token 有效期（秒） */
  expiresIn: number
  tokenType: 'Bearer'
}

/** 图形验证码 */
export interface CaptchaResult {
  captchaId: string
  /** base64 编码的图片，可直接用于 img src */
  imageBase64: string
}

/** 当前登录用户信息（GET /auth/info） */
export interface UserInfo {
  id: number
  username: string
  /** 已脱敏 */
  nickname: string
  avatar: string
  /** 已脱敏，如 u**r@example.com */
  email: string
  /** 已脱敏，如 138****8000 */
  phone: string
  gender: Gender
  deptId: number | null
  deptName: string
  status: Status
  /** 角色标识集合 */
  roles: string[]
  /** 权限标识集合；超级管理员为 ['*:*:*'] */
  perms: string[]
  lastLoginAt: string | null
}

/** 菜单树节点（GET /auth/menus） */
export interface MenuNode {
  id: number
  parentId: number
  name: string
  type: MenuType
  path: string
  component: string
  perms: string
  icon: string
  sort: number
  visible: Status
  /** 停用的菜单不生成路由，防止绕过显隐直接访问 */
  status: Status
  isFrame: number
  children?: MenuNode[]
}

/** 用户列表项 */
export interface UserItem {
  id: number
  username: string
  nickname: string
  email: string
  phone: string
  gender: Gender
  deptId: number | null
  deptName: string
  status: Status
  remark: string
  roles: RoleItem[]
  lastLoginAt: string | null
  createdAt: string
}

/** 角色 */
export interface RoleItem {
  id: number
  name: string
  code: string
  sort: number
  dataScope: DataScope
  status: Status
  remark: string
  createdAt: string
}

/** 部门树节点 */
export interface DeptNode {
  id: number
  parentId: number
  name: string
  sort: number
  leader: string
  phone: string
  status: Status
  children?: DeptNode[]
}

/** 字典数据项 */
export interface DictDataItem {
  id: number
  dictType: string
  label: string
  value: string
  sort: number
  isDefault: number
  status: Status
  remark: string
}
