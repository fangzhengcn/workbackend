// Package treeutil 提供由扁平列表在内存中组装树的通用能力。
//
// 对应设计文档「技术难点提示 3」：菜单树/部门树都存 parent_id，
// 一次查出扁平列表后在内存递归组装，比递归查库高效得多（1 次 SQL vs N 次）。
package treeutil

// Node 约束可参与建树的类型：能提供自身 ID、父 ID，并能挂载子节点。
//
// 因 SetChildren 需要修改接收者，T 应当是指针类型（如 *model.Menu），
// 相应地 Build 的入参与返回值都是指针切片。
// 方法名用 ParentIDValue 而非 ParentID，是为了避开实体上同名的字段。
type Node[T any] interface {
	NodeID() uint64
	ParentIDValue() uint64
	SetChildren([]T)
}

// Build 把扁平列表组装成以 rootID 为根的树，返回顶层节点集合。
//
// 输入顺序即输出顺序，因此调用方可先按 sort 字段排好序再传入。
// 父节点不存在的「孤儿」节点会被丢弃，避免脏数据导致整棵树错乱。
//
// 保证返回非 nil 切片：nil 会被 encoding/json 序列化成 null，
// 前端对树结果普遍直接 .map() 遍历，拿到 null 会抛
// 「Cannot read properties of null」。无节点时返回空切片而非 nil，
// 让「没有数据」在 JSON 里表达为 [] 而不是 null。
func Build[T Node[T]](items []T, rootID uint64) []T {
	// childrenOf 按父 ID 归集子节点，一次遍历完成，整体复杂度 O(n)。
	childrenOf := make(map[uint64][]T, len(items))
	exists := make(map[uint64]struct{}, len(items))
	for _, item := range items {
		exists[item.NodeID()] = struct{}{}
	}
	for _, item := range items {
		parentID := item.ParentIDValue()
		// 父节点缺失且不是根，视为孤儿节点跳过。
		if _, ok := exists[parentID]; !ok && parentID != rootID {
			continue
		}
		childrenOf[parentID] = append(childrenOf[parentID], item)
	}

	// attach 自顶向下递归挂载子节点。
	// visiting 防止脏数据构成环（如 A.parent=B 且 B.parent=A）时无限递归。
	visiting := make(map[uint64]bool, len(items))
	var attach func(parentID uint64) []T
	attach = func(parentID uint64) []T {
		children := childrenOf[parentID]
		for _, child := range children {
			id := child.NodeID()
			if visiting[id] {
				continue
			}
			visiting[id] = true
			child.SetChildren(attach(id))
			delete(visiting, id)
		}
		return children
	}
	roots := attach(rootID)
	if roots == nil {
		return []T{}
	}
	return roots
}
