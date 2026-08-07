package service

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/fangzhengcn/workbackend/apps/backend/internal/dto"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/model"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/repository"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/vo"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/errs"
)

// rootAncestors 是顶级部门的 ancestors 值。
//
// 取 "0" 而非空串，与建表脚本的种子数据保持一致（总公司的 ancestors 就是 "0"）。
// 这样每一级的 ancestors 都以 0 开头，FIND_IN_SET 的行为在各层级上统一。
const rootAncestors = "0"

// DeptService 负责部门树的增删改查。
//
// 核心不变量：ancestors 必须始终与 parent_id 链一致。
// DataScopeDeptTree 的子树过滤完全依赖 ancestors（FindSubtreeIDs 用
// FIND_IN_SET 查询），一旦失配，数据权限判断就会静默出错——
// 用户看到不该看的数据，或看不到本该看的数据，且从界面上完全察觉不到。
type DeptService struct {
	depts *repository.DeptRepository
}

func NewDeptService(depts *repository.DeptRepository) *DeptService {
	return &DeptService{depts: depts}
}

// Tree 返回部门树。
func (s *DeptService) Tree(ctx context.Context) ([]*vo.DeptNode, error) {
	depts, err := s.depts.FindAll(ctx)
	if err != nil {
		return nil, errs.Internal("查询部门失败").WithCause(err)
	}
	return vo.BuildDeptTree(depts), nil
}

// Create 新增部门。
func (s *DeptService) Create(ctx context.Context, operatorID uint64, req *dto.CreateDeptRequest) error {
	ancestors, err := s.ancestorsFor(ctx, req.ParentID)
	if err != nil {
		return err
	}
	if err := s.checkNameUnique(ctx, req.ParentID, req.Name, 0); err != nil {
		return err
	}

	dept := &model.Dept{
		ParentID:  req.ParentID,
		Ancestors: ancestors,
		Name:      req.Name,
		Sort:      req.Sort,
		Leader:    req.Leader,
		Phone:     req.Phone,
		Status:    defaultInt8(req.Status, model.StatusEnabled),
	}
	dept.CreatedBy = &operatorID
	dept.UpdatedBy = &operatorID

	if err := s.depts.Create(ctx, dept); err != nil {
		return errs.Internal("创建部门失败").WithCause(err)
	}
	return nil
}

// Update 修改部门。
//
// 若变更了上级部门，自身与全部后代的 ancestors 都要重算——
// 只改自己会让后代的 ancestors 仍指向旧路径，子树过滤从此失准。
func (s *DeptService) Update(ctx context.Context, operatorID, id uint64, req *dto.UpdateDeptRequest) error {
	dept, err := s.depts.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return errs.ErrDeptNotFound
		}
		return err
	}

	parentChanged := req.ParentID != nil && *req.ParentID != dept.ParentID
	newAncestors := dept.Ancestors

	if parentChanged {
		if err := s.validateNotSelfOrDescendant(ctx, id, *req.ParentID); err != nil {
			return err
		}
		newAncestors, err = s.ancestorsFor(ctx, *req.ParentID)
		if err != nil {
			return err
		}
		dept.ParentID = *req.ParentID
		dept.Ancestors = newAncestors
	}

	// 同级下的重名校验要按改动后的父级来判断。
	if req.Name != nil {
		if err := s.checkNameUnique(ctx, dept.ParentID, *req.Name, id); err != nil {
			return err
		}
		dept.Name = *req.Name
	}
	if req.Sort != nil {
		dept.Sort = *req.Sort
	}
	if req.Leader != nil {
		dept.Leader = *req.Leader
	}
	if req.Phone != nil {
		dept.Phone = *req.Phone
	}
	if req.Status != nil {
		dept.Status = *req.Status
	}
	dept.UpdatedBy = &operatorID

	// 自身与后代的 ancestors 必须在同一事务里更新，
	// 中途失败会留下「父子 ancestors 不一致」的坏状态。
	return s.depts.DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.WithContext(ctx).Save(dept).Error; err != nil {
			return errs.Internal("保存部门失败").WithCause(err)
		}
		if parentChanged {
			if err := s.rebuildDescendantAncestors(ctx, tx, dept); err != nil {
				return err
			}
		}
		return nil
	})
}

// Delete 删除部门。
func (s *DeptService) Delete(ctx context.Context, id uint64) error {
	if _, err := s.depts.FindByID(ctx, id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return errs.ErrDeptNotFound
		}
		return err
	}

	hasChildren, err := s.depts.HasChildren(ctx, id)
	if err != nil {
		return errs.Internal("查询子部门失败").WithCause(err)
	}
	if hasChildren {
		return errs.BadRequest("该部门下还有子部门，请先删除子部门")
	}

	// 部门下还有用户时不允许删除：这些用户的 dept_id 会变成悬空引用，
	// 而数据权限按部门过滤，悬空后这些用户可能一条数据都看不到。
	count, err := s.depts.CountUsers(ctx, id)
	if err != nil {
		return errs.Internal("统计部门用户数失败").WithCause(err)
	}
	if count > 0 {
		return errs.BadRequest("该部门下还有用户，请先转移用户后再删除")
	}

	if err := s.depts.Delete(ctx, id); err != nil {
		return errs.Internal("删除部门失败").WithCause(err)
	}
	return nil
}

// ancestorsFor 计算挂在 parentID 之下时应写入的 ancestors。
func (s *DeptService) ancestorsFor(ctx context.Context, parentID uint64) (string, error) {
	if parentID == model.RootID {
		return rootAncestors, nil
	}

	parent, err := s.depts.FindByID(ctx, parentID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return "", errs.BadRequest("上级部门不存在")
		}
		return "", err
	}
	return parent.ChildAncestors(), nil
}

// rebuildDescendantAncestors 递归重算全部后代的 ancestors。
//
// 用一次全量查询在内存里按 parent_id 分组后递归，而不是逐层查库：
// 部门总量很小，一次 SQL 远优于 N 次（设计文档「技术难点提示 3」）。
func (s *DeptService) rebuildDescendantAncestors(ctx context.Context, tx *gorm.DB, moved *model.Dept) error {
	var all []*model.Dept
	if err := tx.WithContext(ctx).Find(&all).Error; err != nil {
		return errs.Internal("查询部门失败").WithCause(err)
	}

	childrenOf := make(map[uint64][]*model.Dept, len(all))
	for _, dept := range all {
		childrenOf[dept.ParentID] = append(childrenOf[dept.ParentID], dept)
	}

	// visited 兜住脏数据构成的环，避免无限递归打挂请求。
	visited := make(map[uint64]bool, len(all))
	var walk func(parent *model.Dept) error
	walk = func(parent *model.Dept) error {
		if visited[parent.ID] {
			return nil
		}
		visited[parent.ID] = true

		childAncestors := parent.ChildAncestors()
		for _, child := range childrenOf[parent.ID] {
			// 只更新 ancestors 一列：用 Save 会连带写回其他字段，
			// 而这些实体是刚查出来的，可能覆盖并发事务的改动。
			err := tx.WithContext(ctx).
				Model(&model.Dept{}).
				Where("id = ?", child.ID).
				Update("ancestors", childAncestors).Error
			if err != nil {
				return errs.Internal("更新子部门层级失败").WithCause(err)
			}
			child.Ancestors = childAncestors
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(moved)
}

// validateNotSelfOrDescendant 阻止把部门挂到自己或自己的后代之下。
//
// 与菜单同理：这样做会让整棵子树脱离根节点，从部门树上凭空消失，
// 且无法再通过界面移回来。
func (s *DeptService) validateNotSelfOrDescendant(ctx context.Context, id, newParentID uint64) error {
	if id == newParentID {
		return errs.BadRequest("上级部门不能是自己")
	}
	if newParentID == model.RootID {
		return nil
	}

	all, err := s.depts.FindAll(ctx)
	if err != nil {
		return errs.Internal("查询部门失败").WithCause(err)
	}
	parentOf := make(map[uint64]uint64, len(all))
	for _, dept := range all {
		parentOf[dept.ID] = dept.ParentID
	}

	// 从目标父级向上回溯，途中遇到自己即说明目标是自己的后代。
	// steps 上限兜住库里已有的脏环。
	for cursor, steps := newParentID, 0; cursor != model.RootID && steps <= len(all); steps++ {
		if cursor == id {
			return errs.BadRequest("上级部门不能是自己的子部门")
		}
		next, ok := parentOf[cursor]
		if !ok {
			break
		}
		cursor = next
	}
	return nil
}

// checkNameUnique 校验同一父级下的部门不重名。
//
// 不同父级下允许同名（如两个分公司各有「研发部」），
// 但同级重名会让用户在部门树与下拉框里完全无法区分。
func (s *DeptService) checkNameUnique(ctx context.Context, parentID uint64, name string, excludeID uint64) error {
	all, err := s.depts.FindAll(ctx)
	if err != nil {
		return errs.Internal("查询部门失败").WithCause(err)
	}
	for _, dept := range all {
		if dept.ParentID == parentID && dept.Name == name && dept.ID != excludeID {
			return errs.BadRequest("同级下已存在同名部门「" + name + "」")
		}
	}
	return nil
}
