package service

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"gorm.io/gorm"

	"github.com/fangzhengcn/workbackend/apps/backend/internal/dto"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/model"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/repository"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/vo"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/errs"
)

// dictTypePattern 限定字典类型标识的字符集。
//
// 该值会作为 URL 路径段用于按类型取数据（/dicts/data/{type}），
// 含空格或斜杠会导致路由匹配不到，且不便于排查。
var dictTypePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]*$`)

// DictService 负责字典类型与字典数据的增删改查。
//
// 两个不变量：
//  1. sys_dict_data.dict_type 是为免 join 而冗余的副本，
//     必须始终与所属 sys_dict_type.type 一致。
//  2. 同一类型下最多一个 is_default=1。
type DictService struct {
	dicts *repository.DictRepository
}

func NewDictService(dicts *repository.DictRepository) *DictService {
	return &DictService{dicts: dicts}
}

// ---- 字典类型 ----

// PageTypes 分页查询字典类型。
func (s *DictService) PageTypes(ctx context.Context, query *dto.DictTypeQuery) ([]*vo.DictTypeItem, int64, error) {
	query.Normalize()

	types, total, err := s.dicts.PageTypes(ctx, query)
	if err != nil {
		return nil, 0, errs.Internal("查询字典类型失败").WithCause(err)
	}
	return vo.NewDictTypeItems(types), total, nil
}

// CreateType 新增字典类型。
func (s *DictService) CreateType(ctx context.Context, req *dto.CreateDictTypeRequest) error {
	dictType := strings.TrimSpace(req.Type)
	if err := s.validateTypeCode(dictType); err != nil {
		return err
	}

	exists, err := s.dicts.ExistsType(ctx, dictType, 0)
	if err != nil {
		return errs.Internal("校验字典类型失败").WithCause(err)
	}
	if exists {
		return errs.BadRequest("字典类型标识已存在")
	}

	entity := &model.DictType{
		Name:   req.Name,
		Type:   dictType,
		Status: defaultInt8(req.Status, model.StatusEnabled),
		Remark: req.Remark,
	}
	if err := s.dicts.CreateType(ctx, entity); err != nil {
		return errs.Internal("创建字典类型失败").WithCause(err)
	}
	return nil
}

// UpdateType 修改字典类型。
//
// 改动 type 时必须同步 sys_dict_data 的冗余列，两者在同一事务内完成——
// 只改类型不改数据，按类型查数据的接口会一条都查不到（数据还在，只是找不到了）。
func (s *DictService) UpdateType(ctx context.Context, id uint64, req *dto.UpdateDictTypeRequest) error {
	entity, err := s.dicts.FindTypeByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return errs.NotFound("字典类型不存在")
		}
		return err
	}

	typeChanged := false
	if req.Type != nil {
		newType := strings.TrimSpace(*req.Type)
		if newType != entity.Type {
			if err := s.validateTypeCode(newType); err != nil {
				return err
			}
			exists, err := s.dicts.ExistsType(ctx, newType, id)
			if err != nil {
				return errs.Internal("校验字典类型失败").WithCause(err)
			}
			if exists {
				return errs.BadRequest("字典类型标识已存在")
			}
			entity.Type = newType
			typeChanged = true
		}
	}

	if req.Name != nil {
		entity.Name = *req.Name
	}
	if req.Status != nil {
		entity.Status = *req.Status
	}
	if req.Remark != nil {
		entity.Remark = *req.Remark
	}

	return s.dicts.DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.WithContext(ctx).Save(entity).Error; err != nil {
			return errs.Internal("保存字典类型失败").WithCause(err)
		}
		if typeChanged {
			if err := s.dicts.SyncDataDictType(ctx, tx, id, entity.Type); err != nil {
				return errs.Internal("同步字典数据失败").WithCause(err)
			}
		}
		return nil
	})
}

// DeleteType 删除字典类型及其下全部数据。
//
// 与菜单/部门不同，这里选择级联删除而非拦住：字典数据不能脱离类型存在，
// 留下孤儿数据只会变成永远查不到的垃圾。但会在响应里说明删了多少条，
// 让用户知道代价。
func (s *DictService) DeleteType(ctx context.Context, id uint64) (int64, error) {
	if _, err := s.dicts.FindTypeByID(ctx, id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return 0, errs.NotFound("字典类型不存在")
		}
		return 0, err
	}

	count, err := s.dicts.CountData(ctx, id)
	if err != nil {
		return 0, errs.Internal("统计字典数据失败").WithCause(err)
	}

	err = s.dicts.DB().Transaction(func(tx *gorm.DB) error {
		if err := s.dicts.DeleteDataByTypeID(ctx, tx, id); err != nil {
			return errs.Internal("删除字典数据失败").WithCause(err)
		}
		if err := s.dicts.DeleteType(ctx, tx, id); err != nil {
			return errs.Internal("删除字典类型失败").WithCause(err)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}

// ---- 字典数据 ----

// PageData 分页查询字典数据。
func (s *DictService) PageData(ctx context.Context, query *dto.DictDataQuery) ([]*vo.DictDataItem, int64, error) {
	query.Normalize()

	list, total, err := s.dicts.PageData(ctx, query)
	if err != nil {
		return nil, 0, errs.Internal("查询字典数据失败").WithCause(err)
	}
	return vo.NewDictDataItems(list), total, nil
}

// DataByType 按类型查询启用的字典项，供前端下拉框使用。
func (s *DictService) DataByType(ctx context.Context, dictType string) ([]*vo.DictDataItem, error) {
	list, err := s.dicts.FindEnabledDataByType(ctx, strings.TrimSpace(dictType))
	if err != nil {
		return nil, errs.Internal("查询字典数据失败").WithCause(err)
	}
	return vo.NewDictDataItems(list), nil
}

// CreateData 新增字典数据项。
func (s *DictService) CreateData(ctx context.Context, req *dto.CreateDictDataRequest) error {
	// dict_type 冗余列由类型 ID 推导，不接受前端传入，避免两者不一致。
	dictType, err := s.dicts.FindTypeByID(ctx, req.DictTypeID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return errs.BadRequest("所属字典类型不存在")
		}
		return err
	}

	isDefault := defaultInt8(req.IsDefault, 0)
	data := &model.DictData{
		DictTypeID: dictType.ID,
		DictType:   dictType.Type,
		Label:      req.Label,
		Value:      req.Value,
		Sort:       req.Sort,
		IsDefault:  isDefault,
		Status:     defaultInt8(req.Status, model.StatusEnabled),
		Remark:     req.Remark,
	}

	return s.dicts.DB().Transaction(func(tx *gorm.DB) error {
		// 设为默认前先清掉同类型下原有的默认项，保持「每类型最多一个默认」。
		if isDefault == 1 {
			if err := s.dicts.ClearDefault(ctx, tx, dictType.Type, 0); err != nil {
				return errs.Internal("清除原默认项失败").WithCause(err)
			}
		}
		if err := s.dicts.CreateData(ctx, tx, data); err != nil {
			return errs.Internal("创建字典数据失败").WithCause(err)
		}
		return nil
	})
}

// UpdateData 修改字典数据项。
func (s *DictService) UpdateData(ctx context.Context, id uint64, req *dto.UpdateDictDataRequest) error {
	data, err := s.dicts.FindDataByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return errs.NotFound("字典数据不存在")
		}
		return err
	}

	if req.Label != nil {
		data.Label = *req.Label
	}
	if req.Value != nil {
		data.Value = *req.Value
	}
	if req.Sort != nil {
		data.Sort = *req.Sort
	}
	if req.Status != nil {
		data.Status = *req.Status
	}
	if req.Remark != nil {
		data.Remark = *req.Remark
	}

	becameDefault := req.IsDefault != nil && *req.IsDefault == 1
	if req.IsDefault != nil {
		data.IsDefault = *req.IsDefault
	}

	return s.dicts.DB().Transaction(func(tx *gorm.DB) error {
		if becameDefault {
			if err := s.dicts.ClearDefault(ctx, tx, data.DictType, id); err != nil {
				return errs.Internal("清除原默认项失败").WithCause(err)
			}
		}
		if err := s.dicts.SaveData(ctx, tx, data); err != nil {
			return errs.Internal("保存字典数据失败").WithCause(err)
		}
		return nil
	})
}

// DeleteData 删除字典数据项。
func (s *DictService) DeleteData(ctx context.Context, id uint64) error {
	if _, err := s.dicts.FindDataByID(ctx, id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return errs.NotFound("字典数据不存在")
		}
		return err
	}
	if err := s.dicts.DeleteData(ctx, nil, id); err != nil {
		return errs.Internal("删除字典数据失败").WithCause(err)
	}
	return nil
}

// validateTypeCode 校验类型标识的字符集。
func (s *DictService) validateTypeCode(dictType string) error {
	if !dictTypePattern.MatchString(dictType) {
		return errs.BadRequest("字典类型标识只允许字母、数字与下划线，且需以字母开头")
	}
	return nil
}
