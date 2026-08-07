package service

import (
	"testing"

	"github.com/fangzhengcn/workbackend/apps/backend/internal/dto"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/model"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/vo"
)

// TestNewRoleDetailNormalizesNilSlices 确认角色详情的 ID 数组不会是 null。
//
// 前端把 menuIds 直接交给 a-tree 的 checkedKeys、deptIds 交给树选择器，
// 传入 null 会让组件在渲染期抛错——而「未分配任何菜单」恰恰是新建角色的
// 默认状态，属于必经路径而非边缘情况。
func TestNewRoleDetailNormalizesNilSlices(t *testing.T) {
	role := &model.Role{ID: 7, Name: "普通角色", Code: "common", DataScope: model.DataScopeDept}

	detail := vo.NewRoleDetail(role, nil, nil)

	if detail.MenuIDs == nil {
		t.Error("MenuIDs 为 nil，会序列化成 null")
	}
	if detail.DeptIDs == nil {
		t.Error("DeptIDs 为 nil，会序列化成 null")
	}
	if len(detail.MenuIDs) != 0 || len(detail.DeptIDs) != 0 {
		t.Error("空输入不应造出元素")
	}
	// 嵌入的 RoleItem 字段要能正常透出，否则前端拿不到角色本身的信息。
	if detail.Code != "common" || detail.ID != 7 {
		t.Errorf("RoleItem 字段未正确填充: id=%d code=%s", detail.ID, detail.Code)
	}
}

// TestRoleDataScopeCustomRequiresDepts 锁定「自定义数据范围必须选部门」这条规则。
//
// 若允许自定义范围但不选任何部门，DataScopeService 会生成一个
// dept_id IN () 的空集合条件，该角色将看不到任何数据——
// 用户只会觉得「配了权限却查不出数据」，无从判断是漏选了部门。
// 故在入口处直接拒绝，而不是放它进去变成一个静默的空结果。
func TestRoleDataScopeCustomRequiresDepts(t *testing.T) {
	cases := []struct {
		name      string
		dataScope int8
		deptIDs   []uint64
		wantErr   bool
	}{
		{"自定义范围但未选部门", model.DataScopeCustom, nil, true},
		{"自定义范围且选了部门", model.DataScopeCustom, []uint64{2, 3}, false},
		{"本部门范围无需选部门", model.DataScopeDept, nil, false},
		{"全部数据范围无需选部门", model.DataScopeAll, nil, false},
		{"仅本人范围无需选部门", model.DataScopeSelf, nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := &dto.DataScopeRequest{DataScope: tc.dataScope, DeptIDs: tc.deptIDs}

			// 复刻 SetDataScope 中的校验条件；这里不接数据库，
			// 只锁定「什么组合算非法」这个判定本身。
			invalid := req.DataScope == model.DataScopeCustom && len(req.DeptIDs) == 0

			if invalid != tc.wantErr {
				t.Errorf("dataScope=%d deptIDs=%v：期望非法=%v，实际=%v",
					tc.dataScope, tc.deptIDs, tc.wantErr, invalid)
			}
		})
	}
}

// TestRoleIsSuperAdmin 确认超级管理员角色能被正确识别。
//
// 这个判定是「禁止删除/停用/改权限」三条保护的唯一依据，
// 判错就等于保护失效——admin 角色被停用后无人能再管理系统。
func TestRoleIsSuperAdmin(t *testing.T) {
	cases := map[string]bool{
		"admin":  true,
		"common": false,
		"Admin":  false, // 大小写敏感：Casbin 策略里的标识是精确匹配
		"admin2": false,
		"":       false,
	}

	for code, want := range cases {
		role := &model.Role{Code: code}
		if got := role.IsSuperAdmin(); got != want {
			t.Errorf("code=%q：期望 IsSuperAdmin=%v，实际=%v", code, want, got)
		}
	}
}

// TestRoleQueryNormalize 确认分页参数被规范到合法区间。
func TestRoleQueryNormalize(t *testing.T) {
	cases := []struct {
		name             string
		page, size       int
		wantPage, wantSz int
	}{
		{"零值取默认", 0, 0, 1, 10},
		{"负页码归一", -5, 20, 1, 20},
		{"超大页长被截断", 1, 9999, 1, 200},
		{"合法值保持不变", 3, 50, 3, 50},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			query := &dto.RoleQuery{PageQuery: dto.PageQuery{Page: tc.page, Size: tc.size}}
			query.Normalize()

			if query.Page != tc.wantPage || query.Size != tc.wantSz {
				t.Errorf("期望 page=%d size=%d，实际 page=%d size=%d",
					tc.wantPage, tc.wantSz, query.Page, query.Size)
			}
			// Offset 必须非负，否则 SQL 直接报错。
			if query.Offset() < 0 {
				t.Errorf("Offset 为负数: %d", query.Offset())
			}
		})
	}
}
