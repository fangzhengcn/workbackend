package dto

// DictTypeQuery 字典类型列表查询条件。
type DictTypeQuery struct {
	PageQuery
	Name   string `form:"name" binding:"omitempty,max=64"`
	Type   string `form:"type" binding:"omitempty,max=64"`
	Status *int8  `form:"status" binding:"omitempty,oneof=0 1"`
}

// CreateDictTypeRequest 新增字典类型。
type CreateDictTypeRequest struct {
	Name string `json:"name" binding:"required,max=64"`
	// Type 是数据项的归属键，只允许字母/数字/下划线：
	// 它会作为 URL 路径段用于按类型取数据（/dicts/data/{type}）。
	Type   string `json:"type" binding:"required,max=64"`
	Status *int8  `json:"status" binding:"omitempty,oneof=0 1"`
	Remark string `json:"remark" binding:"omitempty,max=255"`
}

// UpdateDictTypeRequest 修改字典类型。
//
// Type 允许修改，但服务层会同步更新 sys_dict_data 的冗余列。
type UpdateDictTypeRequest struct {
	Name   *string `json:"name" binding:"omitempty,max=64"`
	Type   *string `json:"type" binding:"omitempty,max=64"`
	Status *int8   `json:"status" binding:"omitempty,oneof=0 1"`
	Remark *string `json:"remark" binding:"omitempty,max=255"`
}

// DictDataQuery 字典数据列表查询条件。
type DictDataQuery struct {
	PageQuery
	// DictType 为空时返回全部类型的数据；前端双栏布局下通常按类型过滤。
	DictType string `form:"dictType" binding:"omitempty,max=64"`
	Label    string `form:"label" binding:"omitempty,max=100"`
	Status   *int8  `form:"status" binding:"omitempty,oneof=0 1"`
}

// CreateDictDataRequest 新增字典数据项。
type CreateDictDataRequest struct {
	// DictTypeID 决定归属；dict_type 冗余列由服务层按此 ID 推导，不由前端传入，
	// 避免两者不一致。
	DictTypeID uint64 `json:"dictTypeId" binding:"required"`
	Label      string `json:"label" binding:"required,max=100"`
	Value      string `json:"value" binding:"required,max=100"`
	Sort       int    `json:"sort"`
	IsDefault  *int8  `json:"isDefault" binding:"omitempty,oneof=0 1"`
	Status     *int8  `json:"status" binding:"omitempty,oneof=0 1"`
	Remark     string `json:"remark" binding:"omitempty,max=255"`
}

// UpdateDictDataRequest 修改字典数据项。归属类型不可改。
type UpdateDictDataRequest struct {
	Label     *string `json:"label" binding:"omitempty,max=100"`
	Value     *string `json:"value" binding:"omitempty,max=100"`
	Sort      *int    `json:"sort"`
	IsDefault *int8   `json:"isDefault" binding:"omitempty,oneof=0 1"`
	Status    *int8   `json:"status" binding:"omitempty,oneof=0 1"`
	Remark    *string `json:"remark" binding:"omitempty,max=255"`
}
