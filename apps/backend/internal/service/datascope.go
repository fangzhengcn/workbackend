package service

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/fangzhengcn/workbackend/apps/backend/internal/model"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/repository"
)

// DataScopeService 负责生成数据级权限的查询过滤条件。
//
// 对应设计文档「技术难点提示 2」：用 GORM Scopes 封装 data_scope 过滤，
// 避免每个查询都手写一遍。
type DataScopeService struct {
	roles *repository.RoleRepository
	depts *repository.DeptRepository
}

func NewDataScopeService(roles *repository.RoleRepository, depts *repository.DeptRepository) *DataScopeService {
	return &DataScopeService{roles: roles, depts: depts}
}

// Scope 依据用户的角色数据范围，返回可叠加到查询上的 GORM Scope。
//
// 多角色取「最宽松」的范围：一个用户若同时是「全部数据」和「仅本人」，
// 应按前者放行，否则叠加角色反而缩小权限，与 RBAC 直觉相悖。
//
// tableName 为待过滤的主表名，用于给字段加前缀避免 JOIN 时列名歧义。
func (s *DataScopeService) Scope(ctx context.Context, user *model.User, tableName string) (func(*gorm.DB) *gorm.DB, error) {
	noop := func(db *gorm.DB) *gorm.DB { return db }

	// 超级管理员不受数据范围限制。
	if user.IsSuperAdmin() {
		return noop, nil
	}
	if len(user.Roles) == 0 {
		// 无任何角色：不应看到任何数据，而不是看到全部。
		return func(db *gorm.DB) *gorm.DB {
			return db.Where("1 = 0")
		}, nil
	}

	widest := widestScope(user.Roles)
	deptColumn := fmt.Sprintf("%s.dept_id", tableName)
	createdByColumn := fmt.Sprintf("%s.created_by", tableName)

	switch widest {
	case model.DataScopeAll:
		return noop, nil

	case model.DataScopeCustom:
		deptIDs, err := s.customDeptIDs(ctx, user.Roles)
		if err != nil {
			return nil, err
		}
		if len(deptIDs) == 0 {
			return func(db *gorm.DB) *gorm.DB { return db.Where("1 = 0") }, nil
		}
		return func(db *gorm.DB) *gorm.DB {
			return db.Where(deptColumn+" IN ?", deptIDs)
		}, nil

	case model.DataScopeDept:
		deptID := user.DeptIDValue()
		if deptID == 0 {
			return func(db *gorm.DB) *gorm.DB { return db.Where("1 = 0") }, nil
		}
		return func(db *gorm.DB) *gorm.DB {
			return db.Where(deptColumn+" = ?", deptID)
		}, nil

	case model.DataScopeDeptTree:
		deptID := user.DeptIDValue()
		if deptID == 0 {
			return func(db *gorm.DB) *gorm.DB { return db.Where("1 = 0") }, nil
		}
		// 借助 ancestors 列一次查出整棵子树，避免递归查库。
		deptIDs, err := s.depts.FindSubtreeIDs(ctx, deptID)
		if err != nil {
			return nil, err
		}
		return func(db *gorm.DB) *gorm.DB {
			return db.Where(deptColumn+" IN ?", deptIDs)
		}, nil

	case model.DataScopeSelf:
		return func(db *gorm.DB) *gorm.DB {
			// 只看自己创建的数据；用户表本身还应包含自己这一行。
			return db.Where(createdByColumn+" = ? OR "+tableName+".id = ?", user.ID, user.ID)
		}, nil

	default:
		// 出现未知取值时按最严格处理，避免脏数据导致越权。
		return func(db *gorm.DB) *gorm.DB { return db.Where("1 = 0") }, nil
	}
}

// widestScope 返回角色集合中最宽松的数据范围。
// 数值越小范围越大（1 全部 → 5 仅本人）。
func widestScope(roles []model.Role) int8 {
	widest := model.DataScopeSelf
	for _, role := range roles {
		if role.Status != model.StatusEnabled {
			continue
		}
		if role.DataScope < widest {
			widest = role.DataScope
		}
	}
	return widest
}

// customDeptIDs 汇总所有自定义范围角色关联的部门，并去重。
func (s *DataScopeService) customDeptIDs(ctx context.Context, roles []model.Role) ([]uint64, error) {
	seen := make(map[uint64]struct{})
	var deptIDs []uint64
	for _, role := range roles {
		if role.Status != model.StatusEnabled || role.DataScope != model.DataScopeCustom {
			continue
		}
		ids, err := s.roles.FindDeptIDs(ctx, role.ID)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			deptIDs = append(deptIDs, id)
		}
	}
	return deptIDs, nil
}
