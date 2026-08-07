// Package router 负责路由注册与分组。
package router

import (
	"net/http"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"github.com/fangzhengcn/workbackend/apps/backend/config"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/controller"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/middleware"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/repository"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/service"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/cache"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/jwt"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/response"

	// 空导入生成的 docs 包：它在 init() 里把 swagger 规范注册到
	// swag 的全局注册表，ginSwagger.WrapHandler 才能读到。
	_ "github.com/fangzhengcn/workbackend/apps/backend/docs"
)

// 权限标识常量，与 packages/shared/src/perms.ts 及 sys_menu.perms 保持一致。
const (
	permUserList     = "system:user:list"
	permUserQuery    = "system:user:query"
	permUserAdd      = "system:user:add"
	permUserEdit     = "system:user:edit"
	permUserRemove   = "system:user:remove"
	permUserResetPwd = "system:user:resetPwd"
	permUserExport   = "system:user:export"
	permUserAssign   = "system:user:assignRole"

	permRoleList       = "system:role:list"
	permRoleQuery      = "system:role:query"
	permRoleAdd        = "system:role:add"
	permRoleEdit       = "system:role:edit"
	permRoleRemove     = "system:role:remove"
	permRoleAssignMenu = "system:role:assignMenu"
	permRoleDataScope  = "system:role:dataScope"

	permMenuList   = "system:menu:list"
	permMenuAdd    = "system:menu:add"
	permMenuEdit   = "system:menu:edit"
	permMenuRemove = "system:menu:remove"

	permDeptList   = "system:dept:list"
	permDeptAdd    = "system:dept:add"
	permDeptEdit   = "system:dept:edit"
	permDeptRemove = "system:dept:remove"

	permDictList   = "system:dict:list"
	permDictAdd    = "system:dict:add"
	permDictEdit   = "system:dict:edit"
	permDictRemove = "system:dict:remove"

	permOperLogList   = "system:operlog:list"
	permOperLogRemove = "system:operlog:remove"
	permOperLogExport = "system:operlog:export"

	permLoginLogList   = "system:loginlog:list"
	permLoginLogRemove = "system:loginlog:remove"
	permLoginLogExport = "system:loginlog:export"
)

// Dependencies 汇总路由注册所需的全部依赖。
type Dependencies struct {
	Config      *config.Config
	JWT         *jwt.Manager
	Cache       *cache.Client
	Permissions *service.PermissionService
	Logs        *repository.LogRepository

	Auth *controller.AuthController
	User *controller.UserController
	Role *controller.RoleController
	Menu *controller.MenuController
	Dept *controller.DeptController
	Dict *controller.DictController
	Log  *controller.LogController
}

// Setup 构建 Gin 引擎并注册全部路由。
func Setup(deps *Dependencies) *gin.Engine {
	gin.SetMode(deps.Config.App.Mode)

	engine := gin.New()
	// Recovery 必须最先注册，才能兜住后续中间件与 handler 的 panic。
	engine.Use(middleware.Recovery())
	engine.Use(middleware.Logger())
	engine.Use(middleware.CORS(deps.Config.CORS.AllowOrigins))

	// 健康检查：不鉴权，供负载均衡与容器探针使用。
	engine.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	/*
	 * Swagger UI：仅非生产环境暴露。
	 *
	 * 生产环境关闭的理由：这份文档完整列出全部接口、参数结构与字段含义，
	 * 等于把攻击面主动摊开；接口本身有鉴权，但没必要额外送一份地图。
	 * 需要在生产查文档时，用 docs/swagger.json 在内网自行渲染。
	 */
	if !deps.Config.App.IsProduction() {
		engine.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	}

	/*
	 * 上传文件的静态访问。
	 *
	 * 不鉴权：头像要能直接放进 <img src>，带 Token 的请求做不到这一点；
	 * 文件名是 16 字节随机 hex，不可枚举，泄露风险可接受。
	 *
	 * 用 Static 而非 StaticFS + http.Dir 的裸目录：gin 的 Static 会做路径清理，
	 * 挡住 /uploads/../config/xxx 这类穿越尝试。
	 * 真正的安全底线在写入侧——pkg/upload 只接受嗅探为图片的内容、
	 * 文件名全由服务端生成，因此这个目录里不会出现可执行内容。
	 */
	if prefix := deps.Config.Upload.URLPrefix; prefix != "" {
		engine.Static(prefix, deps.Config.Upload.Dir)
	}

	engine.NoRoute(func(c *gin.Context) {
		response.FailWithCode(c, http.StatusNotFound, "接口不存在")
	})

	v1 := engine.Group("/api/v1")

	registerPublicRoutes(v1, deps)
	registerAuthedRoutes(v1, deps)

	return engine
}

// registerPublicRoutes 注册无需登录的接口。
func registerPublicRoutes(group *gin.RouterGroup, deps *Dependencies) {
	auth := group.Group("/auth")
	{
		auth.POST("/login", deps.Auth.Login)
		auth.GET("/captcha", deps.Auth.Captcha)
		auth.POST("/refresh", deps.Auth.Refresh)
	}
}

// registerAuthedRoutes 注册需要登录的接口。
//
// 分层防护：JWTAuth 校验「是谁」，RequirePerm 校验「能不能做」。
// 每个写接口都必须显式声明所需权限点，这是安全边界所在。
func registerAuthedRoutes(group *gin.RouterGroup, deps *Dependencies) {
	authed := group.Group("")
	authed.Use(middleware.JWTAuth(deps.JWT, deps.Cache))

	// 权限点简写，减少下方噪音。
	perm := func(code string) gin.HandlerFunc {
		return middleware.RequirePerm(deps.Permissions, code)
	}
	operLog := func(title string, businessType int8) gin.HandlerFunc {
		return middleware.OperLog(deps.Logs, title, businessType)
	}
	const (
		typeInsert = 1
		typeUpdate = 2
		typeDelete = 3
		// typeQuery 用于导出：它是读操作，但需要留痕。
		typeQuery = 4
	)

	// 当前用户自身相关接口：已登录即可访问，无需额外权限点。
	{
		authed.POST("/auth/logout", deps.Auth.Logout)
		authed.GET("/auth/info", deps.Auth.Info)
		authed.GET("/auth/menus", deps.Auth.Menus)
		authed.PUT("/auth/password", operLog("个人中心", typeUpdate), deps.Auth.ChangePassword)
		// 个人资料：只能改自己，故不挂 RequirePerm——任何登录用户都该能改自己的昵称。
		authed.PUT("/auth/profile", operLog("个人中心", typeUpdate), deps.User.UpdateProfile)
		// 头像上传：只能改自己的，故不挂 RequirePerm。
		authed.POST("/auth/avatar", operLog("个人中心", typeUpdate), deps.User.UploadAvatar)
	}

	users := authed.Group("/users")
	{
		users.GET("", perm(permUserList), deps.User.List)
		/*
		 * 导出必须注册在 /:id 之前，否则 "export" 会被当成 id 参数匹配走。
		 *
		 * 挂 operLog：导出是一次性把大量数据带出系统，属于最该留痕的操作之一。
		 * 中间件只记请求参数与状态，不记响应体，因此不会把导出内容写进日志表。
		 */
		users.GET("/export", perm(permUserExport), operLog("导出用户", typeQuery), deps.User.Export)
		users.GET("/:id", perm(permUserQuery), deps.User.Get)
		users.POST("", perm(permUserAdd), operLog("用户管理", typeInsert), deps.User.Create)
		users.PUT("/:id", perm(permUserEdit), operLog("用户管理", typeUpdate), deps.User.Update)
		users.DELETE("/:id", perm(permUserRemove), operLog("用户管理", typeDelete), deps.User.Delete)
		users.PUT("/:id/password", perm(permUserResetPwd), operLog("重置密码", typeUpdate), deps.User.ResetPassword)
		users.PUT("/:id/roles", perm(permUserAssign), operLog("分配角色", typeUpdate), deps.User.AssignRoles)
	}

	roles := authed.Group("/roles")
	{
		roles.GET("", perm(permRoleList), deps.Role.List)
		// 下拉选择用，权限放宽到「能管理用户」即可，否则无法给用户分配角色。
		// 必须注册在 /:id 之前，否则 "all" 会被当作 id 参数匹配走。
		roles.GET("/all", perm(permUserList), deps.Role.ListAll)
		roles.GET("/:id", perm(permRoleQuery), deps.Role.Get)
		roles.GET("/:id/menus", perm(permRoleList), deps.Role.MenuIDs)
		roles.POST("", perm(permRoleAdd), operLog("角色管理", typeInsert), deps.Role.Create)
		roles.PUT("/:id", perm(permRoleEdit), operLog("角色管理", typeUpdate), deps.Role.Update)
		roles.DELETE("/:id", perm(permRoleRemove), operLog("角色管理", typeDelete), deps.Role.Delete)
		roles.PUT("/:id/menus", perm(permRoleAssignMenu), operLog("分配菜单权限", typeUpdate), deps.Role.AssignMenus)
		roles.PUT("/:id/data-scope", perm(permRoleDataScope), operLog("设置数据权限", typeUpdate), deps.Role.SetDataScope)
	}

	menus := authed.Group("/menus")
	{
		menus.GET("/tree", perm(permMenuList), deps.Menu.Tree)
		menus.POST("", perm(permMenuAdd), operLog("菜单管理", typeInsert), deps.Menu.Create)
		menus.PUT("/:id", perm(permMenuEdit), operLog("菜单管理", typeUpdate), deps.Menu.Update)
		menus.DELETE("/:id", perm(permMenuRemove), operLog("菜单管理", typeDelete), deps.Menu.Delete)
	}

	depts := authed.Group("/depts")
	{
		// 部门树被用户管理页的部门筛选依赖，故用户列表权限也可读取。
		depts.GET("/tree", perm(permDeptList), deps.Dept.Tree)
		depts.POST("", perm(permDeptAdd), operLog("部门管理", typeInsert), deps.Dept.Create)
		depts.PUT("/:id", perm(permDeptEdit), operLog("部门管理", typeUpdate), deps.Dept.Update)
		depts.DELETE("/:id", perm(permDeptRemove), operLog("部门管理", typeDelete), deps.Dept.Delete)
	}

	dicts := authed.Group("/dicts")
	{
		dicts.GET("/types", perm(permDictList), deps.Dict.ListTypes)
		dicts.POST("/types", perm(permDictAdd), operLog("字典类型", typeInsert), deps.Dict.CreateType)
		dicts.PUT("/types/:id", perm(permDictEdit), operLog("字典类型", typeUpdate), deps.Dict.UpdateType)
		dicts.DELETE("/types/:id", perm(permDictRemove), operLog("字典类型", typeDelete), deps.Dict.DeleteType)

		dicts.GET("/data", perm(permDictList), deps.Dict.ListData)
		/*
		 * 按类型取字典项：路径特意加了 /type 这一段。
		 *
		 * 若写成 /data/:type，会与下面的 /data/:id 在 gin 里冲突——
		 * 同一位置不能既是 :type 又是 :id，启动即 panic。
		 * 该接口供各业务页面的下拉框使用，权限放宽到「能查字典」即可。
		 */
		dicts.GET("/data/type/:type", perm(permDictList), deps.Dict.DataByType)
		dicts.POST("/data", perm(permDictAdd), operLog("字典数据", typeInsert), deps.Dict.CreateData)
		dicts.PUT("/data/:id", perm(permDictEdit), operLog("字典数据", typeUpdate), deps.Dict.UpdateData)
		dicts.DELETE("/data/:id", perm(permDictRemove), operLog("字典数据", typeDelete), deps.Dict.DeleteData)
	}

	/*
	 * 日志模块只读 + 删除：日志由中间件与登录流程写入，不提供人工新增/修改，
	 * 否则审计价值不复存在。
	 *
	 * 删除日志本身也会被 operLog 中间件记一条新日志——这是有意为之，
	 * 「谁清空了日志」恰恰是最需要留痕的操作。
	 */
	operLogs := authed.Group("/oper-logs")
	{
		operLogs.GET("", perm(permOperLogList), deps.Log.ListOperLogs)
		// /clean 与 /export 必须注册在 /:id 之前，否则会被当作 id 参数匹配走。
		operLogs.GET("/export", perm(permOperLogExport), operLog("导出操作日志", typeQuery), deps.Log.ExportOperLogs)
		operLogs.DELETE("/clean", perm(permOperLogRemove), operLog("操作日志", typeDelete), deps.Log.CleanOperLogs)
		operLogs.GET("/:id", perm(permOperLogList), deps.Log.GetOperLog)
		operLogs.DELETE("", perm(permOperLogRemove), operLog("操作日志", typeDelete), deps.Log.DeleteOperLogs)
	}

	loginLogs := authed.Group("/login-logs")
	{
		loginLogs.GET("", perm(permLoginLogList), deps.Log.ListLoginLogs)
		loginLogs.GET("/export", perm(permLoginLogExport), operLog("导出登录日志", typeQuery), deps.Log.ExportLoginLogs)
		loginLogs.DELETE("/clean", perm(permLoginLogRemove), operLog("登录日志", typeDelete), deps.Log.CleanLoginLogs)
		loginLogs.DELETE("", perm(permLoginLogRemove), operLog("登录日志", typeDelete), deps.Log.DeleteLoginLogs)
	}
}
