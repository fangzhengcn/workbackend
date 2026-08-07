package controller

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/fangzhengcn/workbackend/apps/backend/internal/dto"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/service"
	_ "github.com/fangzhengcn/workbackend/apps/backend/internal/vo"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/errs"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/response"
)

// DictController 提供字典类型与字典数据的管理接口。
type DictController struct {
	dicts *service.DictService
}

func NewDictController(dicts *service.DictService) *DictController {
	return &DictController{dicts: dicts}
}

// ListTypes 分页查询字典类型。
//
// @Summary  字典类型列表
// @Tags     字典管理
// @Produce  json
// @Param    query query dto.DictTypeQuery false "查询条件"
// @Success  200 {object} response.Body{data=response.PageData{list=[]vo.DictTypeItem}}
// @Router   /dicts/types [get]
func (ctl *DictController) ListTypes(c *gin.Context) {
	var query dto.DictTypeQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Fail(c, errs.BadRequest("参数错误："+err.Error()))
		return
	}
	items, total, err := ctl.dicts.PageTypes(c.Request.Context(), &query)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Page(c, items, total, query.Page, query.Size)
}

// CreateType 新增字典类型。
//
// @Summary  新增字典类型
// @Tags     字典管理
// @Accept   json
// @Produce  json
// @Param    body body dto.CreateDictTypeRequest true "字典类型"
// @Success  200 {object} response.Body
// @Router   /dicts/types [post]
func (ctl *DictController) CreateType(c *gin.Context) {
	var req dto.CreateDictTypeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errs.BadRequest("参数错误："+err.Error()))
		return
	}
	if err := ctl.dicts.CreateType(c.Request.Context(), &req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMessage(c, "新增成功")
}

// UpdateType 修改字典类型。
//
// @Summary  修改字典类型
// @Tags     字典管理
// @Accept   json
// @Produce  json
// @Param    id   path int                       true "字典类型ID"
// @Param    body body dto.UpdateDictTypeRequest true "字典类型"
// @Success  200 {object} response.Body
// @Router   /dicts/types/{id} [put]
func (ctl *DictController) UpdateType(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req dto.UpdateDictTypeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errs.BadRequest("参数错误："+err.Error()))
		return
	}
	if err := ctl.dicts.UpdateType(c.Request.Context(), id, &req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMessage(c, "修改成功")
}

// DeleteType 删除字典类型及其下全部数据。
//
// @Summary  删除字典类型
// @Tags     字典管理
// @Produce  json
// @Param    id path int true "字典类型ID"
// @Success  200 {object} response.Body
// @Router   /dicts/types/{id} [delete]
func (ctl *DictController) DeleteType(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	removed, err := ctl.dicts.DeleteType(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	// 明确告知级联删除了多少条数据，避免用户以为只删了类型本身。
	if removed > 0 {
		response.OKMessage(c, "删除成功，同时删除了 "+strconv.FormatInt(removed, 10)+" 条字典数据")
		return
	}
	response.OKMessage(c, "删除成功")
}

// ListData 分页查询字典数据。
//
// @Summary  字典数据列表
// @Tags     字典管理
// @Produce  json
// @Param    query query dto.DictDataQuery false "查询条件"
// @Success  200 {object} response.Body{data=response.PageData{list=[]vo.DictDataItem}}
// @Router   /dicts/data [get]
func (ctl *DictController) ListData(c *gin.Context) {
	var query dto.DictDataQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Fail(c, errs.BadRequest("参数错误："+err.Error()))
		return
	}
	items, total, err := ctl.dicts.PageData(c.Request.Context(), &query)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Page(c, items, total, query.Page, query.Size)
}

// DataByType 按类型查询启用的字典项，供业务页面的下拉框使用。
//
// @Summary  按类型取字典项
// @Tags     字典管理
// @Produce  json
// @Param    type path string true "字典类型标识"
// @Success  200 {object} response.Body{data=[]vo.DictDataItem}
// @Router   /dicts/data/type/{type} [get]
func (ctl *DictController) DataByType(c *gin.Context) {
	dictType := c.Param("type")
	if dictType == "" {
		response.Fail(c, errs.BadRequest("字典类型不能为空"))
		return
	}
	items, err := ctl.dicts.DataByType(c.Request.Context(), dictType)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, items)
}

// CreateData 新增字典数据项。
//
// @Summary  新增字典数据
// @Tags     字典管理
// @Accept   json
// @Produce  json
// @Param    body body dto.CreateDictDataRequest true "字典数据"
// @Success  200 {object} response.Body
// @Router   /dicts/data [post]
func (ctl *DictController) CreateData(c *gin.Context) {
	var req dto.CreateDictDataRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errs.BadRequest("参数错误："+err.Error()))
		return
	}
	if err := ctl.dicts.CreateData(c.Request.Context(), &req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMessage(c, "新增成功")
}

// UpdateData 修改字典数据项。
//
// @Summary  修改字典数据
// @Tags     字典管理
// @Accept   json
// @Produce  json
// @Param    id   path int                       true "字典数据ID"
// @Param    body body dto.UpdateDictDataRequest true "字典数据"
// @Success  200 {object} response.Body
// @Router   /dicts/data/{id} [put]
func (ctl *DictController) UpdateData(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req dto.UpdateDictDataRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errs.BadRequest("参数错误："+err.Error()))
		return
	}
	if err := ctl.dicts.UpdateData(c.Request.Context(), id, &req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMessage(c, "修改成功")
}

// DeleteData 删除字典数据项。
//
// @Summary  删除字典数据
// @Tags     字典管理
// @Produce  json
// @Param    id path int true "字典数据ID"
// @Success  200 {object} response.Body
// @Router   /dicts/data/{id} [delete]
func (ctl *DictController) DeleteData(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	if err := ctl.dicts.DeleteData(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMessage(c, "删除成功")
}
