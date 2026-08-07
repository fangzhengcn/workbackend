package controller

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/fangzhengcn/workbackend/apps/backend/internal/dto"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/model"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/service"
	_ "github.com/fangzhengcn/workbackend/apps/backend/internal/vo"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/errs"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/export"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/response"
)

// LogController 提供操作日志与登录日志的查询与清理接口。
type LogController struct {
	logs *service.LogService
}

func NewLogController(logs *service.LogService) *LogController {
	return &LogController{logs: logs}
}

// ---- 操作日志 ----

// ListOperLogs 分页查询操作日志。
//
// @Summary  操作日志列表
// @Tags     日志管理
// @Produce  json
// @Param    query query dto.OperLogQuery false "查询条件"
// @Success  200 {object} response.Body{data=response.PageData{list=[]vo.OperLogItem}}
// @Router   /oper-logs [get]
func (ctl *LogController) ListOperLogs(c *gin.Context) {
	var query dto.OperLogQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Fail(c, errs.BadRequest("参数错误："+err.Error()))
		return
	}
	items, total, err := ctl.logs.PageOperLogs(c.Request.Context(), &query)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Page(c, items, total, query.Page, query.Size)
}

// GetOperLog 查询操作日志详情。
//
// @Summary  操作日志详情
// @Tags     日志管理
// @Produce  json
// @Param    id path int true "日志ID"
// @Success  200 {object} response.Body{data=vo.OperLogDetail}
// @Router   /oper-logs/{id} [get]
func (ctl *LogController) GetOperLog(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	item, err := ctl.logs.GetOperLog(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, item)
}

// DeleteOperLogs 批量删除操作日志。
//
// @Summary  删除操作日志
// @Tags     日志管理
// @Accept   json
// @Produce  json
// @Param    body body dto.DeleteLogsRequest true "日志ID集合"
// @Success  200 {object} response.Body
// @Router   /oper-logs [delete]
func (ctl *LogController) DeleteOperLogs(c *gin.Context) {
	var req dto.DeleteLogsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errs.BadRequest("参数错误："+err.Error()))
		return
	}
	removed, err := ctl.logs.DeleteOperLogs(c.Request.Context(), req.IDs)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMessage(c, "已删除 "+strconv.FormatInt(removed, 10)+" 条")
}

// CleanOperLogs 清空全部操作日志。
//
// @Summary  清空操作日志
// @Tags     日志管理
// @Produce  json
// @Success  200 {object} response.Body
// @Router   /oper-logs/clean [delete]
func (ctl *LogController) CleanOperLogs(c *gin.Context) {
	removed, err := ctl.logs.CleanOperLogs(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMessage(c, "已清空 "+strconv.FormatInt(removed, 10)+" 条")
}

// ExportOperLogs 导出操作日志为 CSV。
//
// 不含请求参数与响应体：那两个 TEXT 字段会让文件膨胀几个数量级，
// 且含大量请求细节，一份外流的导出等于交出系统内部行为。
//
// @Summary  导出操作日志
// @Tags     日志管理
// @Produce  text/csv
// @Param    query query dto.OperLogQuery false "查询条件（与列表一致）"
// @Success  200 {file} file "CSV 文件"
// @Router   /oper-logs/export [get]
func (ctl *LogController) ExportOperLogs(c *gin.Context) {
	var query dto.OperLogQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Fail(c, errs.BadRequest("参数错误："+err.Error()))
		return
	}

	items, err := ctl.logs.ExportOperLogs(c.Request.Context(), &query)
	if err != nil {
		response.Fail(c, err)
		return
	}

	// 查询已完成才设置响应头：头一旦发出就无法再改成 JSON 错误响应。
	export.SetHeaders(c.Writer, export.Filename("操作日志", time.Now()))
	w, err := export.NewWriter(c.Writer)
	if err != nil {
		response.Fail(c, errs.Internal("导出失败").WithCause(err))
		return
	}

	header := []string{"操作模块", "业务类型", "操作人", "IP", "请求方式", "请求地址", "状态", "错误信息", "耗时(ms)", "操作时间"}
	if err := w.WriteRow(header); err != nil {
		_ = c.Error(err)
		return
	}

	for _, item := range items {
		row := []string{
			item.Title,
			businessTypeLabel(item.BusinessType),
			item.OperName,
			item.OperIP,
			item.Method,
			item.RequestURL,
			operStatusLabel(item.Status),
			item.ErrorMsg,
			strconv.Itoa(item.CostTime),
			export.FormatTime(item.CreatedAt),
		}
		if err := w.WriteRow(row); err != nil {
			_ = c.Error(err)
			return
		}
	}

	if err := w.Flush(); err != nil {
		_ = c.Error(err)
	}
}

// ExportLoginLogs 导出登录日志为 CSV。
//
// @Summary  导出登录日志
// @Tags     日志管理
// @Produce  text/csv
// @Param    query query dto.LoginLogQuery false "查询条件（与列表一致）"
// @Success  200 {file} file "CSV 文件"
// @Router   /login-logs/export [get]
func (ctl *LogController) ExportLoginLogs(c *gin.Context) {
	var query dto.LoginLogQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Fail(c, errs.BadRequest("参数错误："+err.Error()))
		return
	}

	items, err := ctl.logs.ExportLoginLogs(c.Request.Context(), &query)
	if err != nil {
		response.Fail(c, err)
		return
	}

	export.SetHeaders(c.Writer, export.Filename("登录日志", time.Now()))
	w, err := export.NewWriter(c.Writer)
	if err != nil {
		response.Fail(c, errs.Internal("导出失败").WithCause(err))
		return
	}

	header := []string{"登录账号", "登录IP", "登录地点", "浏览器", "操作系统", "状态", "提示消息", "登录时间"}
	if err := w.WriteRow(header); err != nil {
		_ = c.Error(err)
		return
	}

	for _, item := range items {
		row := []string{
			item.Username,
			item.IPAddr,
			item.Location,
			item.Browser,
			item.OS,
			operStatusLabel(item.Status),
			item.Msg,
			export.FormatTime(item.LoginTime),
		}
		if err := w.WriteRow(row); err != nil {
			_ = c.Error(err)
			return
		}
	}

	if err := w.Flush(); err != nil {
		_ = c.Error(err)
	}
}

// businessTypeLabel 把业务类型码转成可读文字。
func businessTypeLabel(t int8) string {
	switch t {
	case model.BusinessTypeInsert:
		return "新增"
	case model.BusinessTypeUpdate:
		return "修改"
	case model.BusinessTypeDelete:
		return "删除"
	case model.BusinessTypeQuery:
		return "查询"
	default:
		return "其他"
	}
}

// operStatusLabel 日志状态：1 成功 0 失败。
func operStatusLabel(status int8) string {
	if status == model.StatusEnabled {
		return "成功"
	}
	return "失败"
}

// ---- 登录日志 ----

// ListLoginLogs 分页查询登录日志。
//
// @Summary  登录日志列表
// @Tags     日志管理
// @Produce  json
// @Param    query query dto.LoginLogQuery false "查询条件"
// @Success  200 {object} response.Body{data=response.PageData{list=[]vo.LoginLogItem}}
// @Router   /login-logs [get]
func (ctl *LogController) ListLoginLogs(c *gin.Context) {
	var query dto.LoginLogQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Fail(c, errs.BadRequest("参数错误："+err.Error()))
		return
	}
	items, total, err := ctl.logs.PageLoginLogs(c.Request.Context(), &query)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Page(c, items, total, query.Page, query.Size)
}

// DeleteLoginLogs 批量删除登录日志。
//
// @Summary  删除登录日志
// @Tags     日志管理
// @Accept   json
// @Produce  json
// @Param    body body dto.DeleteLogsRequest true "日志ID集合"
// @Success  200 {object} response.Body
// @Router   /login-logs [delete]
func (ctl *LogController) DeleteLoginLogs(c *gin.Context) {
	var req dto.DeleteLogsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errs.BadRequest("参数错误："+err.Error()))
		return
	}
	removed, err := ctl.logs.DeleteLoginLogs(c.Request.Context(), req.IDs)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMessage(c, "已删除 "+strconv.FormatInt(removed, 10)+" 条")
}

// CleanLoginLogs 清空全部登录日志。
//
// @Summary  清空登录日志
// @Tags     日志管理
// @Produce  json
// @Success  200 {object} response.Body
// @Router   /login-logs/clean [delete]
func (ctl *LogController) CleanLoginLogs(c *gin.Context) {
	removed, err := ctl.logs.CleanLoginLogs(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMessage(c, "已清空 "+strconv.FormatInt(removed, 10)+" 条")
}
