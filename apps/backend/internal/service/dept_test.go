package service

import (
	"testing"

	"github.com/fangzhengcn/workbackend/apps/backend/internal/model"
)

// TestDeptChildAncestors 验证子部门的 ancestors 拼接。
//
// 这个值是 DataScopeDeptTree 子树过滤的唯一依据
// （FindSubtreeIDs 用 FIND_IN_SET(?, ancestors) 查询）。
// 拼错不会报错，但数据权限会静默失准——用户看到不该看的数据，
// 或看不到本该看的，且从界面上完全无法察觉。
func TestDeptChildAncestors(t *testing.T) {
	cases := []struct {
		name      string
		id        uint64
		ancestors string
		want      string
	}{
		// 与建表脚本的种子数据一致：总公司 id=1 ancestors="0"，
		// 其子部门（研发部/运营部）的 ancestors 就是 "0,1"。
		{"顶级部门的子级", 1, "0", "0,1"},
		{"二级部门的子级", 2, "0,1", "0,1,2"},
		{"三级部门的子级", 5, "0,1,2", "0,1,2,5"},
		// ancestors 为空时退化为只写自身 ID，避免拼出前导逗号 ",1"
		// ——那会让 FIND_IN_SET 匹配到一个空字符串项。
		{"ancestors 为空", 9, "", "9"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dept := &model.Dept{ID: tc.id, Ancestors: tc.ancestors}
			if got := dept.ChildAncestors(); got != tc.want {
				t.Errorf("期望 %q，实际 %q", tc.want, got)
			}
		})
	}
}

// TestDeptAncestorsRebuildCoversAllDescendants 验证移动部门后，
// 整棵子树（含孙级）的 ancestors 都被重算。
//
// 只改被移动节点自己是最容易犯的错：它的后代 ancestors 仍指向旧路径，
// 子树过滤从此失准。这里复刻 rebuildDescendantAncestors 的递归逻辑。
//
// 初始树形：1(总公司,"0") → 2(研发部,"0,1") → 4(前端组,"0,1,2")
//
//	3(运营部,"0,1")
//
// 操作：把 2 移到 3 之下
// 期望：2 → "0,1,3"，4 → "0,1,3,2"（孙级也必须跟着变）
func TestDeptAncestorsRebuildCoversAllDescendants(t *testing.T) {
	depts := map[uint64]*model.Dept{
		1: {ID: 1, ParentID: 0, Ancestors: "0", Name: "总公司"},
		2: {ID: 2, ParentID: 1, Ancestors: "0,1", Name: "研发部"},
		3: {ID: 3, ParentID: 1, Ancestors: "0,1", Name: "运营部"},
		4: {ID: 4, ParentID: 2, Ancestors: "0,1,2", Name: "前端组"},
	}

	// 把 2 挂到 3 之下，并按新父级重算自身 ancestors。
	moved := depts[2]
	moved.ParentID = 3
	moved.Ancestors = depts[3].ChildAncestors()

	// 复刻 rebuildDescendantAncestors：按 parent_id 分组后自顶向下递归。
	childrenOf := make(map[uint64][]*model.Dept)
	for _, dept := range depts {
		childrenOf[dept.ParentID] = append(childrenOf[dept.ParentID], dept)
	}
	visited := make(map[uint64]bool)
	var walk func(parent *model.Dept)
	walk = func(parent *model.Dept) {
		if visited[parent.ID] {
			return
		}
		visited[parent.ID] = true
		childAncestors := parent.ChildAncestors()
		for _, child := range childrenOf[parent.ID] {
			child.Ancestors = childAncestors
			walk(child)
		}
	}
	walk(moved)

	if got := depts[2].Ancestors; got != "0,1,3" {
		t.Errorf("被移动的部门 2：期望 ancestors=\"0,1,3\"，实际 %q", got)
	}
	if got := depts[4].Ancestors; got != "0,1,3,2" {
		t.Errorf("孙级部门 4：期望 ancestors=\"0,1,3,2\"，实际 %q——"+
			"后代未跟着重算，子树过滤将失准", got)
	}
	// 未受影响的分支不应被改动。
	if got := depts[3].Ancestors; got != "0,1" {
		t.Errorf("无关部门 3 被误改成 %q", got)
	}
}

// TestDeptAncestorsRebuildSurvivesCycle 确认脏环不会让重算无限递归。
func TestDeptAncestorsRebuildSurvivesCycle(t *testing.T) {
	// 6 与 7 互为父子，属于不该出现但需兜住的脏数据。
	depts := map[uint64]*model.Dept{
		6: {ID: 6, ParentID: 7, Ancestors: "0,7"},
		7: {ID: 7, ParentID: 6, Ancestors: "0,6"},
	}

	childrenOf := make(map[uint64][]*model.Dept)
	for _, dept := range depts {
		childrenOf[dept.ParentID] = append(childrenOf[dept.ParentID], dept)
	}

	visited := make(map[uint64]bool)
	var walk func(parent *model.Dept)
	walk = func(parent *model.Dept) {
		if visited[parent.ID] {
			return // 正是这一行防止死循环
		}
		visited[parent.ID] = true
		for _, child := range childrenOf[parent.ID] {
			child.Ancestors = parent.ChildAncestors()
			walk(child)
		}
	}

	// 能返回即说明 visited 生效；缺了它这里会栈溢出打挂进程。
	walk(depts[6])

	if len(visited) != 2 {
		t.Errorf("期望访问 2 个节点后停止，实际 %d", len(visited))
	}
}

// TestDeptAncestorIDs 验证 ancestors 的解析。
func TestDeptAncestorIDs(t *testing.T) {
	dept := &model.Dept{Ancestors: "0,1,2"}
	ids := dept.AncestorIDs()
	if len(ids) != 3 || ids[0] != "0" || ids[2] != "2" {
		t.Errorf("解析结果不符：%v", ids)
	}

	// 空串必须返回 nil 而非 [""]：后者会让「祖先数量」判断多算一层。
	empty := &model.Dept{Ancestors: ""}
	if got := empty.AncestorIDs(); got != nil {
		t.Errorf("空 ancestors 应返回 nil，实际 %v", got)
	}
}
