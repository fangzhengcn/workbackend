package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/fangzhengcn/workbackend/apps/backend/internal/model"
)

// DeptRepository 负责 sys_dept 的数据访问。
type DeptRepository struct {
	db *gorm.DB
}

func NewDeptRepository(db *gorm.DB) *DeptRepository {
	return &DeptRepository{db: db}
}

func (r *DeptRepository) FindByID(ctx context.Context, id uint64) (*model.Dept, error) {
	var dept model.Dept
	err := r.db.WithContext(ctx).First(&dept, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询部门失败: %w", err)
	}
	return &dept, nil
}

// FindAll 查出全部部门，供内存建树。
func (r *DeptRepository) FindAll(ctx context.Context) ([]*model.Dept, error) {
	var depts []*model.Dept
	err := r.db.WithContext(ctx).
		Order("parent_id ASC, sort ASC, id ASC").
		Find(&depts).Error
	if err != nil {
		return nil, fmt.Errorf("查询部门列表失败: %w", err)
	}
	return depts, nil
}

// FindSubtreeIDs 查询指定部门及其所有子部门的 ID。
//
// 借助 ancestors 列做一次 LIKE 即可拿到整棵子树，
// 无需递归查库（对应 DataScopeDeptTree）。
func (r *DeptRepository) FindSubtreeIDs(ctx context.Context, deptID uint64) ([]uint64, error) {
	var ids []uint64
	err := r.db.WithContext(ctx).
		Model(&model.Dept{}).
		Where("id = ? OR FIND_IN_SET(?, ancestors)", deptID, deptID).
		Pluck("id", &ids).Error
	if err != nil {
		return nil, fmt.Errorf("查询子部门失败: %w", err)
	}
	return ids, nil
}

// HasChildren 判断是否存在子部门，删除前校验。
func (r *DeptRepository) HasChildren(ctx context.Context, id uint64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Dept{}).Where("parent_id = ?", id).Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("查询子部门失败: %w", err)
	}
	return count > 0, nil
}

// CountUsers 统计部门下的用户数，删除前校验。
func (r *DeptRepository) CountUsers(ctx context.Context, id uint64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.User{}).Where("dept_id = ?", id).Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("统计部门用户数失败: %w", err)
	}
	return count, nil
}

func (r *DeptRepository) Create(ctx context.Context, dept *model.Dept) error {
	if err := r.db.WithContext(ctx).Create(dept).Error; err != nil {
		return fmt.Errorf("创建部门失败: %w", err)
	}
	return nil
}

func (r *DeptRepository) Save(ctx context.Context, dept *model.Dept) error {
	if err := r.db.WithContext(ctx).Save(dept).Error; err != nil {
		return fmt.Errorf("保存部门失败: %w", err)
	}
	return nil
}

func (r *DeptRepository) Delete(ctx context.Context, id uint64) error {
	if err := r.db.WithContext(ctx).Delete(&model.Dept{}, id).Error; err != nil {
		return fmt.Errorf("删除部门失败: %w", err)
	}
	return nil
}

// DB 暴露底层句柄，供 Service 开启事务（如移动部门时批量重算 ancestors）。
func (r *DeptRepository) DB() *gorm.DB { return r.db }
