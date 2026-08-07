package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/fangzhengcn/workbackend/apps/backend/internal/dto"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/repository"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/vo"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/errs"
)

// maxLogDeleteBatch 限制单次批量删除的条数。
//
// 不设上限时前端一次传上万个 ID 会生成超长的 IN 子句，
// 可能撞上 MySQL 的 max_allowed_packet 而整批失败。
// 需要清空时应走专门的 Clean 接口，而非把全部 ID 传过来。
const maxLogDeleteBatch = 200

// LogService 提供操作日志与登录日志的查询与清理。
//
// 只读 + 删除，没有新增/修改：日志由 middleware/operlog.go 与
// AuthService.recordLogin 写入，人工改动日志会破坏审计价值。
type LogService struct {
	logs *repository.LogRepository
}

func NewLogService(logs *repository.LogRepository) *LogService {
	return &LogService{logs: logs}
}

// ---- 操作日志 ----

// PageOperLogs 分页查询操作日志。
func (s *LogService) PageOperLogs(ctx context.Context, query *dto.OperLogQuery) ([]*vo.OperLogItem, int64, error) {
	query.Normalize()

	logs, total, err := s.logs.PageOperLogs(ctx, query)
	if err != nil {
		return nil, 0, errs.Internal("查询操作日志失败").WithCause(err)
	}
	return vo.NewOperLogItems(logs), total, nil
}

// GetOperLog 查询操作日志详情（含请求参数与响应体）。
func (s *LogService) GetOperLog(ctx context.Context, id uint64) (*vo.OperLogDetail, error) {
	log, err := s.logs.FindOperLogByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, errs.NotFound("操作日志不存在")
		}
		return nil, err
	}
	return vo.NewOperLogDetail(log), nil
}

// DeleteOperLogs 批量删除操作日志。
func (s *LogService) DeleteOperLogs(ctx context.Context, ids []uint64) (int64, error) {
	if err := validateDeleteBatch(ids); err != nil {
		return 0, err
	}

	removed, err := s.logs.DeleteOperLogs(ctx, ids)
	if err != nil {
		return 0, errs.Internal("删除操作日志失败").WithCause(err)
	}
	return removed, nil
}

// CleanOperLogs 清空全部操作日志。
func (s *LogService) CleanOperLogs(ctx context.Context) (int64, error) {
	removed, err := s.logs.CleanOperLogs(ctx)
	if err != nil {
		return 0, errs.Internal("清空操作日志失败").WithCause(err)
	}
	return removed, nil
}

// ---- 登录日志 ----

// PageLoginLogs 分页查询登录日志。
func (s *LogService) PageLoginLogs(ctx context.Context, query *dto.LoginLogQuery) ([]*vo.LoginLogItem, int64, error) {
	query.Normalize()

	logs, total, err := s.logs.PageLoginLogs(ctx, query)
	if err != nil {
		return nil, 0, errs.Internal("查询登录日志失败").WithCause(err)
	}
	return vo.NewLoginLogItems(logs), total, nil
}

// DeleteLoginLogs 批量删除登录日志。
func (s *LogService) DeleteLoginLogs(ctx context.Context, ids []uint64) (int64, error) {
	if err := validateDeleteBatch(ids); err != nil {
		return 0, err
	}

	removed, err := s.logs.DeleteLoginLogs(ctx, ids)
	if err != nil {
		return 0, errs.Internal("删除登录日志失败").WithCause(err)
	}
	return removed, nil
}

// CleanLoginLogs 清空全部登录日志。
func (s *LogService) CleanLoginLogs(ctx context.Context) (int64, error) {
	removed, err := s.logs.CleanLoginLogs(ctx)
	if err != nil {
		return 0, errs.Internal("清空登录日志失败").WithCause(err)
	}
	return removed, nil
}

// maxLogExportRows 限制单次日志导出的行数，理由同 maxExportRows。
//
// 比用户导出给得宽：日志字段短、无关联查询，同样内存下能容纳更多行；
// 而日志本就是海量数据，卡太死会让导出功能没法用。
const maxLogExportRows = 50000

// ExportOperLogs 按查询条件导出操作日志。
//
// 不含 requestParam / jsonResult 两个 TEXT 字段——它们只在详情接口返回。
// 导出带上会让文件体积膨胀几个数量级，且其中可能含大量请求细节，
// 一份外流的导出文件等于把系统内部行为完整交出去。
func (s *LogService) ExportOperLogs(ctx context.Context, query *dto.OperLogQuery) ([]*vo.OperLogItem, error) {
	query.Page = 1
	query.Size = maxLogExportRows + 1

	logs, total, err := s.logs.PageOperLogs(ctx, query)
	if err != nil {
		return nil, errs.Internal("查询操作日志失败").WithCause(err)
	}
	if total > maxLogExportRows {
		return nil, exportLimitErr(total, maxLogExportRows)
	}
	return vo.NewOperLogItems(logs), nil
}

// ExportLoginLogs 按查询条件导出登录日志。
func (s *LogService) ExportLoginLogs(ctx context.Context, query *dto.LoginLogQuery) ([]*vo.LoginLogItem, error) {
	query.Page = 1
	query.Size = maxLogExportRows + 1

	logs, total, err := s.logs.PageLoginLogs(ctx, query)
	if err != nil {
		return nil, errs.Internal("查询登录日志失败").WithCause(err)
	}
	if total > maxLogExportRows {
		return nil, exportLimitErr(total, maxLogExportRows)
	}
	return vo.NewLoginLogItems(logs), nil
}

// exportLimitErr 统一超限提示：明确告知实际条数与上限，而非笼统报错，
// 否则用户不知道该把条件收窄到什么程度。
func exportLimitErr(total int64, limit int) error {
	return errs.BadRequest(fmt.Sprintf(
		"符合条件的数据有 %d 条，超过单次导出上限 %d 条，请收窄时间范围后重试",
		total, limit))
}

// validateDeleteBatch 校验批量删除的 ID 集合。
func validateDeleteBatch(ids []uint64) error {
	if len(ids) == 0 {
		return errs.BadRequest("请选择要删除的记录")
	}
	if len(ids) > maxLogDeleteBatch {
		return errs.BadRequest("单次最多删除 200 条，如需清空请使用「清空」功能")
	}
	return nil
}
