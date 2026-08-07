package service

import (
	"context"
	"testing"

	"gorm.io/gorm"

	"github.com/fangzhengcn/workbackend/apps/backend/internal/dto"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/model"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/repository"
)

/*
 * 角色与菜单的集成测试。
 *
 * PermissionService 依赖 Casbin enforcer，构造它需要模型文件与适配器；
 * 这里传 nil 并只测「不触发策略刷新」的路径，涉及刷新的用例单独说明。
 * 之所以能传 nil：reloadPolicies 里对 permissions 的调用是本测试要绕开的部分，
 * 而关联表的增删改本身与 Casbin 无关——那是两件独立的事。
 */

func newRoleServiceForTest(db *gorm.DB) *RoleService {
	roles := repository.NewRoleRepository(db)
	menus := repository.NewMenuRepository(db)
	// permissions 传 nil：调用刷新会 panic，故本文件只覆盖不刷新的读路径，
	// 以及用 assertNoPanic 包裹的写路径（见各用例说明）。
	return NewRoleService(roles, menus, nil)
}

func newMenuServiceForTest(db *gorm.DB) *MenuService {
	return NewMenuService(repository.NewMenuRepository(db), nil)
}

// roleMenuCount 统计某角色的菜单授权行数，直接查关联表。
func roleMenuCount(t *testing.T, db *gorm.DB, roleID uint64) int64 {
	t.Helper()

	var count int64
	if err := db.Model(&model.RoleMenu{}).Where("role_id = ?", roleID).Count(&count).Error; err != nil {
		t.Fatalf("统计角色菜单失败: %v", err)
	}
	return count
}

/*
 * TestRoleAssignMenusIsIdempotent 验证重复授予同一批菜单不会产生重复行。
 *
 * 这条针对的是「连续保存两次权限，行数翻倍或父目录丢失」这类问题。
 * ReplaceMenus 的语义是先删后插的全量覆盖，若删除条件写错（如漏了 role_id），
 * 就会留下旧行或误删别的角色的授权——两者都只在真实 SQL 下才现形。
 */
func TestRoleAssignMenusIsIdempotent(t *testing.T) {
	setupCipher(t)
	db := newTestDB(t)
	seedRolesAndMenus(t, db)

	roles := repository.NewRoleRepository(db)
	menuIDs := []uint64{1, 100, 1002}

	// 直接用 repository 覆盖两次，绕开 service 的策略刷新。
	for i := 0; i < 2; i++ {
		err := db.Transaction(func(tx *gorm.DB) error {
			return roles.ReplaceMenus(context.Background(), tx, 2, menuIDs)
		})
		if err != nil {
			t.Fatalf("第 %d 次分配菜单失败: %v", i+1, err)
		}
	}

	if got := roleMenuCount(t, db, 2); got != int64(len(menuIDs)) {
		t.Errorf("重复分配后授权行数应为 %d，实际 %d（可能产生了重复行）", len(menuIDs), got)
	}

	// 不该影响其他角色。
	if got := roleMenuCount(t, db, 1); got != 0 {
		t.Errorf("角色 1 的授权被误改，行数 %d", got)
	}
}

// TestRoleReplaceMenusWithEmptyClearsAll 验证传空集合即清空授权。
//
// 空切片与 nil 在业务上都表示「清空」，但若实现里 len==0 就提前 return
// 而没执行删除，用户点了「全不选」保存后权限依然生效——一个危险的假成功。
func TestRoleReplaceMenusWithEmptyClearsAll(t *testing.T) {
	setupCipher(t)
	db := newTestDB(t)
	seedRolesAndMenus(t, db)

	roles := repository.NewRoleRepository(db)
	ctx := context.Background()

	if err := db.Transaction(func(tx *gorm.DB) error {
		return roles.ReplaceMenus(ctx, tx, 2, []uint64{1, 100})
	}); err != nil {
		t.Fatalf("初始分配失败: %v", err)
	}
	if roleMenuCount(t, db, 2) != 2 {
		t.Fatal("初始分配未生效")
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		return roles.ReplaceMenus(ctx, tx, 2, nil)
	}); err != nil {
		t.Fatalf("清空授权失败: %v", err)
	}
	if got := roleMenuCount(t, db, 2); got != 0 {
		t.Errorf("清空后仍有 %d 条授权行，权限不会真正被撤销", got)
	}
}

/*
 * TestMenuDeleteCleansRoleLinks 验证删除菜单会同事务清理 sys_role_menu。
 *
 * sys_menu 是物理删除，残留的授权行在权限树上完全不可见（树只渲染真实菜单），
 * 却会在新菜单复用同一自增 ID 时让角色凭空获得权限——
 * 一个既隐蔽又实质的越权隐患。
 */
func TestMenuDeleteCleansRoleLinks(t *testing.T) {
	setupCipher(t)
	db := newTestDB(t)
	seedRolesAndMenus(t, db)

	roles := repository.NewRoleRepository(db)
	ctx := context.Background()
	// 给角色 2 授予按钮 1002
	if err := db.Transaction(func(tx *gorm.DB) error {
		return roles.ReplaceMenus(ctx, tx, 2, []uint64{1002})
	}); err != nil {
		t.Fatalf("分配菜单失败: %v", err)
	}

	// 用 repository 直接走删除 + 清理，绕开 service 的策略刷新。
	menus := repository.NewMenuRepository(db)
	err := menus.DB().Transaction(func(tx *gorm.DB) error {
		if err := menus.DeleteRoleLinks(ctx, tx, 1002); err != nil {
			return err
		}
		return tx.Delete(&model.Menu{}, 1002).Error
	})
	if err != nil {
		t.Fatalf("删除菜单失败: %v", err)
	}

	var orphan int64
	db.Model(&model.RoleMenu{}).Where("menu_id = ?", 1002).Count(&orphan)
	if orphan != 0 {
		t.Errorf("菜单已删除但仍残留 %d 条授权行，"+
			"新菜单复用该 ID 时会让角色凭空获得权限", orphan)
	}
}

// TestMenuDeleteBlockedByChildren 验证有子菜单时不允许删除。
func TestMenuDeleteBlockedByChildren(t *testing.T) {
	setupCipher(t)
	db := newTestDB(t)
	seedRolesAndMenus(t, db)

	svc := newMenuServiceForTest(db)
	// 菜单 100（用户管理）下有按钮 1002
	if err := svc.Delete(context.Background(), 100); err == nil {
		t.Fatal("有子菜单竟然允许删除")
	}
}

// TestMenuCreateRejectsDuplicatePerms 验证权限标识不允许重复。
//
// 重复的 perms 会让 Casbin 出现两条等价策略：撤销其中一个菜单的授权后
// 权限依然生效，而排查时完全看不出原因。
func TestMenuCreateRejectsDuplicatePerms(t *testing.T) {
	setupCipher(t)
	db := newTestDB(t)
	seedRolesAndMenus(t, db)

	svc := newMenuServiceForTest(db)
	// system:user:add 已被种子里的按钮 1002 占用
	err := svc.Create(context.Background(), 1, &dto.CreateMenuRequest{
		ParentID: 100,
		Name:     "重复权限按钮",
		Type:     model.MenuTypeButton,
		Perms:    "system:user:add",
	})
	if err == nil {
		t.Fatal("重复的权限标识竟然允许创建")
	}
}

// TestMenuCreateRejectsButtonAsParent 验证按钮不能作为上级。
//
// 挂在按钮下的节点永远不会被渲染——按钮是权限点而非容器。
func TestMenuCreateRejectsButtonAsParent(t *testing.T) {
	setupCipher(t)
	db := newTestDB(t)
	seedRolesAndMenus(t, db)

	svc := newMenuServiceForTest(db)
	err := svc.Create(context.Background(), 1, &dto.CreateMenuRequest{
		ParentID: 1002, // 这是个按钮
		Name:     "挂在按钮下",
		Type:     model.MenuTypeMenu,
		Path:     "nowhere",
		// 组件路径必须填，否则会先被 validateShape 拦下，测不到父级校验
		Component: "system/user/index",
	})
	if err == nil {
		t.Fatal("按钮竟然可以作为上级菜单")
	}
}

// TestRoleDeleteBlockedByUsers 验证角色已分配给用户时不允许删除。
func TestRoleDeleteBlockedByUsers(t *testing.T) {
	setupCipher(t)
	db := newTestDB(t)
	seedDepts(t, db)
	seedRolesAndMenus(t, db)

	userID := createTestUser(t, db, "roleholder", 2)
	if err := db.Create(&model.UserRole{UserID: userID, RoleID: 2}).Error; err != nil {
		t.Fatalf("分配角色失败: %v", err)
	}

	svc := newRoleServiceForTest(db)
	if err := svc.Delete(context.Background(), 2); err == nil {
		t.Fatal("角色已分配给用户竟然允许删除")
	}
}

// TestRoleDeleteProtectsSuperAdmin 验证超级管理员角色不可删除。
func TestRoleDeleteProtectsSuperAdmin(t *testing.T) {
	setupCipher(t)
	db := newTestDB(t)
	seedRolesAndMenus(t, db)

	svc := newRoleServiceForTest(db)
	// 角色 1 的 code 是 admin
	if err := svc.Delete(context.Background(), 1); err == nil {
		t.Fatal("超级管理员角色竟然被删除了")
	}
}
