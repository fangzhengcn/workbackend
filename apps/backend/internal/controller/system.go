package controller

import (
	"github.com/gin-gonic/gin"

	"github.com/fangzhengcn/workbackend/apps/backend/internal/dto"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/middleware"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/service"
	_ "github.com/fangzhengcn/workbackend/apps/backend/internal/vo"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/errs"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/response"
)

/*
 * 角色 / 菜单 / 部门管理（设计文档「阶段三」）。
 *
 * 三者均已完整实现，可作为其余模块的参照：
 * Controller 只做参数绑定与响应组装，业务逻辑与事务边界都在 Service。
 */

// RoleController 提供角色管理接口。
type RoleController struct {
	roles *service.RoleService
}

func NewRoleController(roles *service.RoleService) *RoleController {
	return &RoleController{roles: roles}
}

// List 分页查询角色列表。
//
// @Summary  角色列表
// @Tags     角色管理
// @Produce  json
// @Param    query query dto.RoleQuery false "查询条件"
// @Success  200 {object} response.Body{data=response.PageData{list=[]vo.RoleItem}}
// @Router   /roles [get]
func (ctl *RoleController) List(c *gin.Context) {
	var query dto.RoleQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Fail(c, errs.BadRequest("参数错误："+err.Error()))
		return
	}
	items, total, err := ctl.roles.Page(c.Request.Context(), &query)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Page(c, items, total, query.Page, query.Size)
}

// ListAll 查询全部角色，供用户分配角色的下拉框使用。
//
// @Summary  全部角色
// @Tags     角色管理
// @Produce  json
// @Success  200 {object} response.Body{data=[]vo.RoleItem}
// @Router   /roles/all [get]
func (ctl *RoleController) ListAll(c *gin.Context) {
	items, err := ctl.roles.ListAll(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, items)
}

// Get 查询角色详情，含已分配菜单与部门 ID。
//
// @Summary  角色详情
// @Tags     角色管理
// @Produce  json
// @Param    id path int true "角色ID"
// @Success  200 {object} response.Body{data=vo.RoleDetail}
// @Router   /roles/{id} [get]
func (ctl *RoleController) Get(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	item, err := ctl.roles.Get(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, item)
}

// MenuIDs 查询角色已分配的菜单 ID，用于权限树回显。
//
// @Summary  角色菜单ID
// @Tags     角色管理
// @Produce  json
// @Param    id path int true "角色ID"
// @Success  200 {object} response.Body{data=[]int}
// @Router   /roles/{id}/menus [get]
func (ctl *RoleController) MenuIDs(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	ids, err := ctl.roles.MenuIDs(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, ids)
}

// Create 新增角色。
//
// @Summary  新增角色
// @Tags     角色管理
// @Accept   json
// @Produce  json
// @Param    body body dto.CreateRoleRequest true "角色信息"
// @Success  200 {object} response.Body
// @Router   /roles [post]
func (ctl *RoleController) Create(c *gin.Context) {
	var req dto.CreateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errs.BadRequest("参数错误："+err.Error()))
		return
	}
	if err := ctl.roles.Create(c.Request.Context(), middleware.CurrentUserID(c), &req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMessage(c, "新增成功")
}

// Update 修改角色。
//
// @Summary  修改角色
// @Tags     角色管理
// @Accept   json
// @Produce  json
// @Param    id   path int                   true "角色ID"
// @Param    body body dto.UpdateRoleRequest true "角色信息"
// @Success  200 {object} response.Body
// @Router   /roles/{id} [put]
func (ctl *RoleController) Update(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req dto.UpdateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errs.BadRequest("参数错误："+err.Error()))
		return
	}
	if err := ctl.roles.Update(c.Request.Context(), middleware.CurrentUserID(c), id, &req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMessage(c, "修改成功")
}

// Delete 删除角色。
//
// @Summary  删除角色
// @Tags     角色管理
// @Produce  json
// @Param    id path int true "角色ID"
// @Success  200 {object} response.Body
// @Router   /roles/{id} [delete]
func (ctl *RoleController) Delete(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	if err := ctl.roles.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMessage(c, "删除成功")
}

// AssignMenus 分配角色菜单权限。
//
// @Summary  分配菜单权限
// @Tags     角色管理
// @Accept   json
// @Produce  json
// @Param    id   path int                    true "角色ID"
// @Param    body body dto.AssignMenusRequest true "菜单ID集合"
// @Success  200 {object} response.Body
// @Router   /roles/{id}/menus [put]
func (ctl *RoleController) AssignMenus(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req dto.AssignMenusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errs.BadRequest("参数错误："+err.Error()))
		return
	}
	if err := ctl.roles.AssignMenus(c.Request.Context(), id, req.MenuIDs); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMessage(c, "分配成功")
}

// SetDataScope 设置角色数据权限范围。
//
// @Summary  设置数据权限
// @Tags     角色管理
// @Accept   json
// @Produce  json
// @Param    id   path int                  true "角色ID"
// @Param    body body dto.DataScopeRequest true "数据范围"
// @Success  200 {object} response.Body
// @Router   /roles/{id}/data-scope [put]
func (ctl *RoleController) SetDataScope(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req dto.DataScopeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errs.BadRequest("参数错误："+err.Error()))
		return
	}
	if err := ctl.roles.SetDataScope(c.Request.Context(), id, &req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMessage(c, "设置成功")
}

// MenuController 提供菜单管理接口。
type MenuController struct {
	menus *service.MenuService
}

func NewMenuController(menus *service.MenuService) *MenuController {
	return &MenuController{menus: menus}
}

// Tree 返回完整菜单树（含隐藏与停用项），供菜单管理页与角色授权树使用。
//
// @Summary  菜单树
// @Tags     菜单管理
// @Produce  json
// @Success  200 {object} response.Body{data=[]vo.MenuNode}
// @Router   /menus/tree [get]
func (ctl *MenuController) Tree(c *gin.Context) {
	nodes, err := ctl.menus.Tree(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nodes)
}

// Create 新增菜单。
//
// @Summary  新增菜单
// @Tags     菜单管理
// @Accept   json
// @Produce  json
// @Param    body body dto.CreateMenuRequest true "菜单信息"
// @Success  200 {object} response.Body
// @Router   /menus [post]
func (ctl *MenuController) Create(c *gin.Context) {
	var req dto.CreateMenuRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errs.BadRequest("参数错误："+err.Error()))
		return
	}
	if err := ctl.menus.Create(c.Request.Context(), middleware.CurrentUserID(c), &req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMessage(c, "新增成功")
}

// Update 修改菜单。
//
// @Summary  修改菜单
// @Tags     菜单管理
// @Accept   json
// @Produce  json
// @Param    id   path int                   true "菜单ID"
// @Param    body body dto.UpdateMenuRequest true "菜单信息"
// @Success  200 {object} response.Body
// @Router   /menus/{id} [put]
func (ctl *MenuController) Update(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req dto.UpdateMenuRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errs.BadRequest("参数错误："+err.Error()))
		return
	}
	if err := ctl.menus.Update(c.Request.Context(), middleware.CurrentUserID(c), id, &req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMessage(c, "修改成功")
}

// Delete 删除菜单。
//
// @Summary  删除菜单
// @Tags     菜单管理
// @Produce  json
// @Param    id path int true "菜单ID"
// @Success  200 {object} response.Body
// @Router   /menus/{id} [delete]
func (ctl *MenuController) Delete(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	if err := ctl.menus.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMessage(c, "删除成功")
}

// DeptController 提供部门管理接口。
type DeptController struct {
	depts *service.DeptService
}

func NewDeptController(depts *service.DeptService) *DeptController {
	return &DeptController{depts: depts}
}

// Tree 返回部门树。
//
// @Summary  部门树
// @Tags     部门管理
// @Produce  json
// @Success  200 {object} response.Body{data=[]vo.DeptNode}
// @Router   /depts/tree [get]
func (ctl *DeptController) Tree(c *gin.Context) {
	nodes, err := ctl.depts.Tree(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nodes)
}

// Create 新增部门。
//
// @Summary  新增部门
// @Tags     部门管理
// @Accept   json
// @Produce  json
// @Param    body body dto.CreateDeptRequest true "部门信息"
// @Success  200 {object} response.Body
// @Router   /depts [post]
func (ctl *DeptController) Create(c *gin.Context) {
	var req dto.CreateDeptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errs.BadRequest("参数错误："+err.Error()))
		return
	}
	if err := ctl.depts.Create(c.Request.Context(), middleware.CurrentUserID(c), &req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMessage(c, "新增成功")
}

// Update 修改部门。
//
// @Summary  修改部门
// @Tags     部门管理
// @Accept   json
// @Produce  json
// @Param    id   path int                   true "部门ID"
// @Param    body body dto.UpdateDeptRequest true "部门信息"
// @Success  200 {object} response.Body
// @Router   /depts/{id} [put]
func (ctl *DeptController) Update(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req dto.UpdateDeptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errs.BadRequest("参数错误："+err.Error()))
		return
	}
	if err := ctl.depts.Update(c.Request.Context(), middleware.CurrentUserID(c), id, &req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMessage(c, "修改成功")
}

// Delete 删除部门。
//
// @Summary  删除部门
// @Tags     部门管理
// @Produce  json
// @Param    id path int true "部门ID"
// @Success  200 {object} response.Body
// @Router   /depts/{id} [delete]
func (ctl *DeptController) Delete(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	if err := ctl.depts.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMessage(c, "删除成功")
}
