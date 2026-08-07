package model

// DictType 对应 sys_dict_type 表：字典分类。
type DictType struct {
	ID     uint64 `gorm:"primaryKey;column:id" json:"id"`
	Name   string `gorm:"column:name;size:64;not null" json:"name"`
	Type   string `gorm:"column:type;size:64;not null;uniqueIndex:uk_type" json:"type"`
	Status int8   `gorm:"column:status;not null;default:1" json:"status"`
	Remark string `gorm:"column:remark;size:255" json:"remark"`

	Timestamps
}

func (DictType) TableName() string { return "sys_dict_type" }

// DictData 对应 sys_dict_data 表：字典键值项。
type DictData struct {
	ID         uint64 `gorm:"primaryKey;column:id" json:"id"`
	DictTypeID uint64 `gorm:"column:dict_type_id;not null" json:"dictTypeId"`
	// DictType 是冗余字段，便于按类型直接查询而无需 join。
	DictType  string `gorm:"column:dict_type;size:64;not null;index:idx_dict_type" json:"dictType"`
	Label     string `gorm:"column:label;size:100;not null" json:"label"`
	Value     string `gorm:"column:value;size:100;not null" json:"value"`
	Sort      int    `gorm:"column:sort;not null;default:0" json:"sort"`
	IsDefault int8   `gorm:"column:is_default;not null;default:0" json:"isDefault"`
	Status    int8   `gorm:"column:status;not null;default:1" json:"status"`
	Remark    string `gorm:"column:remark;size:255" json:"remark"`

	Timestamps
}

func (DictData) TableName() string { return "sys_dict_data" }
