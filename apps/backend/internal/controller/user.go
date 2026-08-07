package controller

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/fangzhengcn/workbackend/apps/backend/config"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/dto"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/middleware"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/model"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/service"
	_ "github.com/fangzhengcn/workbackend/apps/backend/internal/vo"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/errs"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/export"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/response"
)

// UserController 提供用户管理接口。
type UserController struct {
	users *service.UserService
	// upload 是文件上传配置；头像接口需要存储目录与对外前缀。
	upload config.UploadConfig
}

func NewUserController(users *service.UserService, uploadCfg config.UploadConfig) *UserController {
	return &UserController{users: users, upload: uploadCfg}
}

// UploadAvatar 上传当前登录用户的头像。
//
// 只能改自己的头像：操作对象取自 Token，不接受前端传 ID。
// 文件类型按内容嗅探而非扩展名判定（见 pkg/upload），
// 否则把可执行内容改名为 .png 就能落到静态目录下被访问。
//
// @Summary  上传头像
// @Tags     个人中心
// @Accept   multipart/form-data
// @Produce  json
// @Param    file formData file true "头像图片（JPG/PNG/GIF/WEBP，不超过 2MB）"
// @Success  200 {object} response.Body{data=string} "头像访问 URL"
// @Router   /auth/avatar [post]
func (ctl *UserController) UploadAvatar(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		/*
		 * 把失败原因与实际收到的 Content-Type 一起告知。
		 *
		 * 只回「请选择要上传的图片」时，前端明明选了文件却收到这句提示，
		 * 完全无法判断是字段名不对、还是 multipart 头缺 boundary
		 * （后者是接入上传时最常踩的坑）。带上 Content-Type 能一眼分辨。
		 */
		contentType := c.GetHeader("Content-Type")
		if !strings.HasPrefix(contentType, "multipart/form-data") {
			response.Fail(c, errs.BadRequest(
				"请求不是 multipart/form-data（当前为 "+contentType+"），无法解析上传文件"))
			return
		}
		response.Fail(c, errs.BadRequest("未找到名为 file 的上传字段："+err.Error()))
		return
	}

	url, err := ctl.users.UpdateAvatar(
		c.Request.Context(), middleware.CurrentUserID(c),
		file, ctl.upload.Dir, ctl.upload.URLPrefix,
	)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, url)
}

// Export 导出用户列表为 CSV。
//
// 手机号/邮箱是脱敏值（与列表接口同一套 VO 构造函数）：
// 导出文件极易外流且流向不可控，不在此开绕过脱敏的后门。
//
// @Summary  导出用户
// @Tags     用户管理
// @Produce  text/csv
// @Param    query query dto.UserQuery false "查询条件（与列表一致）"
// @Success  200 {file} file "CSV 文件"
// @Router   /users/export [get]
func (ctl *UserController) Export(c *gin.Context) {
	var query dto.UserQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Fail(c, errs.BadRequest("参数错误："+err.Error()))
		return
	}

	items, err := ctl.users.Export(c.Request.Context(), middleware.CurrentUserID(c), &query)
	if err != nil {
		response.Fail(c, err)
		return
	}

	/*
	 * 响应头必须在写入任何内容之前设置。
	 *
	 * 一旦开始写 body，状态码与头就已发出，此后即使出错也无法再改成
	 * JSON 错误响应——用户会下载到一个内容截断的 CSV 而不是看到错误提示。
	 * 故所有可能失败的查询都放在设置头之前完成。
	 */
	export.SetHeaders(c.Writer, export.Filename("用户列表", time.Now()))

	w, err := export.NewWriter(c.Writer)
	if err != nil {
		response.Fail(c, errs.Internal("导出失败").WithCause(err))
		return
	}

	header := []string{"账号", "昵称", "手机号", "邮箱", "性别", "部门", "角色", "状态", "创建时间"}
	if err := w.WriteRow(header); err != nil {
		// 头已发出，无法再返回 JSON 错误；记录到 gin 错误链由日志中间件统一记录。
		_ = c.Error(err)
		return
	}

	for _, item := range items {
		roleNames := make([]string, 0, len(item.Roles))
		for _, role := range item.Roles {
			roleNames = append(roleNames, role.Name)
		}
		row := []string{
			item.Username,
			item.Nickname,
			item.Phone, // 已脱敏
			item.Email, // 已脱敏
			genderLabel(item.Gender),
			item.DeptName,
			strings.Join(roleNames, "、"),
			statusLabel(item.Status),
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

// genderLabel 把性别码转成可读文字，避免导出文件里出现裸数字。
func genderLabel(gender int8) string {
	switch gender {
	case model.GenderMale:
		return "男"
	case model.GenderFemale:
		return "女"
	default:
		return "未知"
	}
}

// statusLabel 把状态码转成可读文字。
func statusLabel(status int8) string {
	if status == model.StatusEnabled {
		return "正常"
	}
	return "停用"
}

// UpdateProfile 修改当前登录用户自己的资料。
//
// 路由挂在 /auth/profile（语义是「我」），实现放在 UserController：
// 加密字段赋值与手机号/邮箱查重的逻辑都在 UserService 里，
// 复制一份到 AuthService 迟早会走偏。
//
// @Summary  修改个人资料
// @Tags     个人中心
// @Accept   json
// @Produce  json
// @Param    body body dto.UpdateProfileRequest true "个人资料"
// @Success  200 {object} response.Body
// @Router   /auth/profile [put]
func (ctl *UserController) UpdateProfile(c *gin.Context) {
	var req dto.UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errs.BadRequest("参数错误："+err.Error()))
		return
	}
	// 操作对象固定取自 Token，不接受前端传 ID，避免越权修改他人资料。
	if err := ctl.users.UpdateProfile(c.Request.Context(), middleware.CurrentUserID(c), &req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMessage(c, "保存成功")
}

// List 分页查询用户。
//
// @Summary  用户列表
// @Tags     用户管理
// @Produce  json
// @Success  200 {object} response.Body{data=response.PageData}
// @Router   /users [get]
func (ctl *UserController) List(c *gin.Context) {
	var query dto.UserQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Fail(c, errs.BadRequest("参数错误："+err.Error()))
		return
	}

	items, total, err := ctl.users.Page(c.Request.Context(), middleware.CurrentUserID(c), &query)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Page(c, items, total, query.Page, query.Size)
}

// Get 查询用户详情。
//
// @Summary  用户详情
// @Tags     用户管理
// @Produce  json
// @Param    id path int true "用户ID"
// @Success  200 {object} response.Body{data=vo.UserItem}
// @Router   /users/{id} [get]
func (ctl *UserController) Get(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	item, err := ctl.users.Get(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, item)
}

// Create 新增用户。
//
// @Summary  新增用户
// @Tags     用户管理
// @Accept   json
// @Produce  json
// @Param    body body dto.CreateUserRequest true "用户参数"
// @Success  200 {object} response.Body
// @Router   /users [post]
func (ctl *UserController) Create(c *gin.Context) {
	var req dto.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errs.BadRequest("参数错误："+err.Error()))
		return
	}
	if err := ctl.users.Create(c.Request.Context(), middleware.CurrentUserID(c), &req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMessage(c, "新增成功")
}

// Update 修改用户。
//
// @Summary  修改用户
// @Tags     用户管理
// @Accept   json
// @Produce  json
// @Param    id   path int                   true "用户ID"
// @Param    body body dto.UpdateUserRequest true "用户参数"
// @Success  200  {object} response.Body
// @Router   /users/{id} [put]
func (ctl *UserController) Update(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req dto.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errs.BadRequest("参数错误："+err.Error()))
		return
	}
	if err := ctl.users.Update(c.Request.Context(), middleware.CurrentUserID(c), id, &req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMessage(c, "修改成功")
}

// Delete 删除用户。
//
// @Summary  删除用户
// @Tags     用户管理
// @Produce  json
// @Param    id path int true "用户ID"
// @Success  200 {object} response.Body
// @Router   /users/{id} [delete]
func (ctl *UserController) Delete(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	if err := ctl.users.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMessage(c, "删除成功")
}

// ResetPassword 重置用户密码。
//
// @Summary  重置密码
// @Tags     用户管理
// @Accept   json
// @Produce  json
// @Param    id   path int                      true "用户ID"
// @Param    body body dto.ResetPasswordRequest true "密码参数"
// @Success  200  {object} response.Body
// @Router   /users/{id}/password [put]
func (ctl *UserController) ResetPassword(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req dto.ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errs.BadRequest("参数错误："+err.Error()))
		return
	}
	if err := ctl.users.ResetPassword(c.Request.Context(), id, req.Password); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMessage(c, "密码重置成功")
}

// AssignRoles 分配用户角色。
//
// @Summary  分配角色
// @Tags     用户管理
// @Accept   json
// @Produce  json
// @Param    id   path int                    true "用户ID"
// @Param    body body dto.AssignRolesRequest true "角色参数"
// @Success  200  {object} response.Body
// @Router   /users/{id}/roles [put]
func (ctl *UserController) AssignRoles(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req dto.AssignRolesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errs.BadRequest("参数错误："+err.Error()))
		return
	}
	if err := ctl.users.AssignRoles(c.Request.Context(), id, req.RoleIDs); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMessage(c, "分配成功")
}

// parseIDParam 解析路径中的 :id 参数。
func parseIDParam(c *gin.Context) (uint64, error) {
	raw := c.Param("id")
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || id == 0 {
		return 0, errs.BadRequest("ID 参数非法")
	}
	return id, nil
}
