/**
 * 权限标识常量。
 *
 * 命名约定：`模块:资源:操作`（设计文档「技术难点提示 5」）。
 * 这里集中定义，避免前端各处手写字符串与后端 sys_menu.perms 拼写不一致。
 *
 * 新增权限点时的同步清单：
 *   1. 在此文件添加常量
 *   2. 在 sys_menu 插入对应 type=3 的按钮记录
 *   3. 为角色分配该菜单（sys_role_menu），并刷新 Casbin 策略
 */
export const Perms = {
  user: {
    list: 'system:user:list',
    query: 'system:user:query',
    add: 'system:user:add',
    edit: 'system:user:edit',
    remove: 'system:user:remove',
    resetPwd: 'system:user:resetPwd',
    /** 分配角色 */
    assignRole: 'system:user:assignRole',
    import: 'system:user:import',
    export: 'system:user:export',
  },
  role: {
    list: 'system:role:list',
    query: 'system:role:query',
    add: 'system:role:add',
    edit: 'system:role:edit',
    remove: 'system:role:remove',
    /** 分配菜单权限 */
    assignMenu: 'system:role:assignMenu',
    /** 设置数据权限范围 */
    dataScope: 'system:role:dataScope',
  },
  menu: {
    list: 'system:menu:list',
    query: 'system:menu:query',
    add: 'system:menu:add',
    edit: 'system:menu:edit',
    remove: 'system:menu:remove',
  },
  dept: {
    list: 'system:dept:list',
    query: 'system:dept:query',
    add: 'system:dept:add',
    edit: 'system:dept:edit',
    remove: 'system:dept:remove',
  },
  dict: {
    list: 'system:dict:list',
    query: 'system:dict:query',
    add: 'system:dict:add',
    edit: 'system:dict:edit',
    remove: 'system:dict:remove',
  },
  operLog: {
    list: 'system:operlog:list',
    remove: 'system:operlog:remove',
    export: 'system:operlog:export',
  },
  loginLog: {
    list: 'system:loginlog:list',
    remove: 'system:loginlog:remove',
    export: 'system:loginlog:export',
  },
} as const

/** 所有权限标识的联合类型，用于 v-permission 的类型约束 */
export type PermCode =
  | (typeof Perms)['user'][keyof (typeof Perms)['user']]
  | (typeof Perms)['role'][keyof (typeof Perms)['role']]
  | (typeof Perms)['menu'][keyof (typeof Perms)['menu']]
  | (typeof Perms)['dept'][keyof (typeof Perms)['dept']]
  | (typeof Perms)['dict'][keyof (typeof Perms)['dict']]
  | (typeof Perms)['operLog'][keyof (typeof Perms)['operLog']]
  | (typeof Perms)['loginLog'][keyof (typeof Perms)['loginLog']]
