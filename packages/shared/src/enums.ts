/**
 * 与后端 model 常量一一对应的枚举。
 *
 * 改动前请同步 apps/backend/internal/model/common.go，两边必须一致。
 */

/** 通用状态：对应所有 status TINYINT 字段 */
export const Status = {
  /** 停用 */
  Disabled: 0,
  /** 正常 */
  Enabled: 1,
} as const
export type Status = (typeof Status)[keyof typeof Status]

/** 菜单类型：sys_menu.type */
export const MenuType = {
  /** 目录 */
  Dir: 1,
  /** 菜单（对应一个页面组件） */
  Menu: 2,
  /** 按钮（仅承载 perms，无路由） */
  Button: 3,
} as const
export type MenuType = (typeof MenuType)[keyof typeof MenuType]

/** 数据权限范围：sys_role.data_scope */
export const DataScope = {
  /** 全部数据 */
  All: 1,
  /** 自定义（按 sys_role_dept 关联的部门） */
  Custom: 2,
  /** 本部门数据 */
  Dept: 3,
  /** 本部门及子部门 */
  DeptTree: 4,
  /** 仅本人数据 */
  Self: 5,
} as const
export type DataScope = (typeof DataScope)[keyof typeof DataScope]

/** 性别：sys_user.gender */
export const Gender = {
  Unknown: 0,
  Male: 1,
  Female: 2,
} as const
export type Gender = (typeof Gender)[keyof typeof Gender]

/** 操作日志业务类型：sys_oper_log.business_type */
export const BusinessType = {
  Other: 0,
  Insert: 1,
  Update: 2,
  Delete: 3,
  Query: 4,
} as const
export type BusinessType = (typeof BusinessType)[keyof typeof BusinessType]

/**
 * 超级管理员角色标识。
 * 后端鉴权中间件识别到该角色直接放行（设计文档 §4.3）。
 */
export const SUPER_ADMIN_ROLE_CODE = 'admin'

/** 统一响应 code，对应设计文档 §9.1 */
export const ApiCode = {
  Success: 200,
  BadRequest: 400,
  Unauthorized: 401,
  Forbidden: 403,
  NotFound: 404,
  Internal: 500,
} as const
export type ApiCode = (typeof ApiCode)[keyof typeof ApiCode]
