package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/fangzhengcn/workbackend/apps/backend/internal/dto"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/model"
)

// LogRepository 负责操作日志与登录日志的写入与查询。
type LogRepository struct {
	db *gorm.DB
}

func NewLogRepository(db *gorm.DB) *LogRepository {
	return &LogRepository{db: db}
}

// CreateOperLog 写入操作日志。
//
// 调用方须确保 RequestParam 已脱敏，避免密码/手机号明文入库。
func (r *LogRepository) CreateOperLog(ctx context.Context, log *model.OperLog) error {
	if err := r.db.WithContext(ctx).Create(log).Error; err != nil {
		return fmt.Errorf("写入操作日志失败: %w", err)
	}
	return nil
}

// CreateLoginLog 写入登录日志（含失败记录）。
func (r *LogRepository) CreateLoginLog(ctx context.Context, log *model.LoginLog) error {
	if err := r.db.WithContext(ctx).Create(log).Error; err != nil {
		return fmt.Errorf("写入登录日志失败: %w", err)
	}
	return nil
}

// operLogListColumns 是列表查询要取的列。
//
// 刻意排除 request_param 与 json_result 两个 TEXT 列：
// 它们可能各有数 KB，列表一页 10 条就要多传几十 KB，而列表页并不展示它们。
// 详情接口（FindOperLogByID）才取全部字段。
var operLogListColumns = []string{
	"id", "title", "business_type", "method", "request_url",
	"oper_user_id", "oper_name", "oper_ip", "status", "error_msg",
	"cost_time", "created_at",
}

// PageOperLogs 分页查询操作日志，支持按模块/操作人/业务类型/状态/时间区间筛选。
func (r *LogRepository) PageOperLogs(ctx context.Context, query *dto.OperLogQuery) ([]*model.OperLog, int64, error) {
	db := r.db.WithContext(ctx).Model(&model.OperLog{})

	if query.Title != "" {
		db = db.Where("title LIKE ?", "%"+query.Title+"%")
	}
	if query.OperName != "" {
		db = db.Where("oper_name LIKE ?", "%"+query.OperName+"%")
	}
	if query.BusinessType != nil {
		db = db.Where("business_type = ?", *query.BusinessType)
	}
	if query.Status != nil {
		db = db.Where("status = ?", *query.Status)
	}
	if query.BeginTime != "" {
		db = db.Where("created_at >= ?", query.BeginTime)
	}
	// 结束日期按「当天 23:59:59」处理，由调用方传入完整时间戳；
	// 这里只做区间比较，不猜测精度。
	if query.EndTime != "" {
		db = db.Where("created_at <= ?", query.EndTime)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计操作日志失败: %w", err)
	}

	var logs []*model.OperLog
	err := db.Select(operLogListColumns).
		Order("id DESC").
		Offset(query.Offset()).
		Limit(query.Limit()).
		Find(&logs).Error
	if err != nil {
		return nil, 0, fmt.Errorf("查询操作日志失败: %w", err)
	}
	return logs, total, nil
}

// FindOperLogByID 查询操作日志详情，含请求参数与响应体。
func (r *LogRepository) FindOperLogByID(ctx context.Context, id uint64) (*model.OperLog, error) {
	var log model.OperLog
	err := r.db.WithContext(ctx).First(&log, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询操作日志失败: %w", err)
	}
	return &log, nil
}

// DeleteOperLogs 按 ID 批量删除操作日志。
func (r *LogRepository) DeleteOperLogs(ctx context.Context, ids []uint64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	result := r.db.WithContext(ctx).Where("id IN ?", ids).Delete(&model.OperLog{})
	if result.Error != nil {
		return 0, fmt.Errorf("删除操作日志失败: %w", result.Error)
	}
	return result.RowsAffected, nil
}

// CleanOperLogs 清空全部操作日志。
//
// 用 DELETE 而非 TRUNCATE：TRUNCATE 是 DDL，在事务中无法回滚，
// 且部分环境下需要更高权限。
func (r *LogRepository) CleanOperLogs(ctx context.Context) (int64, error) {
	// GORM 要求批量删除必须带条件，用恒真条件表达「全部」。
	result := r.db.WithContext(ctx).Where("1 = 1").Delete(&model.OperLog{})
	if result.Error != nil {
		return 0, fmt.Errorf("清空操作日志失败: %w", result.Error)
	}
	return result.RowsAffected, nil
}

// PageLoginLogs 分页查询登录日志，支持按账号/状态/时间区间筛选。
func (r *LogRepository) PageLoginLogs(ctx context.Context, query *dto.LoginLogQuery) ([]*model.LoginLog, int64, error) {
	db := r.db.WithContext(ctx).Model(&model.LoginLog{})

	if query.Username != "" {
		db = db.Where("username LIKE ?", "%"+query.Username+"%")
	}
	if query.IPAddr != "" {
		db = db.Where("ipaddr LIKE ?", "%"+query.IPAddr+"%")
	}
	if query.Status != nil {
		db = db.Where("status = ?", *query.Status)
	}
	// 登录日志的时间列是 login_time，不是 created_at。
	if query.BeginTime != "" {
		db = db.Where("login_time >= ?", query.BeginTime)
	}
	if query.EndTime != "" {
		db = db.Where("login_time <= ?", query.EndTime)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计登录日志失败: %w", err)
	}

	var logs []*model.LoginLog
	err := db.Order("id DESC").
		Offset(query.Offset()).
		Limit(query.Limit()).
		Find(&logs).Error
	if err != nil {
		return nil, 0, fmt.Errorf("查询登录日志失败: %w", err)
	}
	return logs, total, nil
}

// DeleteLoginLogs 按 ID 批量删除登录日志。
func (r *LogRepository) DeleteLoginLogs(ctx context.Context, ids []uint64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	result := r.db.WithContext(ctx).Where("id IN ?", ids).Delete(&model.LoginLog{})
	if result.Error != nil {
		return 0, fmt.Errorf("删除登录日志失败: %w", result.Error)
	}
	return result.RowsAffected, nil
}

// CleanLoginLogs 清空全部登录日志。
func (r *LogRepository) CleanLoginLogs(ctx context.Context) (int64, error) {
	result := r.db.WithContext(ctx).Where("1 = 1").Delete(&model.LoginLog{})
	if result.Error != nil {
		return 0, fmt.Errorf("清空登录日志失败: %w", result.Error)
	}
	return result.RowsAffected, nil
}
