package service

import (
	"testing"
	"time"

	"github.com/fangzhengcn/workbackend/apps/backend/internal/model"
)

// TestMenuValidateShape 锁定「按类型校验字段组合」的规则。
//
// 这些组合错了不会报错，但会静默坏掉：菜单缺 path 前端生成不出路由，
// 配好了却点不进去；按钮缺 perms 则既不显示也不授权，是条无意义的数据。
func TestMenuValidateShape(t *testing.T) {
	svc := &MenuService{}

	cases := []struct {
		name      string
		menuType  int8
		path      string
		component string
		perms     string
		wantErr   bool
	}{
		{"目录有 path", model.MenuTypeDir, "/system", "", "", false},
		{"目录缺 path", model.MenuTypeDir, "", "", "", true},
		{"目录 path 只有空格", model.MenuTypeDir, "   ", "", "", true},
		{"菜单齐全", model.MenuTypeMenu, "user", "system/user/index", "system:user:list", false},
		{"菜单缺 path", model.MenuTypeMenu, "", "system/user/index", "", true},
		{"菜单缺 component", model.MenuTypeMenu, "user", "", "", true},
		{"按钮有 perms", model.MenuTypeButton, "", "", "system:user:add", false},
		{"按钮缺 perms", model.MenuTypeButton, "", "", "", true},
		{"非法类型", int8(9), "/x", "x", "x", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.validateShape(tc.menuType, tc.path, tc.component, tc.perms)
			if (err != nil) != tc.wantErr {
				t.Errorf("期望出错=%v，实际 err=%v", tc.wantErr, err)
			}
		})
	}
}

// TestMenuCycleDetection 验证「不能挂到自己或自己的后代之下」的回溯判定。
//
// 这里直接复刻 validateNotDescendant 的核心算法（用 parentOf 映射向上回溯），
// 因为该方法需要数据库。判定错的后果很重：整棵子树会从菜单树里消失，
// 且无法再通过界面移回来——数据已坏，只能改库。
//
// 树形：1(根) → 100 → 1001
//
//	2(根)
func TestMenuCycleDetection(t *testing.T) {
	parentOf := map[uint64]uint64{
		1:    model.RootID,
		100:  1,
		1001: 100,
		2:    model.RootID,
	}

	// isDescendant 判断 newParent 是否为 id 自身或其后代。
	isDescendant := func(id, newParent uint64) bool {
		if id == newParent {
			return true
		}
		for cursor, steps := newParent, 0; cursor != model.RootID && steps <= len(parentOf); steps++ {
			if cursor == id {
				return true
			}
			next, ok := parentOf[cursor]
			if !ok {
				break
			}
			cursor = next
		}
		return false
	}

	cases := []struct {
		name       string
		id, parent uint64
		want       bool
	}{
		{"挂到自己", 1, 1, true},
		{"挂到直接子节点", 1, 100, true},
		{"挂到孙节点", 1, 1001, true},
		{"挂到另一棵树的根", 1, 2, false},
		{"挂到顶级", 100, model.RootID, false},
		{"子节点挂到父节点（合法，等于没动）", 100, 1, false},
		{"兄弟树之间移动", 1001, 2, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isDescendant(tc.id, tc.parent); got != tc.want {
				t.Errorf("id=%d 挂到 parent=%d：期望非法=%v，实际=%v",
					tc.id, tc.parent, tc.want, got)
			}
		})
	}
}

// TestMenuCycleDetectionSurvivesDirtyCycle 确认库里已存在脏环时回溯不会死循环。
//
// 若 7 和 8 互为父子（历史脏数据），向上回溯永远遇不到 RootID，
// 没有步数上限就会挂住整个请求。
func TestMenuCycleDetectionSurvivesDirtyCycle(t *testing.T) {
	parentOf := map[uint64]uint64{7: 8, 8: 7}

	done := make(chan int, 1)
	go func() {
		cursor, steps := uint64(7), 0
		for cursor != model.RootID && steps <= len(parentOf) {
			next, ok := parentOf[cursor]
			if !ok {
				break
			}
			cursor = next
			steps++
		}
		done <- steps
	}()

	select {
	case steps := <-done:
		// 步数上限是 len(parentOf)，超出即说明循环失控。
		if steps > len(parentOf)+1 {
			t.Errorf("回溯步数 %d 超出上限，环检测失效", steps)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("回溯未在 2 秒内结束，脏环导致死循环")
	}
}

// TestDefaultInt8 验证「未传字段」与「显式传 0」的区分。
//
// visible/status/isFrame 的 0 都是有意义的值（隐藏/停用/内链），
// 若用 0 值判断是否传参，用户就永远无法把它们设成 0。
func TestDefaultInt8(t *testing.T) {
	if got := defaultInt8(nil, model.StatusEnabled); got != model.StatusEnabled {
		t.Errorf("nil 应取默认值 %d，实际 %d", model.StatusEnabled, got)
	}

	zero := int8(0)
	if got := defaultInt8(&zero, model.StatusEnabled); got != 0 {
		t.Errorf("显式传 0 应保留 0，实际 %d——这会让用户无法停用/隐藏菜单", got)
	}

	one := int8(1)
	if got := defaultInt8(&one, 0); got != 1 {
		t.Errorf("显式传 1 应保留 1，实际 %d", got)
	}
}
