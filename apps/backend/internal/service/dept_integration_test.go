package service

import (
	"context"
	"testing"

	"gorm.io/gorm"

	"github.com/fangzhengcn/workbackend/apps/backend/internal/dto"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/model"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/repository"
)

func newDeptServiceForTest(db *gorm.DB) *DeptService {
	return NewDeptService(repository.NewDeptRepository(db))
}

// ancestorsOf 直接查库取某部门的 ancestors，绕开 service 的内存态。
func ancestorsOf(t *testing.T, db *gorm.DB, id uint64) string {
	t.Helper()

	var dept model.Dept
	if err := db.First(&dept, id).Error; err != nil {
		t.Fatalf("读取部门 %d 失败: %v", id, err)
	}
	return dept.Ancestors
}

/*
 * TestDeptMoveRebuildsDescendantAncestors 用真实 SQL 验证移动部门后
 * 整棵子树的 ancestors 都被重算。
 *
 * dept_test.go 里已有同名意图的用例，但那是在内存里复刻算法——
 * 它能验证算法本身，却无法证明「事务里那几条 UPDATE 真的写进了库」。
 * ancestors 是 DataScopeDeptTree 子树过滤的唯一依据（FIND_IN_SET 查询），
 * 只更新被移动节点自己而漏掉后代，数据权限就会静默失准：
 * 用户看到不该看的数据，界面上完全无从察觉。
 *
 * 初始：1 总公司 → 2 研发部 → 4 前端组
 *                → 3 运营部
 * 操作：把 2 移到 3 之下
 * 期望：2 → "0,1,3"，且孙级 4 → "0,1,3,2"
 */
func TestDeptMoveRebuildsDescendantAncestors(t *testing.T) {
	setupCipher(t)
	db := newTestDB(t)
	seedDepts(t, db)

	svc := newDeptServiceForTest(db)
	newParent := uint64(3) // 运营部
	err := svc.Update(context.Background(), 1, 2, &dto.UpdateDeptRequest{ParentID: &newParent})
	if err != nil {
		t.Fatalf("移动部门失败: %v", err)
	}

	if got := ancestorsOf(t, db, 2); got != "0,1,3" {
		t.Errorf("被移动的部门 2：期望 ancestors=%q，实际 %q", "0,1,3", got)
	}
	if got := ancestorsOf(t, db, 4); got != "0,1,3,2" {
		t.Errorf("孙级部门 4：期望 ancestors=%q，实际 %q\n"+
			"后代未随之重算，DataScopeDeptTree 的子树过滤将失准", "0,1,3,2", got)
	}
	// 未受影响的分支不该被改动。
	if got := ancestorsOf(t, db, 3); got != "0,1" {
		t.Errorf("无关部门 3 被误改成 %q", got)
	}
}

// TestDeptMoveRejectsDescendantAsParent 验证不能把部门挂到自己的后代之下。
//
// 允许的话整棵子树会脱离根节点、从部门树上消失，且无法再从界面移回来。
func TestDeptMoveRejectsDescendantAsParent(t *testing.T) {
	setupCipher(t)
	db := newTestDB(t)
	seedDepts(t, db)

	svc := newDeptServiceForTest(db)

	cases := []struct {
		name       string
		id, parent uint64
	}{
		{"挂到自己", 2, 2},
		{"挂到直接子部门", 2, 4},
		{"挂到孙级", 1, 4},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parent := tc.parent
			err := svc.Update(context.Background(), 1, tc.id, &dto.UpdateDeptRequest{ParentID: &parent})
			if err == nil {
				t.Errorf("把部门 %d 挂到 %d 之下竟然成功了", tc.id, tc.parent)
			}
		})
	}

	// 被拒绝后原树形不该被破坏。
	if got := ancestorsOf(t, db, 4); got != "0,1,2" {
		t.Errorf("非法操作被拒后部门 4 的 ancestors 仍被改动：%q", got)
	}
}

// TestDeptDeleteBlockedByChildren 验证有子部门时不允许删除。
func TestDeptDeleteBlockedByChildren(t *testing.T) {
	setupCipher(t)
	db := newTestDB(t)
	seedDepts(t, db)

	svc := newDeptServiceForTest(db)
	// 部门 2 有子部门 4
	if err := svc.Delete(context.Background(), 2); err == nil {
		t.Fatal("有子部门竟然允许删除")
	}
}

// TestDeptDeleteBlockedByUsers 验证部门下还有用户时不允许删除。
//
// 放行会让这些用户的 dept_id 变成悬空引用，而数据权限按部门过滤，
// 悬空后他们可能一条数据都看不到。
func TestDeptDeleteBlockedByUsers(t *testing.T) {
	setupCipher(t)
	db := newTestDB(t)
	seedDepts(t, db)

	// 部门 3（运营部）没有子部门，但给它挂一个用户
	createTestUser(t, db, "opsuser", 3)

	svc := newDeptServiceForTest(db)
	if err := svc.Delete(context.Background(), 3); err == nil {
		t.Fatal("部门下有用户竟然允许删除")
	}

	// 移走用户后应可删除。
	if err := db.Model(&model.User{}).Where("dept_id = ?", 3).Update("dept_id", 1).Error; err != nil {
		t.Fatalf("转移用户失败: %v", err)
	}
	if err := svc.Delete(context.Background(), 3); err != nil {
		t.Fatalf("用户转移后仍无法删除部门: %v", err)
	}
}

// TestDeptCreateComputesAncestors 验证新建部门时 ancestors 按父级推导。
func TestDeptCreateComputesAncestors(t *testing.T) {
	setupCipher(t)
	db := newTestDB(t)
	seedDepts(t, db)

	svc := newDeptServiceForTest(db)

	// 挂在前端组(4, ancestors="0,1,2")下，应得 "0,1,2,4"
	err := svc.Create(context.Background(), 1, &dto.CreateDeptRequest{ParentID: 4, Name: "前端一组"})
	if err != nil {
		t.Fatalf("新建子部门失败: %v", err)
	}
	var child model.Dept
	if err := db.Where("name = ?", "前端一组").First(&child).Error; err != nil {
		t.Fatalf("读取新部门失败: %v", err)
	}
	if child.Ancestors != "0,1,2,4" {
		t.Errorf("子部门 ancestors：期望 %q，实际 %q", "0,1,2,4", child.Ancestors)
	}

	// 顶级部门（parentId=0）的 ancestors 应为 "0"，与建表脚本的种子约定一致。
	if err := svc.Create(context.Background(), 1, &dto.CreateDeptRequest{ParentID: 0, Name: "分公司"}); err != nil {
		t.Fatalf("新建顶级部门失败: %v", err)
	}
	var top model.Dept
	if err := db.Where("name = ?", "分公司").First(&top).Error; err != nil {
		t.Fatalf("读取顶级部门失败: %v", err)
	}
	if top.Ancestors != rootAncestors {
		t.Errorf("顶级部门 ancestors：期望 %q，实际 %q", rootAncestors, top.Ancestors)
	}
}

// TestDeptCreateRejectsDuplicateNameInSameParent 验证同级不许重名。
//
// 同级同名会让用户在部门树与下拉框里完全无法区分；
// 不同父级下允许同名（两个分公司各有「研发部」是合理的）。
func TestDeptCreateRejectsDuplicateNameInSameParent(t *testing.T) {
	setupCipher(t)
	db := newTestDB(t)
	seedDepts(t, db)

	svc := newDeptServiceForTest(db)

	// 总公司(1)下已有「研发部」
	err := svc.Create(context.Background(), 1, &dto.CreateDeptRequest{ParentID: 1, Name: "研发部"})
	if err == nil {
		t.Error("同级重名竟然允许创建")
	}

	// 换到别的父级下应允许。
	if err := svc.Create(context.Background(), 1, &dto.CreateDeptRequest{ParentID: 3, Name: "研发部"}); err != nil {
		t.Errorf("不同父级下的同名部门应允许创建，实际报错: %v", err)
	}
}
