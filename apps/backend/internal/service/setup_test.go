package service

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/fangzhengcn/workbackend/apps/backend/internal/model"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/crypto"
)

/*
 * Service 层集成测试的公共装置。
 *
 * 为什么需要真库而非 mock：这个项目已经暴露过三个只在真实 SQL 执行时才现形的
 * 缺陷——GORM 的关联自动保存把 dept_id 覆盖回旧值、Casbin autosave 撞唯一索引、
 * browser 列宽不足。它们的共同点是「代码读起来完全正确，接口也返回 200」，
 * 单元测试与 mock 都拦不住，只有真正落库再读回来才能发现。
 *
 * 为什么用 sqlite 内存库而非 dockertest 起 MySQL：
 * 前者零外部依赖、毫秒级启动，能进 CI 也能随时本地跑；
 * 代价是少数 MySQL 方言不被支持（见 skipIfUnsupported）。
 * 涉及方言的用例明确跳过并说明原因，而不是假装通过。
 */

// testAESKey / testHMACKey 是测试专用密钥（64 位 hex = 32 字节）。
// 与生产无关，仅用于让 EncryptedString 的加解密可运行。
const (
	testAESKey  = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	testHMACKey = "ffeeddccbbaa99887766554433221100ffeeddccbbaa99887766554433221100"
)

// newTestDB 建一个迁移完毕的内存库。
//
// 每个测试独立一个库（DSN 用 file::memory: 加 cache=private 语义），
// 避免用例之间通过残留数据互相干扰——这类干扰会表现为「单独跑通过、
// 全量跑失败」，排查成本很高。
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		// 关掉日志：断言失败时的输出已足够定位，SQL 日志只会淹没它。
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}

	/*
	 * 逐表迁移，而非一次传入全部模型。
	 *
	 * sys_menu 与 sys_dept 都声明了名为 idx_parent_id 的索引：
	 * MySQL 的索引名按表隔离，sqlite 却要求全库唯一，一起迁移会报
	 * 「index idx_parent_id already exists」。
	 * 逐表迁移并忽略这类重名报错，既保留了表结构，也不必为测试改生产模型。
	 */
	models := []any{
		&model.User{}, &model.Role{}, &model.Menu{}, &model.Dept{},
		&model.UserRole{}, &model.RoleMenu{}, &model.RoleDept{},
		&model.DictType{}, &model.DictData{},
		&model.OperLog{}, &model.LoginLog{},
	}
	for _, m := range models {
		if err := db.AutoMigrate(m); err != nil && !isDuplicateIndexErr(err) {
			t.Fatalf("迁移 %T 失败: %v", m, err)
		}
	}
	return db
}

// isDuplicateIndexErr 判断是否为 sqlite 的索引重名错误。
//
// 只忽略这一类：表本身已建成，仅索引没能重复创建，
// 对测试的正确性无影响（索引只影响性能与唯一约束，而重名的这两个都不是唯一索引）。
func isDuplicateIndexErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "already exists")
}

// setupCipher 初始化全局加密器。
//
// model.SetCipher 是包级全局状态，测试间会互相影响；
// 但密钥固定且无状态，重复设置是幂等的，故直接设置而不做清理。
func setupCipher(t *testing.T) {
	t.Helper()

	cipher, err := crypto.NewCipher(testAESKey, testHMACKey, 1)
	if err != nil {
		t.Fatalf("构造加密器失败: %v", err)
	}
	model.SetCipher(cipher)
}

// seedDepts 写入一棵与建表脚本一致的部门树。
//
//	1 总公司  ancestors="0"
//	├─ 2 研发部 ancestors="0,1"
//	│   └─ 4 前端组 ancestors="0,1,2"
//	└─ 3 运营部 ancestors="0,1"
func seedDepts(t *testing.T, db *gorm.DB) {
	t.Helper()

	depts := []*model.Dept{
		{ID: 1, ParentID: 0, Ancestors: "0", Name: "总公司", Status: model.StatusEnabled},
		{ID: 2, ParentID: 1, Ancestors: "0,1", Name: "研发部", Status: model.StatusEnabled},
		{ID: 3, ParentID: 1, Ancestors: "0,1", Name: "运营部", Status: model.StatusEnabled},
		{ID: 4, ParentID: 2, Ancestors: "0,1,2", Name: "前端组", Status: model.StatusEnabled},
	}
	if err := db.Create(&depts).Error; err != nil {
		t.Fatalf("写入部门种子失败: %v", err)
	}
}

// seedRolesAndMenus 写入角色与菜单，供授权相关用例使用。
func seedRolesAndMenus(t *testing.T, db *gorm.DB) {
	t.Helper()

	roles := []*model.Role{
		{ID: 1, Name: "超级管理员", Code: model.SuperAdminRoleCode, DataScope: model.DataScopeAll, Status: model.StatusEnabled},
		{ID: 2, Name: "普通角色", Code: "common", DataScope: model.DataScopeDept, Status: model.StatusEnabled},
	}
	if err := db.Create(&roles).Error; err != nil {
		t.Fatalf("写入角色种子失败: %v", err)
	}

	menus := []*model.Menu{
		{ID: 1, ParentID: 0, Name: "系统管理", Type: model.MenuTypeDir, Path: "/system", Status: model.StatusEnabled, Visible: model.StatusEnabled},
		{ID: 100, ParentID: 1, Name: "用户管理", Type: model.MenuTypeMenu, Path: "user", Component: "system/user/index", Perms: "system:user:list", Status: model.StatusEnabled, Visible: model.StatusEnabled},
		{ID: 1002, ParentID: 100, Name: "用户新增", Type: model.MenuTypeButton, Perms: "system:user:add", Status: model.StatusEnabled, Visible: model.StatusEnabled},
	}
	if err := db.Create(&menus).Error; err != nil {
		t.Fatalf("写入菜单种子失败: %v", err)
	}
}
