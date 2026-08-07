-- =====================================================================
-- 权限系统数据库 · 建表脚本
-- 项目：前后端分离 RBAC 权限管理后台
-- 技术栈：Golang + Gin + GORM + MySQL 8.0
-- 说明：
--   1. 采用 RBAC 模型：用户-角色-菜单/权限 多对多
--   2. sys_user 的 email / phone / nickname 存 AES-256-GCM 密文
--      email / phone 另设 HMAC-SHA256 盲索引列用于精确查询与去重
--   3. 主表统一软删除 (deleted_at) + 审计字段 (created_at/updated_at/...)
--   4. 字符集 utf8mb4，存储引擎 InnoDB
-- =====================================================================

SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS = 0;

CREATE DATABASE IF NOT EXISTS `rbac_admin`
    DEFAULT CHARACTER SET utf8mb4
    DEFAULT COLLATE utf8mb4_general_ci;

USE `rbac_admin`;

-- ---------------------------------------------------------------------
-- 1. 部门表 sys_dept（组织架构，树形，先建，供用户外键引用）
-- ---------------------------------------------------------------------
DROP TABLE IF EXISTS `sys_dept`;
CREATE TABLE `sys_dept` (
    `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
    `parent_id`  BIGINT UNSIGNED NOT NULL DEFAULT 0        COMMENT '父部门ID，0为顶级',
    `ancestors`  VARCHAR(255)    NOT NULL DEFAULT ''        COMMENT '祖级列表，逗号分隔，便于查子树',
    `name`       VARCHAR(64)     NOT NULL      COMMENT '部门名称',
    `sort`       INT             NOT NULL DEFAULT 0         COMMENT '显示顺序',
    `leader`     VARCHAR(64)          DEFAULT NULL      COMMENT '负责人',
    `phone`      VARCHAR(32)     DEFAULT NULL      COMMENT '联系电话',
    `status`     TINYINT     NOT NULL DEFAULT 1       COMMENT '状态：0停用 1正常',
    `created_by` BIGINT UNSIGNED          DEFAULT NULL      COMMENT '创建人',
    `updated_by` BIGINT UNSIGNED          DEFAULT NULL      COMMENT '更新人',
    `created_at` DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `updated_at` DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    `deleted_at` DATETIME        DEFAULT NULL COMMENT '软删除时间',
    PRIMARY KEY (`id`),
    KEY `idx_parent_id` (`parent_id`),
    KEY `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='部门表';

-- ---------------------------------------------------------------------
-- 2. 用户表 sys_user
--    敏感字段说明：
--      nickname / email / phone 存 AES-256-GCM 密文（Base64），故列放大为 varchar
--      email_hash / phone_hash 存 HMAC-SHA256 十六进制值(64位)，用于精确查询与唯一约束
-- ---------------------------------------------------------------------
DROP TABLE IF EXISTS `sys_user`;
CREATE TABLE `sys_user` (
    `id`   BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
    `username`      VARCHAR(64)     NOT NULL      COMMENT '登录账号（明文，唯一）',
    `password`      VARCHAR(100)    NOT NULL    COMMENT '密码（bcrypt 哈希）',
    `nickname`      VARCHAR(255)           DEFAULT NULL   COMMENT '昵称（AES-256-GCM 密文）',
    `avatar`        VARCHAR(255)   DEFAULT NULL   COMMENT '头像URL',
    `email`   VARCHAR(255)    DEFAULT NULL   COMMENT '邮箱（AES-256-GCM 密文）',
    `email_hash`    CHAR(64) DEFAULT NULL   COMMENT '邮箱盲索引 HMAC-SHA256',
    `phone`         VARCHAR(128)  DEFAULT NULL   COMMENT '手机号（AES-256-GCM 密文）',
    `phone_hash`    CHAR(64)    DEFAULT NULL   COMMENT '手机号盲索引 HMAC-SHA256',
    `gender`   TINYINT         NOT NULL DEFAULT 0      COMMENT '性别：0未知 1男 2女',
    `dept_id`       BIGINT UNSIGNED   DEFAULT NULL   COMMENT '所属部门ID',
    `status`   TINYINT         NOT NULL DEFAULT 1      COMMENT '状态：0停用 1正常',
  `key_version`   INT           NOT NULL DEFAULT 1      COMMENT '加密密钥版本，用于密钥轮换',
 `last_login_at` DATETIME    DEFAULT NULL   COMMENT '最后登录时间',
    `last_login_ip` VARCHAR(64)   DEFAULT NULL   COMMENT '最后登录IP',
    `remark`        VARCHAR(255)     DEFAULT NULL   COMMENT '备注',
    `created_by`    BIGINT UNSIGNED     DEFAULT NULL   COMMENT '创建人',
    `updated_by`    BIGINT UNSIGNED        DEFAULT NULL   COMMENT '更新人',
    `created_at`    DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `updated_at`    DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    `deleted_at`    DATETIME   DEFAULT NULL   COMMENT '软删除时间',
PRIMARY KEY (`id`),
  UNIQUE KEY `uk_username` (`username`),
    UNIQUE KEY `uk_email_hash` (`email_hash`),
    UNIQUE KEY `uk_phone_hash` (`phone_hash`),
    KEY `idx_dept_id` (`dept_id`),
    KEY `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='用户表';

-- ---------------------------------------------------------------------
-- 3. 角色表 sys_role
-- ---------------------------------------------------------------------
DROP TABLE IF EXISTS `sys_role`;
CREATE TABLE `sys_role` (
`id`    BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
    `name`       VARCHAR(64)     NOT NULL COMMENT '角色名称',
    `code`       VARCHAR(64)     NOT NULL                COMMENT '角色标识（唯一，如 admin）',
    `sort`    INT             NOT NULL DEFAULT 0      COMMENT '显示顺序',
    `data_scope` TINYINT NOT NULL DEFAULT 3      COMMENT '数据权限：1全部 2自定义 3本部门 4本部门及子 5仅本人',
    `status`   TINYINT         NOT NULL DEFAULT 1      COMMENT '状态：0停用 1正常',
    `remark`   VARCHAR(255)  DEFAULT NULL   COMMENT '备注',
    `created_by` BIGINT UNSIGNED          DEFAULT NULL   COMMENT '创建人',
    `updated_by` BIGINT UNSIGNED   DEFAULT NULL   COMMENT '更新人',
    `created_at` DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `updated_at` DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    `deleted_at` DATETIME          DEFAULT NULL   COMMENT '软删除时间',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_code` (`code`),
    KEY `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='角色表';

-- ---------------------------------------------------------------------
-- 4. 菜单/权限表 sys_menu（目录/菜单/按钮三合一，树形）
-- ---------------------------------------------------------------------
DROP TABLE IF EXISTS `sys_menu`;
CREATE TABLE `sys_menu` (
    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
    `parent_id`  BIGINT UNSIGNED NOT NULL DEFAULT 0      COMMENT '父菜单ID，0为顶级',
    `name`       VARCHAR(64)     NOT NULL           COMMENT '菜单名称',
    `type`       TINYINT         NOT NULL DEFAULT 2    COMMENT '类型：1目录 2菜单 3按钮',
    `path`       VARCHAR(200)    DEFAULT NULL   COMMENT '路由路径',
 `component`  VARCHAR(255)        DEFAULT NULL   COMMENT '前端组件路径',
    `perms`      VARCHAR(100)       DEFAULT NULL   COMMENT '权限标识，如 system:user:add',
  `icon`       VARCHAR(100)      DEFAULT NULL   COMMENT '图标',
    `sort`       INT       NOT NULL DEFAULT 0      COMMENT '显示顺序',
  `visible`    TINYINT         NOT NULL DEFAULT 1      COMMENT '是否显示：0隐藏 1显示',
    `status`     TINYINT     NOT NULL DEFAULT 1      COMMENT '状态：0停用 1正常',
    `is_frame`   TINYINT  NOT NULL DEFAULT 0      COMMENT '是否外链：0否 1是',
    `created_by` BIGINT UNSIGNED        DEFAULT NULL   COMMENT '创建人',
    `updated_by` BIGINT UNSIGNED DEFAULT NULL   COMMENT '更新人',
    `created_at` DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `updated_at` DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    PRIMARY KEY (`id`),
    KEY `idx_parent_id` (`parent_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='菜单权限表';

-- ---------------------------------------------------------------------
-- 5. 用户-角色关联表 sys_user_role（多对多）
-- ---------------------------------------------------------------------
DROP TABLE IF EXISTS `sys_user_role`;
CREATE TABLE `sys_user_role` (
    `user_id` BIGINT UNSIGNED NOT NULL COMMENT '用户ID',
    `role_id` BIGINT UNSIGNED NOT NULL COMMENT '角色ID',
    PRIMARY KEY (`user_id`, `role_id`),
    KEY `idx_role_id` (`role_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='用户角色关联表';

-- ---------------------------------------------------------------------
-- 6. 角色-菜单关联表 sys_role_menu（多对多）
-- ---------------------------------------------------------------------
DROP TABLE IF EXISTS `sys_role_menu`;
CREATE TABLE `sys_role_menu` (
    `role_id` BIGINT UNSIGNED NOT NULL COMMENT '角色ID',
    `menu_id` BIGINT UNSIGNED NOT NULL COMMENT '菜单ID',
  PRIMARY KEY (`role_id`, `menu_id`),
    KEY `idx_menu_id` (`menu_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='角色菜单关联表';

-- ---------------------------------------------------------------------
-- 7. 角色-部门关联表 sys_role_dept（数据权限自定义范围）
-- ---------------------------------------------------------------------
DROP TABLE IF EXISTS `sys_role_dept`;
CREATE TABLE `sys_role_dept` (
    `role_id` BIGINT UNSIGNED NOT NULL COMMENT '角色ID',
    `dept_id` BIGINT UNSIGNED NOT NULL COMMENT '部门ID',
    PRIMARY KEY (`role_id`, `dept_id`),
    KEY `idx_dept_id` (`dept_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='角色部门关联表';

-- ---------------------------------------------------------------------
-- 8. 字典类型表 sys_dict_type
-- ---------------------------------------------------------------------
DROP TABLE IF EXISTS `sys_dict_type`;
CREATE TABLE `sys_dict_type` (
    `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
    `name`       VARCHAR(64)     NOT NULL           COMMENT '字典名称',
    `type` VARCHAR(64)     NOT NULL       COMMENT '字典类型（唯一）',
    `status`     TINYINT         NOT NULL DEFAULT 1      COMMENT '状态：0停用 1正常',
    `remark`     VARCHAR(255)   DEFAULT NULL   COMMENT '备注',
    `created_at` DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `updated_at` DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_type` (`type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='字典类型表';

-- ---------------------------------------------------------------------
-- 9. 字典数据表 sys_dict_data
-- ---------------------------------------------------------------------
DROP TABLE IF EXISTS `sys_dict_data`;
CREATE TABLE `sys_dict_data` (
    `id`           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
    `dict_type_id` BIGINT UNSIGNED NOT NULL         COMMENT '字典类型ID',
    `dict_type`    VARCHAR(64)     NOT NULL              COMMENT '字典类型（冗余，便于查询）',
    `label`        VARCHAR(100)    NOT NULL  COMMENT '字典标签',
    `value`  VARCHAR(100)    NOT NULL  COMMENT '字典键值',
    `sort`  INT             NOT NULL DEFAULT 0      COMMENT '显示顺序',
    `is_default`   TINYINT         NOT NULL DEFAULT 0      COMMENT '是否默认：0否 1是',
    `status`       TINYINT   NOT NULL DEFAULT 1      COMMENT '状态：0停用 1正常',
    `remark`       VARCHAR(255) DEFAULT NULL   COMMENT '备注',
    `created_at`   DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `updated_at`   DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    PRIMARY KEY (`id`),
    KEY `idx_dict_type` (`dict_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='字典数据表';

-- ---------------------------------------------------------------------
-- 10. 操作日志表 sys_oper_log
-- ---------------------------------------------------------------------
DROP TABLE IF EXISTS `sys_oper_log`;
CREATE TABLE `sys_oper_log` (
    `id`       BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
    `title`         VARCHAR(64)      DEFAULT NULL   COMMENT '操作模块',
    `business_type` TINYINT     NOT NULL DEFAULT 0      COMMENT '业务类型：0其他 1新增 2修改 3删除 4查询',
    `method`        VARCHAR(100)        DEFAULT NULL   COMMENT '请求方法(HTTP Method)',
    `request_url`   VARCHAR(255)       DEFAULT NULL   COMMENT '请求URL',
    `oper_user_id`  BIGINT UNSIGNED        DEFAULT NULL   COMMENT '操作人ID',
    `oper_name`     VARCHAR(64)      DEFAULT NULL   COMMENT '操作人账号',
    `oper_ip`     VARCHAR(64) DEFAULT NULL   COMMENT '操作IP',
    `request_param` TEXT               COMMENT '请求参数（敏感信息需脱敏）',
    `json_result`   TEXT        COMMENT '返回结果',
    `status`      TINYINT      NOT NULL DEFAULT 1      COMMENT '状态：0异常 1正常',
 `error_msg`     VARCHAR(2000)    DEFAULT NULL   COMMENT '错误消息',
    `cost_time`     INT    NOT NULL DEFAULT 0      COMMENT '耗时（毫秒）',
    `created_at`    DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '操作时间',
    PRIMARY KEY (`id`),
KEY `idx_oper_user_id` (`oper_user_id`),
    KEY `idx_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='操作日志表';

-- ---------------------------------------------------------------------
-- 11. 登录日志表 sys_login_log
-- ---------------------------------------------------------------------
DROP TABLE IF EXISTS `sys_login_log`;
CREATE TABLE `sys_login_log` (
    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
    `username`   VARCHAR(64)  DEFAULT NULL   COMMENT '登录账号',
  `ipaddr`     VARCHAR(64)              DEFAULT NULL   COMMENT '登录IP',
    `location`   VARCHAR(128)             DEFAULT NULL   COMMENT '登录地点',
    `browser`    VARCHAR(256)             DEFAULT NULL   COMMENT '浏览器（存原始 User-Agent，故需足够长）',
  `os`         VARCHAR(64)              DEFAULT NULL   COMMENT '操作系统',
 `status`     TINYINT         NOT NULL DEFAULT 1      COMMENT '登录状态：0失败 1成功',
    `msg`        VARCHAR(255)     DEFAULT NULL   COMMENT '提示消息',
    `login_time` DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '登录时间',
    PRIMARY KEY (`id`),
    KEY `idx_username` (`username`),
    KEY `idx_login_time` (`login_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='登录日志表';

-- ---------------------------------------------------------------------
-- Casbin 策略表 casbin_rule（若使用 Casbin gorm-adapter，通常由程序自动建表）
-- 此处给出参考结构，可按需保留
-- ---------------------------------------------------------------------
DROP TABLE IF EXISTS `casbin_rule`;
CREATE TABLE `casbin_rule` (
 `id`    BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
    `ptype` VARCHAR(100) DEFAULT NULL   COMMENT '策略类型 p 或 g',
    `v0`    VARCHAR(100)       DEFAULT NULL,
    `v1`    VARCHAR(100)             DEFAULT NULL,
    `v2`    VARCHAR(100)    DEFAULT NULL,
    `v3`    VARCHAR(100)             DEFAULT NULL,
    `v4`    VARCHAR(100)      DEFAULT NULL,
    `v5`    VARCHAR(100)       DEFAULT NULL,
    PRIMARY KEY (`id`),
    KEY `idx_ptype` (`ptype`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='Casbin 策略表';


-- =====================================================================
-- 初始化数据 (seed data)
-- 注意：sys_user 的 nickname/email/phone 为 AES 密文、_hash 为 HMAC 值，
--       无法在纯 SQL 中直接写明文，需由后端程序加密后写入或更新。
--    下方 admin 用户仅初始化必要字段（用户名+密码），
--       敏感字段留 NULL，待程序补写。
--       密码 '123456' 的 bcrypt 值仅为示例，上线务必替换。
-- =====================================================================

-- 1) 部门
INSERT INTO `sys_dept` (`id`, `parent_id`, `ancestors`, `name`, `sort`, `leader`, `status`) VALUES
(1, 0, '0',   '总公司', 0, 'admin', 1),
(2, 1, '0,1', '研发部', 1, NULL,    1),
(3, 1, '0,1', '运营部', 2, NULL,    1);

-- 2) 角色
INSERT INTO `sys_role` (`id`, `name`, `code`, `sort`, `data_scope`, `status`, `remark`) VALUES
(1, '超级管理员', 'admin',  1, 1, 1, '拥有全部权限，鉴权中间件直接放行'),
(2, '普通角色',   'common', 2, 3, 1, '示例角色，仅本部门数据权限');

-- 3) 管理员用户（敏感字段待程序加密写入）
--    password 为 '123456' 的 bcrypt 示例哈希，请上线前重置
INSERT INTO `sys_user`
(`id`, `username`, `password`, `nickname`, `email`, `email_hash`, `phone`, `phone_hash`, `gender`, `dept_id`, `status`, `key_version`, `remark`) VALUES
(1, 'admin', '$2a$10$pSOLiM/6NBmgjllsX5NJje6Tyu39rKIsvS4/lFn3fbHbSZO.YLh.W',
 NULL, NULL, NULL, NULL, NULL, 1, 1, 1, 1, '初始超级管理员，敏感字段由程序补写');

-- 4) 用户-角色
INSERT INTO `sys_user_role` (`user_id`, `role_id`) VALUES (1, 1);

-- 5) 菜单（目录/菜单/按钮示例：系统管理下的用户管理）
INSERT INTO `sys_menu` (`id`, `parent_id`, `name`, `type`, `path`, `component`, `perms`, `icon`, `sort`, `visible`, `status`) VALUES
(1,   0, '系统管理', 1, '/system',      NULL,    NULL,      'setting', 1, 1, 1),
(100, 1, '用户管理', 2, 'user',         'system/user/index',       'system:user:list',  'user',    1, 1, 1),
(101, 1, '角色管理', 2, 'role',         'system/role/index',       'system:role:list',  'team',    2, 1, 1),
(102, 1, '菜单管理', 2, 'menu',         'system/menu/index',       'system:menu:list',  'menu',    3, 1, 1),
(103, 1, '部门管理', 2, 'dept',         'system/dept/index',   'system:dept:list',  'cluster', 4, 1, 1),
(104, 1, '字典管理', 2, 'dict',         'system/dict/index',    'system:dict:list',  'book',    5, 1, 1),
(105, 1, '操作日志', 2, 'operlog',      'system/log/operlog',  'system:operlog:list','file-text',6,1, 1),
(106, 1, '登录日志', 2, 'loginlog',     'system/log/loginlog',     'system:loginlog:list','login', 7, 1, 1),
-- 用户管理下的按钮权限点
(1001, 100, '用户查询', 3, NULL, NULL, 'system:user:query',  NULL, 1, 1, 1),
(1002, 100, '用户新增', 3, NULL, NULL, 'system:user:add',    NULL, 2, 1, 1),
(1003, 100, '用户修改', 3, NULL, NULL, 'system:user:edit',   NULL, 3, 1, 1),
(1004, 100, '用户删除', 3, NULL, NULL, 'system:user:remove', NULL, 4, 1, 1),
(1005, 100, '重置密码', 3, NULL, NULL, 'system:user:resetPwd',NULL,5, 1, 1),
(1006, 100, '分配角色', 3, NULL, NULL, 'system:user:assignRole',NULL,6,1, 1),
(1007, 100, '用户导出', 3, NULL, NULL, 'system:user:export',   NULL, 7, 1, 1),
-- 角色管理下的按钮权限点
(1101, 101, '角色查询',   3, NULL, NULL, 'system:role:query',      NULL, 1, 1, 1),
(1102, 101, '角色新增',   3, NULL, NULL, 'system:role:add',        NULL, 2, 1, 1),
(1103, 101, '角色修改',   3, NULL, NULL, 'system:role:edit',       NULL, 3, 1, 1),
(1104, 101, '角色删除',   3, NULL, NULL, 'system:role:remove',     NULL, 4, 1, 1),
(1105, 101, '分配权限',   3, NULL, NULL, 'system:role:assignMenu', NULL, 5, 1, 1),
(1106, 101, '设置数据权限',3, NULL, NULL, 'system:role:dataScope',  NULL, 6, 1, 1),
-- 菜单管理下的按钮权限点
(1201, 102, '菜单查询', 3, NULL, NULL, 'system:menu:query',  NULL, 1, 1, 1),
(1202, 102, '菜单新增', 3, NULL, NULL, 'system:menu:add',    NULL, 2, 1, 1),
(1203, 102, '菜单修改', 3, NULL, NULL, 'system:menu:edit',   NULL, 3, 1, 1),
(1204, 102, '菜单删除', 3, NULL, NULL, 'system:menu:remove', NULL, 4, 1, 1),
-- 部门管理下的按钮权限点
(1301, 103, '部门查询', 3, NULL, NULL, 'system:dept:query',  NULL, 1, 1, 1),
(1302, 103, '部门新增', 3, NULL, NULL, 'system:dept:add',    NULL, 2, 1, 1),
(1303, 103, '部门修改', 3, NULL, NULL, 'system:dept:edit',   NULL, 3, 1, 1),
(1304, 103, '部门删除', 3, NULL, NULL, 'system:dept:remove', NULL, 4, 1, 1),
-- 字典管理下的按钮权限点
(1401, 104, '字典查询', 3, NULL, NULL, 'system:dict:query',  NULL, 1, 1, 1),
(1402, 104, '字典新增', 3, NULL, NULL, 'system:dict:add',    NULL, 2, 1, 1),
(1403, 104, '字典修改', 3, NULL, NULL, 'system:dict:edit',   NULL, 3, 1, 1),
(1404, 104, '字典删除', 3, NULL, NULL, 'system:dict:remove', NULL, 4, 1, 1),
-- 操作日志下的按钮权限点
(1501, 105, '日志删除', 3, NULL, NULL, 'system:operlog:remove', NULL, 1, 1, 1),
(1502, 105, '日志导出', 3, NULL, NULL, 'system:operlog:export', NULL, 2, 1, 1),
-- 登录日志下的按钮权限点
(1601, 106, '日志删除', 3, NULL, NULL, 'system:loginlog:remove', NULL, 1, 1, 1),
(1602, 106, '日志导出', 3, NULL, NULL, 'system:loginlog:export', NULL, 2, 1, 1);

-- 6) 超级管理员角色拥有以上全部菜单权限
--    admin 角色在鉴权中间件中直接放行，这里显式授权是为了让权限树能正确回显，
--    也便于以本角色为模板复制出受限角色。
INSERT INTO `sys_role_menu` (`role_id`, `menu_id`) VALUES
(1,1),(1,100),(1,101),(1,102),(1,103),(1,104),(1,105),(1,106),
(1,1001),(1,1002),(1,1003),(1,1004),(1,1005),(1,1006),(1,1007),
(1,1101),(1,1102),(1,1103),(1,1104),(1,1105),(1,1106),
(1,1201),(1,1202),(1,1203),(1,1204),
(1,1301),(1,1302),(1,1303),(1,1304),
(1,1401),(1,1402),(1,1403),(1,1404),
(1,1501),(1,1502),
(1,1601),(1,1602);

-- 7) 字典示例（用户性别、启用状态）
INSERT INTO `sys_dict_type` (`id`, `name`, `type`, `status`, `remark`) VALUES
(1, '用户性别', 'sys_user_gender', 1, '用户性别列表'),
(2, '系统状态', 'sys_normal_disable', 1, '启用停用状态列表');

INSERT INTO `sys_dict_data` (`dict_type_id`, `dict_type`, `label`, `value`, `sort`, `is_default`, `status`) VALUES
(1, 'sys_user_gender',    '未知', '0', 1, 0, 1),
(1, 'sys_user_gender',    '男',   '1', 2, 1, 1),
(1, 'sys_user_gender',    '女',   '2', 3, 0, 1),
(2, 'sys_normal_disable', '正常', '1', 1, 1, 1),
(2, 'sys_normal_disable', '停用', '0', 2, 0, 1);

SET FOREIGN_KEY_CHECKS = 1;

-- =====================================================================
-- 脚本结束
-- 后续操作提示：
--   1. admin 用户的 nickname/email/phone 请通过后端接口或初始化程序，
--      用 AES-256-GCM 加密写入密文，并同步计算 email_hash/phone_hash。
--   2. 生产环境务必重置 admin 初始密码，并妥善保管 AES/HMAC 密钥。
-- =====================================================================
