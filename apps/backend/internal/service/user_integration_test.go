package service

import (
	"context"
	"testing"

	"gorm.io/gorm"

	"github.com/fangzhengcn/workbackend/apps/backend/internal/dto"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/model"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/repository"
)

// newUserServiceForTest 组装一个可用的 UserService（缓存传 nil，走空实现）。
func newUserServiceForTest(t *testing.T, db *gorm.DB) *UserService {
	t.Helper()

	users := repository.NewUserRepository(db)
	roles := repository.NewRoleRepository(db)
	depts := repository.NewDeptRepository(db)
	return NewUserService(users, roles, NewDataScopeService(roles, depts), nil)
}

// createTestUser 建一个属于指定部门的用户，返回其 ID。
//
// 显式指定 ID 从 10 起：自增会让第一个用户拿到 id=1，
// 而那正是受保护的超级管理员 ID，删除类用例会被保护逻辑挡下而误判为失败。
func createTestUser(t *testing.T, db *gorm.DB, username string, deptID uint64) uint64 {
	t.Helper()

	hashed, err := HashPassword("123456")
	if err != nil {
		t.Fatalf("生成密码哈希失败: %v", err)
	}
	user := &model.User{
		ID:       nextTestUserID(),
		Username: username,
		Password: hashed,
		Nickname: model.EncryptedString("测试用户"),
		DeptID:   &deptID,
		Status:   model.StatusEnabled,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("创建测试用户失败: %v", err)
	}
	return user.ID
}

// testUserIDSeq 为测试用户分配 ID，避开 adminUserID(=1)。
var testUserIDSeq uint64 = 10

func nextTestUserID() uint64 {
	testUserIDSeq++
	return testUserIDSeq
}

/*
 * TestUserUpdateChangesDeptID 是一条回归测试。
 *
 * 真实故障：把用户的部门从「研发部」改成「财务部」，接口返回 200，
 * 数据库里的 dept_id 却没变。
 *
 * 原因是 UserRepository.FindByID 用 Preload("Dept") 载入了旧部门实体，
 * 而 GORM 的 Save 默认「完整保存关联」——它按 user.Dept 这个旧实体反推
 * dept_id 写回，把刚赋的新值覆盖成了旧值。
 *
 * 这个缺陷读代码完全看不出来（赋值语句就在那儿），接口也返回成功，
 * 单元测试与 mock 同样拦不住：只有真正落库再读回来才会暴露。
 * 修法是 Save 时 Omit("Dept", "Roles")。
 */
func TestUserUpdateChangesDeptID(t *testing.T) {
	setupCipher(t)
	db := newTestDB(t)
	seedDepts(t, db)

	const devDept, opsDept = uint64(2), uint64(3)
	userID := createTestUser(t, db, "fz", devDept)

	svc := newUserServiceForTest(t, db)
	newDept := opsDept
	err := svc.Update(context.Background(), 1, userID, &dto.UpdateUserRequest{DeptID: &newDept})
	if err != nil {
		t.Fatalf("修改用户失败: %v", err)
	}

	// 关键断言：直接查库，不复用 service 的返回值——
	// 后者可能返回内存中已改好的实体，掩盖「没落库」这个事实。
	var got model.User
	if err := db.First(&got, userID).Error; err != nil {
		t.Fatalf("读取用户失败: %v", err)
	}
	if got.DeptID == nil || *got.DeptID != opsDept {
		var actual any = "nil"
		if got.DeptID != nil {
			actual = *got.DeptID
		}
		t.Fatalf("dept_id 未更新：期望 %d，实际 %v\n"+
			"这正是 GORM 关联自动保存把 dept_id 覆盖回旧值的症状，"+
			"检查 Save 是否漏了 Omit(\"Dept\", \"Roles\")", opsDept, actual)
	}
}

// TestUserUpdateKeepsPhoneWhenOmitted 验证「不传手机号即保持原值」。
//
// 前端编辑用户时手机号留空表示不修改（列表返回的是脱敏值，回填会毁掉真实号码）。
// 后端靠 *string 的 nil 表达这一语义，若误把 nil 当成「清空」，
// 用户改个昵称就会丢掉手机号——且因为字段是密文，肉眼比对不出来。
func TestUserUpdateKeepsPhoneWhenOmitted(t *testing.T) {
	setupCipher(t)
	db := newTestDB(t)
	seedDepts(t, db)

	hashed, _ := HashPassword("123456")
	deptID := uint64(2)
	user := &model.User{
		Username: "keeper",
		Password: hashed,
		Phone:    model.EncryptedString("13800138000"),
		DeptID:   &deptID,
		Status:   model.StatusEnabled,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}

	svc := newUserServiceForTest(t, db)
	nickname := "改了昵称"
	// 只传昵称，Phone 为 nil
	err := svc.Update(context.Background(), 1, user.ID, &dto.UpdateUserRequest{Nickname: &nickname})
	if err != nil {
		t.Fatalf("修改用户失败: %v", err)
	}

	var got model.User
	if err := db.First(&got, user.ID).Error; err != nil {
		t.Fatalf("读取用户失败: %v", err)
	}
	if got.Phone.String() != "13800138000" {
		t.Errorf("手机号被意外改动：期望 13800138000，实际 %q", got.Phone.String())
	}
	if got.Nickname.String() != nickname {
		t.Errorf("昵称未更新：期望 %q，实际 %q", nickname, got.Nickname.String())
	}
}

// TestUserPhoneBlindIndexRoundTrip 验证密文写入与盲索引查询能对上。
//
// 手机号以 AES-GCM 密文存储，无法直接 WHERE phone=?，必须先算 HMAC 盲索引
// 再查 phone_hash 列。这条链路涉及 BeforeSave 钩子自动维护 hash，
// 只要钩子没触发或算法不一致，按手机号查用户就会永远查不到。
func TestUserPhoneBlindIndexRoundTrip(t *testing.T) {
	setupCipher(t)
	db := newTestDB(t)
	seedDepts(t, db)

	const phone = "13912345678"
	hashed, _ := HashPassword("123456")
	deptID := uint64(2)
	user := &model.User{
		Username: "blind",
		Password: hashed,
		Phone:    model.EncryptedString(phone),
		DeptID:   &deptID,
		Status:   model.StatusEnabled,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}

	// 落库的必须是密文，不能是明文——否则加密形同虚设。
	var rawPhone string
	err := db.Model(&model.User{}).Where("id = ?", user.ID).Select("phone").Scan(&rawPhone).Error
	if err != nil {
		t.Fatalf("读取原始列失败: %v", err)
	}
	if rawPhone == phone {
		t.Fatal("手机号以明文落库，加密未生效")
	}

	// BeforeSave 应已自动算出盲索引；用它能精确查回该用户。
	cipher, err := model.Cipher()
	if err != nil {
		t.Fatalf("获取加密器失败: %v", err)
	}
	repo := repository.NewUserRepository(db)
	found, err := repo.FindByPhoneHash(context.Background(), cipher.BlindIndex(phone))
	if err != nil {
		t.Fatalf("按盲索引查询失败: %v（BeforeSave 可能未维护 phone_hash）", err)
	}
	if found.ID != user.ID {
		t.Errorf("查到的用户不对：期望 %d，实际 %d", user.ID, found.ID)
	}
	// 读回来应自动解密成明文。
	if found.Phone.String() != phone {
		t.Errorf("读回的手机号未解密：期望 %q，实际 %q", phone, found.Phone.String())
	}
}

// TestUserDeleteIsSoftDelete 验证删除为软删除且不再出现在查询里。
func TestUserDeleteIsSoftDelete(t *testing.T) {
	setupCipher(t)
	db := newTestDB(t)
	seedDepts(t, db)

	userID := createTestUser(t, db, "tobedeleted", 2)
	svc := newUserServiceForTest(t, db)

	if err := svc.Delete(context.Background(), userID); err != nil {
		t.Fatalf("删除用户失败: %v", err)
	}

	// 常规查询应查不到。
	var count int64
	db.Model(&model.User{}).Where("id = ?", userID).Count(&count)
	if count != 0 {
		t.Error("软删除后常规查询仍能查到该用户")
	}

	// 但行还在（Unscoped 可见），deleted_at 已填——这是软删除的定义。
	var withDeleted int64
	db.Unscoped().Model(&model.User{}).Where("id = ?", userID).Count(&withDeleted)
	if withDeleted != 1 {
		t.Error("记录被物理删除了，与 sys_user 的软删除设计不符")
	}
}

// TestUserDeleteProtectsAdmin 验证初始超级管理员不可删除。
//
// 这是防呆的关键一环：删掉 id=1 会让系统失去唯一的管理入口，且无法从界面恢复。
func TestUserDeleteProtectsAdmin(t *testing.T) {
	setupCipher(t)
	db := newTestDB(t)
	seedDepts(t, db)

	svc := newUserServiceForTest(t, db)
	if err := svc.Delete(context.Background(), adminUserID); err == nil {
		t.Fatal("超级管理员竟然被删除了，保护失效")
	}
}
