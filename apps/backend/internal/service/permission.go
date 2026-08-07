package service

import (
	"context"
	"fmt"
	"sync"

	"github.com/casbin/casbin/v2"

	"github.com/fangzhengcn/workbackend/apps/backend/internal/repository"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/logger"
)

// PermissionService 维护 Casbin 策略，供接口级鉴权中间件使用。
//
// 策略语义：p = 角色标识(role.code), 权限标识(perms), *
// 即「某角色拥有某个权限点」。中间件把请求路由映射为所需的 perms，
// 再逐个角色 Enforce。
//
// 为什么不直接用「请求路径」作为 obj：
// 数据库里的权限载体是 sys_menu.perms（如 system:user:add），
// 路径与 perms 的对应关系由路由注册时显式声明，比在库里再存一份路径更不易失配。
type PermissionService struct {
	enforcer *casbin.Enforcer
	roles    *repository.RoleRepository
	// mu 保护策略重建：LoadPolicy 期间若有并发 Enforce 可能读到半成品。
	mu sync.RWMutex
}

func NewPermissionService(enforcer *casbin.Enforcer, roles *repository.RoleRepository) *PermissionService {
	return &PermissionService{enforcer: enforcer, roles: roles}
}

// Enforce 判断角色集合中是否有任一角色拥有指定权限点。
func (s *PermissionService) Enforce(roleCodes []string, perm string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, code := range roleCodes {
		ok, err := s.enforcer.Enforce(code, perm, "*")
		if err != nil {
			return false, fmt.Errorf("权限校验失败: %w", err)
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

// ReloadPolicies 依据 sys_role_menu 全量重建 Casbin 策略。
//
// 角色权限变更后必须调用，否则改动不会立即生效
// （设计文档「技术难点提示 4」）。
func (s *PermissionService) ReloadPolicies(ctx context.Context) error {
	rolePerms, err := s.roles.FindRoleMenuPerms(ctx)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 全量重建而非增量 diff：策略量级小（角色数 × 权限点数），
	// 全量替换逻辑简单且不会残留脏策略。
	s.enforcer.ClearPolicy()

	// 重建期间必须关掉 autosave：ClearPolicy 只清内存，不删 casbin_rule 表，
	// 而 autosave 会让每条 AddPolicy 立即单条 INSERT，撞上表里已有记录的
	// 唯一索引直接报 1062（重启后端时必然触发）。
	// 末尾的 SavePolicy() 会整表覆盖，这些单条写入本就是多余的。
	s.enforcer.EnableAutoSave(false)
	defer s.enforcer.EnableAutoSave(true)

	total := 0
	for code, perms := range rolePerms {
		for _, perm := range perms {
			if _, err := s.enforcer.AddPolicy(code, perm, "*"); err != nil {
				return fmt.Errorf("添加权限策略失败 (%s, %s): %w", code, perm, err)
			}
			total++
		}
	}

	// 持久化到 casbin_rule 表（整表覆盖），重启后可由 adapter 直接加载。
	if err := s.enforcer.SavePolicy(); err != nil {
		return fmt.Errorf("保存权限策略失败: %w", err)
	}

	logger.Infof("Casbin 策略已重建：%d 个角色，%d 条策略", len(rolePerms), total)
	return nil
}
