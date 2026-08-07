package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/fangzhengcn/workbackend/apps/backend/internal/dto"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/model"
)

// DictRepository 负责 sys_dict_type 与 sys_dict_data 的数据访问。
//
// 两张表放在一个 Repository 里：字典数据不能脱离字典类型独立存在，
// 且改类型的 type 值时必须同步更新数据表的冗余列，拆开会让这个事务跨仓库。
type DictRepository struct {
	db *gorm.DB
}

func NewDictRepository(db *gorm.DB) *DictRepository {
	return &DictRepository{db: db}
}

// ---- 字典类型 ----

func (r *DictRepository) FindTypeByID(ctx context.Context, id uint64) (*model.DictType, error) {
	var dictType model.DictType
	err := r.db.WithContext(ctx).First(&dictType, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询字典类型失败: %w", err)
	}
	return &dictType, nil
}

// PageTypes 分页查询字典类型。
func (r *DictRepository) PageTypes(ctx context.Context, query *dto.DictTypeQuery) ([]*model.DictType, int64, error) {
	db := r.db.WithContext(ctx).Model(&model.DictType{})

	if query.Name != "" {
		db = db.Where("name LIKE ?", "%"+query.Name+"%")
	}
	if query.Type != "" {
		db = db.Where("type LIKE ?", "%"+query.Type+"%")
	}
	if query.Status != nil {
		db = db.Where("status = ?", *query.Status)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计字典类型失败: %w", err)
	}

	var types []*model.DictType
	err := db.Order("id ASC").
		Offset(query.Offset()).
		Limit(query.Limit()).
		Find(&types).Error
	if err != nil {
		return nil, 0, fmt.Errorf("查询字典类型失败: %w", err)
	}
	return types, total, nil
}

// ExistsType 判断字典类型标识是否已存在。
func (r *DictRepository) ExistsType(ctx context.Context, dictType string, excludeID uint64) (bool, error) {
	db := r.db.WithContext(ctx).Model(&model.DictType{}).Where("type = ?", dictType)
	if excludeID > 0 {
		db = db.Where("id <> ?", excludeID)
	}

	var count int64
	if err := db.Count(&count).Error; err != nil {
		return false, fmt.Errorf("校验字典类型失败: %w", err)
	}
	return count > 0, nil
}

func (r *DictRepository) CreateType(ctx context.Context, dictType *model.DictType) error {
	if err := r.db.WithContext(ctx).Create(dictType).Error; err != nil {
		return fmt.Errorf("创建字典类型失败: %w", err)
	}
	return nil
}

func (r *DictRepository) SaveType(ctx context.Context, dictType *model.DictType) error {
	if err := r.db.WithContext(ctx).Save(dictType).Error; err != nil {
		return fmt.Errorf("保存字典类型失败: %w", err)
	}
	return nil
}

// DeleteType 物理删除字典类型（sys_dict_type 无软删除列）。
func (r *DictRepository) DeleteType(ctx context.Context, tx *gorm.DB, id uint64) error {
	db := r.dbOr(tx)
	if err := db.WithContext(ctx).Delete(&model.DictType{}, id).Error; err != nil {
		return fmt.Errorf("删除字典类型失败: %w", err)
	}
	return nil
}

// SyncDataDictType 在类型标识变更后同步数据表的冗余列。
//
// sys_dict_data.dict_type 是为了免 join 而冗余的副本，改了类型的 type 却不同步，
// 按类型查数据的接口就会一条都查不到——数据还在，只是再也找不到。
func (r *DictRepository) SyncDataDictType(ctx context.Context, tx *gorm.DB, typeID uint64, newType string) error {
	db := r.dbOr(tx)
	err := db.WithContext(ctx).
		Model(&model.DictData{}).
		Where("dict_type_id = ?", typeID).
		Update("dict_type", newType).Error
	if err != nil {
		return fmt.Errorf("同步字典数据的类型标识失败: %w", err)
	}
	return nil
}

// ---- 字典数据 ----

func (r *DictRepository) FindDataByID(ctx context.Context, id uint64) (*model.DictData, error) {
	var data model.DictData
	err := r.db.WithContext(ctx).First(&data, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询字典数据失败: %w", err)
	}
	return &data, nil
}

// PageData 分页查询字典数据。
func (r *DictRepository) PageData(ctx context.Context, query *dto.DictDataQuery) ([]*model.DictData, int64, error) {
	db := r.db.WithContext(ctx).Model(&model.DictData{})

	if query.DictType != "" {
		db = db.Where("dict_type = ?", query.DictType)
	}
	if query.Label != "" {
		db = db.Where("label LIKE ?", "%"+query.Label+"%")
	}
	if query.Status != nil {
		db = db.Where("status = ?", *query.Status)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计字典数据失败: %w", err)
	}

	var list []*model.DictData
	err := db.Order("sort ASC, id ASC").
		Offset(query.Offset()).
		Limit(query.Limit()).
		Find(&list).Error
	if err != nil {
		return nil, 0, fmt.Errorf("查询字典数据失败: %w", err)
	}
	return list, total, nil
}

// FindEnabledDataByType 按类型查出启用的字典项，供前端下拉框使用。
func (r *DictRepository) FindEnabledDataByType(ctx context.Context, dictType string) ([]*model.DictData, error) {
	var list []*model.DictData
	err := r.db.WithContext(ctx).
		Where("dict_type = ? AND status = ?", dictType, model.StatusEnabled).
		Order("sort ASC, id ASC").
		Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("查询字典数据失败: %w", err)
	}
	return list, nil
}

// CountData 统计某类型下的数据条数，删除类型前校验。
func (r *DictRepository) CountData(ctx context.Context, typeID uint64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.DictData{}).
		Where("dict_type_id = ?", typeID).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("统计字典数据失败: %w", err)
	}
	return count, nil
}

// ClearDefault 清除某类型下其他项的默认标记。
//
// 每个类型只应有一个默认值：多个默认时前端取默认项的逻辑会随查询顺序漂移，
// 表现为「同样的配置，有时选中这项有时选中那项」。
func (r *DictRepository) ClearDefault(ctx context.Context, tx *gorm.DB, dictType string, excludeID uint64) error {
	db := r.dbOr(tx)
	query := db.WithContext(ctx).
		Model(&model.DictData{}).
		Where("dict_type = ? AND is_default = ?", dictType, 1)
	if excludeID > 0 {
		query = query.Where("id <> ?", excludeID)
	}
	if err := query.Update("is_default", 0).Error; err != nil {
		return fmt.Errorf("清除默认标记失败: %w", err)
	}
	return nil
}

func (r *DictRepository) CreateData(ctx context.Context, tx *gorm.DB, data *model.DictData) error {
	db := r.dbOr(tx)
	if err := db.WithContext(ctx).Create(data).Error; err != nil {
		return fmt.Errorf("创建字典数据失败: %w", err)
	}
	return nil
}

func (r *DictRepository) SaveData(ctx context.Context, tx *gorm.DB, data *model.DictData) error {
	db := r.dbOr(tx)
	if err := db.WithContext(ctx).Save(data).Error; err != nil {
		return fmt.Errorf("保存字典数据失败: %w", err)
	}
	return nil
}

// DeleteData 物理删除字典数据。
func (r *DictRepository) DeleteData(ctx context.Context, tx *gorm.DB, id uint64) error {
	db := r.dbOr(tx)
	if err := db.WithContext(ctx).Delete(&model.DictData{}, id).Error; err != nil {
		return fmt.Errorf("删除字典数据失败: %w", err)
	}
	return nil
}

// DeleteDataByTypeID 删除某类型下的全部数据，随类型一起删除时使用。
func (r *DictRepository) DeleteDataByTypeID(ctx context.Context, tx *gorm.DB, typeID uint64) error {
	db := r.dbOr(tx)
	err := db.WithContext(ctx).Where("dict_type_id = ?", typeID).Delete(&model.DictData{}).Error
	if err != nil {
		return fmt.Errorf("删除字典数据失败: %w", err)
	}
	return nil
}

// DB 暴露底层句柄，供 Service 开启跨表事务。
func (r *DictRepository) DB() *gorm.DB { return r.db }

// dbOr 在 tx 为空时回退到默认句柄，使各方法既能独立调用也能参与事务。
func (r *DictRepository) dbOr(tx *gorm.DB) *gorm.DB {
	if tx != nil {
		return tx
	}
	return r.db
}
