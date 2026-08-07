package treeutil

import (
	"encoding/json"
	"testing"
)

// node 是测试用的最小树节点实现。
type node struct {
	ID       uint64  `json:"id"`
	ParentID uint64  `json:"parentId"`
	Children []*node `json:"children"`
}

func (n *node) NodeID() uint64        { return n.ID }
func (n *node) ParentIDValue() uint64 { return n.ParentID }
func (n *node) SetChildren(c []*node) { n.Children = c }

// TestBuildEmptyReturnsNonNilSlice 锁定「无节点时返回空切片而非 nil」。
//
// 这个用例来自一个真实故障：普通角色未被授予任何菜单时，/auth/menus 返回
// {"data":null}，前端路由守卫对结果直接 .map() 遍历，抛出
// 「Cannot read properties of null (reading 'map')」，
// 表现为登录成功却停在登录页——报错信息与真实原因（无菜单授权）毫无关联，
// 极难排查。故在建树这一层就保证不返回 nil。
func TestBuildEmptyReturnsNonNilSlice(t *testing.T) {
	cases := map[string][]*node{
		"nil 输入": nil,
		"空切片输入":  {},
		"全是孤儿节点": {{ID: 5, ParentID: 999}},
	}

	for name, items := range cases {
		t.Run(name, func(t *testing.T) {
			got := Build(items, 0)
			if got == nil {
				t.Fatal("返回了 nil，会被序列化成 null 导致前端 .map() 崩溃")
			}
			if len(got) != 0 {
				t.Fatalf("期望空结果，实际 %d 个节点", len(got))
			}

			// 直接验证序列化结果，这才是前端真正拿到的东西。
			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("序列化失败: %v", err)
			}
			if string(encoded) != "[]" {
				t.Fatalf("期望 JSON 为 []，实际为 %s", encoded)
			}
		})
	}
}

// TestBuildAssemblesTree 验证正常建树、层级与顺序。
func TestBuildAssemblesTree(t *testing.T) {
	items := []*node{
		{ID: 1, ParentID: 0},
		{ID: 2, ParentID: 1},
		{ID: 3, ParentID: 1},
		{ID: 4, ParentID: 2},
		{ID: 100, ParentID: 0},
	}

	roots := Build(items, 0)
	if len(roots) != 2 {
		t.Fatalf("期望 2 个根节点，实际 %d", len(roots))
	}
	if roots[0].ID != 1 || roots[1].ID != 100 {
		t.Fatalf("根节点顺序应与输入一致，实际 %d,%d", roots[0].ID, roots[1].ID)
	}
	if len(roots[0].Children) != 2 {
		t.Fatalf("节点 1 期望 2 个子节点，实际 %d", len(roots[0].Children))
	}
	// 孙节点必须挂到正确的父级上，而不是被拍平到根。
	if len(roots[0].Children[0].Children) != 1 || roots[0].Children[0].Children[0].ID != 4 {
		t.Fatal("节点 4 未正确挂到节点 2 之下")
	}
}

// TestBuildDropsOrphans 验证父节点缺失的孤儿节点被丢弃，而非错挂到根上。
func TestBuildDropsOrphans(t *testing.T) {
	items := []*node{
		{ID: 1, ParentID: 0},
		{ID: 7, ParentID: 404}, // 父节点不存在
	}

	roots := Build(items, 0)
	if len(roots) != 1 || roots[0].ID != 1 {
		t.Fatalf("孤儿节点应被丢弃，实际根节点数 %d", len(roots))
	}
}

// TestBuildSurvivesCycle 验证脏数据构成环时不会无限递归。
//
// A.parent=B 且 B.parent=A 时，若无环检测会直接栈溢出打挂进程。
func TestBuildSurvivesCycle(t *testing.T) {
	items := []*node{
		{ID: 1, ParentID: 0},
		{ID: 2, ParentID: 3},
		{ID: 3, ParentID: 2},
	}

	// 只要能返回就说明没有无限递归；互为父子的 2/3 都进不了以 0 为根的树。
	roots := Build(items, 0)
	if len(roots) != 1 || roots[0].ID != 1 {
		t.Fatalf("期望只有节点 1 成为根，实际根节点数 %d", len(roots))
	}
}
